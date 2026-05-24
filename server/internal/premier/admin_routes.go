package premier

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"fredericfanclub/server/internal/middleware"
)

var configMutex sync.Mutex

func premierAdminToken() string {
	return strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN"))
}

func checkAdmin(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("X-Admin-Token") != premierAdminToken() {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return false
	}
	return true
}

// MatchSummary is the shape returned to the admin UI for match selection.
type MatchSummary struct {
	MatchID   string        `json:"match_id"`
	Date      string        `json:"date"`
	GameStart int64         `json:"game_start"`
	Map       string        `json:"map"`
	Players   []PlayerEntry `json:"players"`
}

type PlayerEntry struct {
	Full string `json:"full"` // "Name#Tag"
}

// --- Alias file helpers ---

func readAliasFile() ([]AliasEntry, error) {
	data, err := os.ReadFile("./data/premier/aliases.json")
	if err != nil {
		if os.IsNotExist(err) {
			return []AliasEntry{}, nil
		}
		return nil, err
	}
	var entries []AliasEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return []AliasEntry{}, nil
	}
	return entries, nil
}

func writeAliasFile(entries []AliasEntry) error {
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll("./data/premier", 0755)
	return os.WriteFile("./data/premier/aliases.json", b, 0644)
}

// --- Reassignment file helpers ---

func readReassignmentFile() ([]ReassignmentEntry, error) {
	data, err := os.ReadFile("./data/premier/reassignments.json")
	if err != nil {
		if os.IsNotExist(err) {
			return []ReassignmentEntry{}, nil
		}
		return nil, err
	}
	var entries []ReassignmentEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return []ReassignmentEntry{}, nil
	}
	return entries, nil
}

func writeReassignmentFile(entries []ReassignmentEntry) error {
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	os.MkdirAll("./data/premier", 0755)
	return os.WriteFile("./data/premier/reassignments.json", b, 0644)
}

