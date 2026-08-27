package matches

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/middleware"
)

var (
	cachedMatchesData []byte
	cacheMutex        sync.RWMutex
)

// GetCachedData returns the current in-memory match cache (thread-safe).
func GetCachedData() []byte {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()
	return cachedMatchesData
}

// injectFirstBloods derives per-player first-blood / first-death counts from the
// match kill feed and writes them onto each player as first_bloods / first_deaths.
// Must run BEFORE the kills array is stripped from the served payload. The earliest
// kill in each round is a first blood for the killer and a first death for the victim
// (same definition as the premier stats module).
func injectFirstBloods(m map[string]interface{}) {
	killsRaw, ok := m["kills"].([]interface{})
	if !ok {
		return
	}
	type firstKill struct {
		killer, victim string
		t              float64
		set            bool
	}
	roundFirst := map[float64]firstKill{}
	for _, kr := range killsRaw {
		k, ok := kr.(map[string]interface{})
		if !ok {
			continue
		}
		round, _ := k["round"].(float64)
		t, _ := k["kill_time_in_round"].(float64)
		killer, _ := k["killer_puuid"].(string)
		victim, _ := k["victim_puuid"].(string)
		if prev, exists := roundFirst[round]; !exists || !prev.set || t < prev.t {
			roundFirst[round] = firstKill{killer: killer, victim: victim, t: t, set: true}
		}
	}
	fb := map[string]int{}
	fd := map[string]int{}
	for _, e := range roundFirst {
		if e.killer != "" {
			fb[e.killer]++
		}
		if e.victim != "" {
			fd[e.victim]++
		}
	}

	players, ok := m["players"].(map[string]interface{})
	if !ok {
		return
	}
	all, ok := players["all_players"].([]interface{})
	if !ok {
		return
	}
	for _, pr := range all {
		p, ok := pr.(map[string]interface{})
		if !ok {
			continue
		}
		puuid, _ := p["puuid"].(string)
		p["first_bloods"] = fb[puuid]
		p["first_deaths"] = fd[puuid]
	}
}

// playersHaveFirstBloods reports whether a (stripped) cached match already carries
// the injected first_bloods field — used to decide if an older entry needs a backfill.
func playersHaveFirstBloods(m map[string]interface{}) bool {
	players, ok := m["players"].(map[string]interface{})
	if !ok {
		return false
	}
	all, ok := players["all_players"].([]interface{})
	if !ok || len(all) == 0 {
		return false
	}
	p, ok := all[0].(map[string]interface{})
	if !ok {
		return false
	}
	_, has := p["first_bloods"]
	return has
}

