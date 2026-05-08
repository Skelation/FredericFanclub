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
	// THE ANCHOR PLAYER: The server only checks this person to find team matches
	anchorName := "Heri"
	anchorTag := "BLUB"

	// 1. Create our new Two-Tier folder structure
	os.MkdirAll("./data/premier/archive", 0755) // THE VAULT (Fat files)
	os.MkdirAll("./data/premier/lite", 0755)    // THE CACHE (UI files)

	ticker := time.NewTicker(10 * time.Minute)

	go func() {
		for ; true; <-ticker.C {

			// We now read from the LITE file so we don't blow up our RAM on boot
			monthStr := time.Now().Format("2006-01")
			liteFilePath := fmt.Sprintf("./data/premier/lite/lite_%s.json", monthStr)

			premierMatches := make(map[string]map[string]interface{})

			// 2. READ EXISTING *LITE* DATA FROM DISK
			existingFile, err := os.ReadFile(liteFilePath)
			if err == nil {
				var existing struct { Data []map[string]interface{} `json:"data"` }
				if err := json.Unmarshal(existingFile, &existing); err == nil {
					for _, m := range existing.Data {
						if meta, ok := m["metadata"].(map[string]interface{}); ok {
							matchID, _ := meta["matchid"].(string)
							if matchID == "" { matchID, _ = meta["match_id"].(string) }
							if matchID != "" {
								premierMatches[matchID] = m
							}
						}
					}
				}
			}

			// 3. FETCH NEW DATA FROM RIOT API
			reqURL := base + "/" + matchPath + "/eu"
			if strings.Contains(matchPath, "v4/matches") { reqURL += "/pc" }
			reqURL += "/" + url.PathEscape(anchorName) + "/" + url.PathEscape(anchorTag) + "?mode=premier&size=5"

			req, _ := http.NewRequest("GET", reqURL, nil)
			if apiKey != "" { req.Header.Set("Authorization", apiKey) }
			
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("❌ Premier Poller Network Error: %v", err)
				if resp != nil { resp.Body.Close() }
				continue
			}
			if resp.StatusCode != 200 {
				log.Printf("❌ Premier API Rejected! Status: %d", resp.StatusCode)
				resp.Body.Close()
				continue
			}

			var result struct { Data []map[string]interface{} `json:"data"` }
			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()

			// 4. THE TWO-TIER MERGE LOGIC
			for _, m := range result.Data {
				if meta, ok := m["metadata"].(map[string]interface{}); ok {
					matchID, _ := meta["matchid"].(string)
					if matchID == "" { matchID, _ = meta["match_id"].(string) }
					if matchID != "" {
						
						// --- TIER 1: THE VAULT (Save full raw data permanently) ---
						archivePath := fmt.Sprintf("./data/premier/archive/%s.json", matchID)
						// Only write it to disk if we haven't saved this match before
						if _, err := os.Stat(archivePath); os.IsNotExist(err) {
							fatBytes, _ := json.Marshal(m)
							os.WriteFile(archivePath, fatBytes, 0644)
						}

						// --- TIER 2: THE CACHE (Strip it down for the RAM and Frontend) ---
						delete(m, "rounds")
						delete(m, "kills")
						delete(m, "events")

						// Add the lightweight version to our memory tracker
						premierMatches[matchID] = m
					}
				}
			}

			// Convert memory map back into a clean array
			var finalData []map[string]interface{}
			for _, m := range premierMatches { finalData = append(finalData, m) }

			// 5. SORT BY DATE (Newest First)
			sort.Slice(finalData, func(i, j int) bool {
				metaI, _ := finalData[i]["metadata"].(map[string]interface{})
				metaJ, _ := finalData[j]["metadata"].(map[string]interface{})
				timeI, _ := metaI["game_start"].(float64)
				timeJ, _ := metaJ["game_start"].(float64)
				return timeI > timeJ
			})

			// Keep memory clean: Only hold the last 100 matches in RAM
			if len(finalData) > 30{
				finalData = finalData[:30]
			}

			// 6. SAVE LITE VERSION TO DISK AND MEMORY
			responseObj := map[string]interface{}{"data": finalData}
			newBytes, _ := json.Marshal(responseObj)

			// Write the lightweight file so the server boots fast next time
			os.WriteFile(liteFilePath, newBytes, 0644)

			// Update the live website cache
			premierMutex.Lock()
			cachedPremierData = newBytes
			premierMutex.Unlock()

			log.Printf("Premier Poller: Tracked %d matches (Raw files secured in Vault)", len(finalData))
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
}