// RegisterAdminRoutes wires up all /api/admin/premier/* endpoints.
func RegisterAdminRoutes(mux *http.ServeMux, allowed []string) {
	corsOpts := func(path string) {
		mux.HandleFunc("OPTIONS "+path, func(w http.ResponseWriter, r *http.Request) {
			middleware.ApplyCORS(w, r, allowed)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
			w.WriteHeader(http.StatusNoContent)
		})
	}

	// ── List archived matches for the admin UI picker ──────────────────────
	corsOpts("/api/admin/premier/matches")
	mux.HandleFunc("GET /api/admin/premier/matches", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if !checkAdmin(w, r) {
			return
		}

		files, err := os.ReadDir("./data/premier/archive")
		if err != nil {
			http.Error(w, `{"error":"cannot read archive"}`, http.StatusInternalServerError)
			return
		}

		var summaries []MatchSummary
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			matchID := strings.TrimSuffix(f.Name(), ".json")
			data, err := os.ReadFile("./data/premier/archive/" + f.Name())
			if err != nil {
				continue
			}
			var m MatchData
			if err := json.Unmarshal(data, &m); err != nil {
				continue
			}
			var players []PlayerEntry
			seen := map[string]bool{}
			for _, p := range m.Players.AllPlayers {
				full := fmt.Sprintf("%s#%s", p.Name, p.Tag)
				if !seen[full] {
					players = append(players, PlayerEntry{Full: full})
					seen[full] = true
				}
			}
			date := time.Unix(m.Metadata.GameStart, 0).UTC().Format("2006-01-02 15:04")
			summaries = append(summaries, MatchSummary{
				MatchID:   matchID,
				Date:      date,
				GameStart: m.Metadata.GameStart,
				Map:       m.Metadata.Map,
				Players:   players,
			})
		}

		sort.Slice(summaries, func(i, j int) bool {
			return summaries[i].GameStart > summaries[j].GameStart
		})

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(summaries)
	})

	// ── Aliases ─────────────────────────────────────────────────────────────
	corsOpts("/api/admin/premier/aliases")

	mux.HandleFunc("GET /api/admin/premier/aliases", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if !checkAdmin(w, r) {
			return
		}
		configMutex.Lock()
		entries, _ := readAliasFile()
		configMutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	})

	mux.HandleFunc("POST /api/admin/premier/aliases", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if !checkAdmin(w, r) {
			return
		}
		var req AliasEntry
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Ingame == "" || req.Player == "" {
			http.Error(w, `{"error":"ingame and player are required"}`, http.StatusBadRequest)
			return
		}
		req.Ingame = strings.TrimSpace(req.Ingame)
		req.Player = strings.TrimSpace(req.Player)

		configMutex.Lock()
		defer configMutex.Unlock()

		entries, _ := readAliasFile()
		// Upsert: replace existing entry for same ingame name
		updated := false
		for i, e := range entries {
			if e.Ingame == req.Ingame {
				entries[i].Player = req.Player
				updated = true
				break
			}
		}
		if !updated {
			entries = append(entries, req)
		}
		if err := writeAliasFile(entries); err != nil {
			http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
			return
		}
		go GenerateTeamStats(teamRoster)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"message":"Alias saved. Stats refreshing in background."}`)
	})

	mux.HandleFunc("DELETE /api/admin/premier/aliases", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if !checkAdmin(w, r) {
			return
		}
		var req struct {
			Ingame string `json:"ingame"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Ingame == "" {
			http.Error(w, `{"error":"ingame is required"}`, http.StatusBadRequest)
			return
		}

		configMutex.Lock()
		defer configMutex.Unlock()

		entries, _ := readAliasFile()
		filtered := entries[:0]
		for _, e := range entries {
			if e.Ingame != req.Ingame {
				filtered = append(filtered, e)
			}
		}
		if err := writeAliasFile(filtered); err != nil {
			http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
			return
		}
		go GenerateTeamStats(teamRoster)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"message":"Alias removed. Stats refreshing in background."}`)
	})

	// ── Reassignments ────────────────────────────────────────────────────────
	corsOpts("/api/admin/premier/reassignments")

	mux.HandleFunc("GET /api/admin/premier/reassignments", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if !checkAdmin(w, r) {
			return
		}
		configMutex.Lock()
		entries, _ := readReassignmentFile()
		configMutex.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	})

	mux.HandleFunc("POST /api/admin/premier/reassignments", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if !checkAdmin(w, r) {
			return
		}
		var req ReassignmentEntry
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MatchID == "" || req.Ingame == "" || req.Player == "" {
			http.Error(w, `{"error":"match_id, ingame and player are required"}`, http.StatusBadRequest)
			return
		}
		req.Ingame = strings.TrimSpace(req.Ingame)
		req.Player = strings.TrimSpace(req.Player)

		configMutex.Lock()
		defer configMutex.Unlock()

		entries, _ := readReassignmentFile()
		// Upsert: replace existing entry for same match+ingame pair
		updated := false
		for i, e := range entries {
			if e.MatchID == req.MatchID && e.Ingame == req.Ingame {
				entries[i].Player = req.Player
				updated = true
				break
			}
		}
		if !updated {
			entries = append(entries, req)
		}
		if err := writeReassignmentFile(entries); err != nil {
			http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
			return
		}
		go GenerateTeamStats(teamRoster)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"message":"Reassignment saved. Stats refreshing in background."}`)
	})

	mux.HandleFunc("DELETE /api/admin/premier/reassignments", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if !checkAdmin(w, r) {
			return
		}
		var req struct {
			MatchID string `json:"match_id"`
			Ingame  string `json:"ingame"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MatchID == "" || req.Ingame == "" {
			http.Error(w, `{"error":"match_id and ingame are required"}`, http.StatusBadRequest)
			return
		}

		configMutex.Lock()
		defer configMutex.Unlock()

		entries, _ := readReassignmentFile()
		filtered := entries[:0]
		for _, e := range entries {
			if !(e.MatchID == req.MatchID && e.Ingame == req.Ingame) {
				filtered = append(filtered, e)
			}
		}
		if err := writeReassignmentFile(filtered); err != nil {
			http.Error(w, `{"error":"failed to save"}`, http.StatusInternalServerError)
			return
		}
		go GenerateTeamStats(teamRoster)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"message":"Reassignment removed. Stats refreshing in background."}`)
	})

	// ── Team roster list (for admin autocomplete) ────────────────────────
	corsOpts("/api/admin/premier/roster")
	mux.HandleFunc("GET /api/admin/premier/roster", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if !checkAdmin(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(teamRoster)
	})

	// ── Manual stats refresh ──────────────────────────────────────────────
	corsOpts("/api/admin/premier/refresh-stats")
	mux.HandleFunc("POST /api/admin/premier/refresh-stats", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if !checkAdmin(w, r) {
			return
		}
		go GenerateTeamStats(teamRoster)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success":true,"message":"Stats regeneration started."}`)
	})
}