func StartMatchPoller(base, matchPath, apiKey string) {
	roster := []struct{ Name, Tag string }{
		{"TheMisterED", "0007"},
		{"Heri", "BLUB"},
		{"hhj", "8769"},
		{"Djibhouuuuuu", "SQL"},
		{"Graussbyt", "FRED"},
		{"Lal6s9gne", "6641"},
		{"XTrixツ", "DREAM"},
		{"小胖子vincent", "4397"},
		{"Riboox", "NG4LF"},
	}

	os.MkdirAll("./data/matches/archive", 0755)
	os.MkdirAll("./data/matches/lite", 0755)

	ticker := time.NewTicker(1 * time.Minute)

	type MatchEntry struct {
		Match      map[string]interface{} `json:"match"`
		Roster     []map[string]string    `json:"roster"`
		RrByPlayer map[string]int         `json:"rrByPlayer"`
	}

	go func() {
		for ; true; <-ticker.C {
			monthStr := time.Now().Format("2006-01")
			filePath := fmt.Sprintf("./data/matches/lite/lite_%s.json", monthStr)

			monthlyMatches := make(map[string]*MatchEntry)
			existingFile, err := os.ReadFile(filePath)
			if err == nil {
				var existing struct {
					Data []MatchEntry `json:"data"`
				}
				if err := json.Unmarshal(existingFile, &existing); err == nil {
					for i := range existing.Data {
						entry := existing.Data[i]
						if meta, ok := entry.Match["metadata"].(map[string]interface{}); ok {
							var matchID string
							if id, ok := meta["matchid"].(string); ok {
								matchID = id
							}
							if id, ok := meta["match_id"].(string); ok {
								matchID = id
							}
							if matchID != "" {
								monthlyMatches[matchID] = &entry
							}
						}
					}
				}
			}

			for _, p := range roster {
				reqURL := base + "/" + matchPath + "/eu"
				if strings.Contains(matchPath, "v4/matches") {
					reqURL += "/pc"
				}
				reqURL += "/" + url.PathEscape(p.Name) + "/" + url.PathEscape(p.Tag) + "?mode=competitive&size=15"

				req, _ := http.NewRequest("GET", reqURL, nil)
				if apiKey != "" {
					req.Header.Set("Authorization", apiKey)
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil || resp.StatusCode != 200 {
					if resp != nil {
						resp.Body.Close()
					}
					continue
				}

				var result struct {
					Data []map[string]interface{} `json:"data"`
				}
				json.NewDecoder(resp.Body).Decode(&result)
				resp.Body.Close()

				for _, m := range result.Data {
					if meta, ok := m["metadata"].(map[string]interface{}); ok {
						var matchID string
						if id, ok := meta["matchid"].(string); ok {
							matchID = id
						}
						if id, ok := meta["match_id"].(string); ok {
							matchID = id
						}

						if matchID != "" {
							archivePath := fmt.Sprintf("./data/matches/archive/%s.json", matchID)
							if _, err := os.Stat(archivePath); os.IsNotExist(err) {
								fatBytes, _ := json.Marshal(m)
								os.WriteFile(archivePath, fatBytes, 0644)
							}

							// Derive first bloods/deaths while the kill feed is still present.
							injectFirstBloods(m)

							delete(m, "rounds")
							delete(m, "kills")
							delete(m, "events")

							entry, exists := monthlyMatches[matchID]
							if !exists {
								entry = &MatchEntry{
									Match:      m,
									Roster:     make([]map[string]string, 0),
									RrByPlayer: make(map[string]int),
								}
								monthlyMatches[matchID] = entry
							} else if !playersHaveFirstBloods(entry.Match) {
								// Backfill matches cached before first bloods were tracked.
								entry.Match = m
							}

							found := false
							for _, r := range entry.Roster {
								if r["name"] == p.Name && r["tag"] == p.Tag {
									found = true
									break
								}
							}
							if !found {
								entry.Roster = append(entry.Roster, map[string]string{"name": p.Name, "tag": p.Tag})
							}
						}
					}
				}

				mmrURL := fmt.Sprintf("%s/v1/mmr-history/eu/%s/%s", base, url.PathEscape(p.Name), url.PathEscape(p.Tag))
				reqMmr, _ := http.NewRequest("GET", mmrURL, nil)
				if apiKey != "" {
					reqMmr.Header.Set("Authorization", apiKey)
				}
				respMmr, errMmr := http.DefaultClient.Do(reqMmr)
				if errMmr == nil && respMmr.StatusCode == 200 {
					var mmrResult struct {
						Data []struct {
							MatchID string `json:"match_id"`
							Change  int    `json:"mmr_change_to_last_game"`
						} `json:"data"`
					}
					if err := json.NewDecoder(respMmr.Body).Decode(&mmrResult); err == nil {
						playerKey := strings.ToLower(p.Name + "#" + p.Tag)
						for _, mmrItem := range mmrResult.Data {
							if entry, exists := monthlyMatches[mmrItem.MatchID]; exists {
								if entry.RrByPlayer == nil {
									entry.RrByPlayer = make(map[string]int)
								}
								entry.RrByPlayer[playerKey] = mmrItem.Change
							}
						}
					}
					respMmr.Body.Close()
				}
				time.Sleep(2 * time.Second)
			}

			finalData := make([]MatchEntry, 0)
			for _, entry := range monthlyMatches {
				finalData = append(finalData, *entry)
			}

			sort.Slice(finalData, func(i, j int) bool {
				metaI, _ := finalData[i].Match["metadata"].(map[string]interface{})
				metaJ, _ := finalData[j].Match["metadata"].(map[string]interface{})
				var timeI, timeJ float64
				if t, ok := metaI["game_start"].(float64); ok {
					timeI = t
				}
				if t, ok := metaJ["game_start"].(float64); ok {
					timeJ = t
				}
				if s, ok := metaI["started_at"].(string); ok {
					if p, e := time.Parse(time.RFC3339, s); e == nil {
						timeI = float64(p.Unix())
					}
				}
				if s, ok := metaJ["started_at"].(string); ok {
					if p, e := time.Parse(time.RFC3339, s); e == nil {
						timeJ = float64(p.Unix())
					}
				}
				return timeI > timeJ
			})

			if len(finalData) > 50 {
				finalData = finalData[:50]
			}

			responseObj := map[string]interface{}{"data": finalData}
			newBytes, _ := json.Marshal(responseObj)

			cacheMutex.Lock()
			cachedMatchesData = newBytes
			cacheMutex.Unlock()

			os.WriteFile(filePath, newBytes, 0644)
			log.Printf("Background Poller: Updated %s with %d total matches", filePath, len(finalData))
		}
	}()
}

func RegisterRoutes(mux *http.ServeMux, allowed []string, base, matchPath, apiKey string) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})

	mux.HandleFunc("GET /api/matches/upcoming", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Method == http.MethodOptions {
			return
		}
		data, err := os.ReadFile("data/premier/nextGames.json")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"matches": []}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	mux.HandleFunc("OPTIONS /api/matches", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("OPTIONS /api/matches/roster", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("OPTIONS /api/matches/roster/more", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/matches/roster/more", func(w http.ResponseWriter, r *http.Request) {
		handleRosterMatchesMore(w, r, allowed, base, matchPath, apiKey)
	})

	mux.HandleFunc("GET /api/matches/roster", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)

		cacheMutex.RLock()
		data := cachedMatchesData
		cacheMutex.RUnlock()

		if len(data) == 0 {
			w.Header().Set("Retry-After", "5")
			http.Error(w, `{"error": "Warming up cache, please try again in a few seconds"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	mux.HandleFunc("GET /api/matches", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		region := strings.TrimSpace(r.URL.Query().Get("region"))
		if region == "" {
			region = strings.TrimSpace(os.Getenv("VALORANT_REGION"))
		}
		if region == "" {
			region = "eu"
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		tag := strings.TrimSpace(r.URL.Query().Get("tag"))
		if region == "" || name == "" || tag == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"missing query: region, name, tag"}`))
			return
		}
		platform := strings.TrimSpace(os.Getenv("VALORANT_PLATFORM"))
		if platform == "" {
			platform = "pc"
		}
		u := base + "/" + matchPath + "/" + url.PathEscape(region)
		if strings.Contains(matchPath, "v4/matches") {
			u += "/" + url.PathEscape(platform)
		}
		u += "/" + url.PathEscape(name) + "/" + url.PathEscape(tag)
		if qs := r.URL.RawQuery; qs != "" {
			u += "?" + qs
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
		if err != nil {
			http.Error(w, `{"error":"bad upstream url"}`, http.StatusInternalServerError)
			return
		}
		if apiKey != "" {
			req.Header.Set("Authorization", apiKey)
		}
		client := &http.Client{Timeout: 25 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("upstream: %v", err)
			http.Error(w, `{"error":"upstream unreachable"}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vv := range resp.Header {
			if strings.EqualFold(k, "Content-Type") {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Printf("copy body: %v", err)
		}
	})

	mux.HandleFunc("GET /api/packs/season", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		var season string
		if err := db.DB.QueryRow("SELECT value FROM server_config WHERE key = 'current_pack_season'").Scan(&season); err != nil {
			season = "Season 1"
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"season": %q}`, season)
	})
}
