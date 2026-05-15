package premier

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	cachedPremierData []byte
	premierMutex      sync.RWMutex
)

// StartPremierPoller runs in the background to fetch Premier matches and save them to disk
func StartPremierPoller(base, matchPath, apiKey string) {
	rosterToCheck := []struct {
		Name string
		Tag  string
	}{
		{"Heri", "BLUB"},
		{"TheMisterED", "0007"},
		{"Graussbyt", "5629"},
	}

	// Create our new Two-Tier folder structure
	os.MkdirAll("./data/premier/archive", 0755) // THE VAULT (Fat files)
	os.MkdirAll("./data/premier/lite", 0755)    // THE CACHE (UI files)

	ticker := time.NewTicker(10 * time.Minute)

	go func() {
		for ; true; <-ticker.C {

			// We now read from the ARCHIVE to see what we already have downloaded
			premierMatches := make(map[string]map[string]interface{})

			files, err := os.ReadDir("./data/premier/archive")
			if err == nil {
				for _, file := range files {
					if !strings.HasSuffix(file.Name(), ".json") {
						continue
					}
					data, err := os.ReadFile("./data/premier/archive/" + file.Name())
					if err == nil {
						var m map[string]interface{}
						if err := json.Unmarshal(data, &m); err == nil {
							meta, ok := m["metadata"].(map[string]interface{})
							if ok {
								matchID, _ := meta["matchid"].(string)
								if matchID == "" {
									matchID, _ = meta["match_id"].(string)
								}
								if matchID != "" {
									premierMatches[matchID] = m
								}
							}
						}
					}
				}
			}

			// 2. FETCH NEW DATA FROM RIOT API FOR EVERY PLAYER IN THE ROSTER
			for _, player := range rosterToCheck {
				reqURL := base + "/" + matchPath + "/eu"
				if strings.Contains(matchPath, "v4/matches") {
					reqURL += "/pc"
				}
				reqURL += "/" + url.PathEscape(player.Name) + "/" + url.PathEscape(player.Tag) + "?mode=premier&size=5"

				req, _ := http.NewRequest("GET", reqURL, nil)
				if apiKey != "" {
					req.Header.Set("Authorization", apiKey)
				}

				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					log.Printf("❌ Premier Poller Network Error for %s: %v", player.Name, err)
					if resp != nil {
						resp.Body.Close()
					}
					continue // Skip to the next player
				}

				var result struct {
					Data []map[string]interface{} `json:"data"`
				}

				if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && len(result.Data) > 0 {
					for _, m := range result.Data {
						meta, ok := m["metadata"].(map[string]interface{})
						if !ok { continue }

						matchID, _ := meta["matchid"].(string)
						if matchID == "" { matchID, _ = meta["match_id"].(string) }

						if matchID != "" && premierMatches[matchID] == nil {
							// Save the FAT file to the archive immediately
							archivePath := fmt.Sprintf("./data/premier/archive/%s.json", matchID)
							fileBytes, _ := json.MarshalIndent(m, "", "  ")
							os.WriteFile(archivePath, fileBytes, 0644)
							
							// Add it to our working map
							premierMatches[matchID] = m
						}
					}
				}
				resp.Body.Close()
			} // End of roster loop

			// 4. BUILD THE LITE VERSIONS FOR THE UI
			var finalData []map[string]interface{}
			for matchID, originalMatch := range premierMatches {
				// We need to deep copy so we don't accidentally delete data from the original map in memory
				mBytes, _ := json.Marshal(originalMatch)
				var liteMatch map[string]interface{}
				json.Unmarshal(mBytes, &liteMatch)

				// Strip the heavy stuff
				delete(liteMatch, "rounds")
				delete(liteMatch, "kills")
				delete(liteMatch, "events")

				// Ensure it has its ID for the UI
				liteMatch["match_id"] = matchID
				finalData = append(finalData, liteMatch)
			}

			// Sort by time descending (newest first)
			sort.Slice(finalData, func(i, j int) bool {
				metaI, okI := finalData[i]["metadata"].(map[string]interface{})
				metaJ, okJ := finalData[j]["metadata"].(map[string]interface{})
				if !okI || !okJ { return false }
				
				startI, _ := metaI["game_start"].(float64)
				startJ, _ := metaJ["game_start"].(float64)
				return startI > startJ
			})

			// Keep memory clean: Only hold the last 100 matches in RAM
			if len(finalData) > 100 {
				finalData = finalData[:100]
			}

			// 5. SAVE AND EXPOSE THE DATA
			liteFilePath := "./data/premier/lite/matches.json"
			responseWrapper := map[string]interface{}{
				"data": finalData,
			}
			newBytes, _ := json.MarshalIndent(responseWrapper, "", "  ")
			os.WriteFile(liteFilePath, newBytes, 0644)

			// Update the live website cache
			premierMutex.Lock()
			cachedPremierData = newBytes
			premierMutex.Unlock()

			log.Printf("Premier Poller: Tracked %d matches (Raw files secured in Vault)", len(finalData))

			// 6. CALCULATE ADVANCED DASHBOARD STATS
			team := []string{"Heri#BLUB", "TheMisterED#0007", "Graussbyt#5629", "hhj#8769", "Djibはコリーヌ お あいして#LoVe", "Lal6s9gne#6641", "Riboox"}
			GenerateTeamStats(team)
		}
	}()
}

// RegisterPremierRoutes exposes the cached Premier matches to your frontend
func RegisterPremierRoutes(mux *http.ServeMux, allowed []string, applyCORS func(w http.ResponseWriter, r *http.Request, allowed []string)) {
	mux.HandleFunc("OPTIONS /api/matches/premier", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/matches/premier", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)

		premierMutex.RLock()
		data := cachedPremierData
		premierMutex.RUnlock()

		if len(data) == 0 {
			w.Header().Set("Retry-After", "10")
			http.Error(w, `{"error": "Premier cache warming up, check back soon."}`, http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	mux.HandleFunc("OPTIONS /api/matches/stats", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/matches/stats", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		statsFilePath := "./data/premier/dashboard_stats.json"

		data, err := os.ReadFile(statsFilePath)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "Dashboard stats not generated yet. Please wait for the poller."}`))
			return
		}

		w.Write(data)
	})
}
