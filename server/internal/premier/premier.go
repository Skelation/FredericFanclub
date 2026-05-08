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

	// 1. Create the data/premier folder if it doesn't exist
	os.MkdirAll("./data/premier", 0755)

	ticker := time.NewTicker(10 * time.Minute)

	go func() {
		// Run immediately on boot, then wait for ticker
		for ; true; <-ticker.C {

			// Create file path organized by month (e.g. "./data/premier/2026-05.json")
			monthStr := time.Now().Format("2006-01")
			filePath := fmt.Sprintf("./data/premier/%s.json", monthStr)

			premierMatches := make(map[string]map[string]interface{})

			// 2. READ EXISTING DATA FROM DISK
			existingFile, err := os.ReadFile(filePath)
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
			if strings.Contains(matchPath, "v4/matches") {
				reqURL += "/pc"
			}
			reqURL += "/" + url.PathEscape(anchorName) + "/" + url.PathEscape(anchorTag) + "?mode=premier&size=5"

			req, _ := http.NewRequest("GET", reqURL, nil)
			if apiKey != "" { req.Header.Set("Authorization", apiKey) }
			
			resp, err := http.DefaultClient.Do(req)
			
			// Handle API Crashes
			if err != nil {
				log.Printf("❌ Premier Poller Network Error: %v", err)
				if resp != nil { resp.Body.Close() }
				continue
			}
			if resp.StatusCode != 200 {
				log.Printf("❌ Premier API Rejected! Status: %d for URL: %s", resp.StatusCode, reqURL)
				resp.Body.Close()
				continue
			}

			var result struct { Data []map[string]interface{} `json:"data"` }
			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()

			// 4. MERGE NEW MATCHES INTO EXISTING MATCHES
			// Because we use the Match ID as the dictionary key, it naturally prevents duplicates!
			for _, m := range result.Data {
				if meta, ok := m["metadata"].(map[string]interface{}); ok {
					matchID, _ := meta["matchid"].(string)
					if matchID == "" { matchID, _ = meta["match_id"].(string) }
					if matchID != "" {
						premierMatches[matchID] = m
					}
				}
			}

			// Convert our dictionary back into a clean array
			var finalData []map[string]interface{}
			for _, m := range premierMatches {
				finalData = append(finalData, m)
			}

			// 5. SORT BY DATE (Newest First)
			sort.Slice(finalData, func(i, j int) bool {
				metaI, _ := finalData[i]["metadata"].(map[string]interface{})
				metaJ, _ := finalData[j]["metadata"].(map[string]interface{})
				timeI, _ := metaI["game_start"].(float64)
				timeJ, _ := metaJ["game_start"].(float64)
				return timeI > timeJ
			})

			// 6. SAVE TO DISK AND MEMORY
			responseObj := map[string]interface{}{"data": finalData}
			newBytes, _ := json.Marshal(responseObj)

			// Write to the file
			os.WriteFile(filePath, newBytes, 0644)

			// Safely write to our global memory cache for the frontend
			premierMutex.Lock()
			cachedPremierData = newBytes
			premierMutex.Unlock()

			log.Printf("Premier Poller: Updated %s with %d total matches", filePath, len(finalData))
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
