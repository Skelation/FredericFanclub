package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"fredericfanclub/server/discordbot"
	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/hub"
	"fredericfanclub/server/internal/matches"
	"fredericfanclub/server/internal/middleware"
	"fredericfanclub/server/internal/premier"
)

// namePart returns the "Name" portion of a "Name#Tag" identity.
func namePart(s string) string {
	if i := strings.Index(s, "#"); i >= 0 {
		return s[:i]
	}
	return s
}

// resolvePlayerName applies the premier alias map to an in-game "Name#Tag" and
// reports whether the (aliased) identity refers to the requested target player.
// target may be a full "Name#Tag" or just a "Name"; matching is case-insensitive
// and tolerant of either form. This mirrors how stats generation resolves
// players, so a renamed player with an alias entry keeps working for betting.
func resolvePlayerName(aliasMap map[string]string, name, tag, target string) bool {
	full := name + "#" + tag
	if canon, ok := aliasMap[full]; ok {
		full = canon
	}
	if strings.EqualFold(full, target) {
		return true
	}
	return strings.EqualFold(namePart(full), namePart(target))
}

var fileMutex sync.Mutex

type UpcomingMatch struct {
	Opponent string `json:"opponent"`
	Tag      string `json:"tag"`
	Time     string `json:"time"`
	Format   string `json:"format"`
	Maps     string `json:"maps"`
}

type NextGamesContainer struct {
	Matches []UpcomingMatch `json:"matches"`
}

func adminToken() string {
	return strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN"))
}

func RegisterRoutes(mux *http.ServeMux, allowed []string, base, apiKey string) {
	// Schedule upcoming match
	mux.HandleFunc("/api/admin/schedule-match", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Method == http.MethodOptions {
			return
		}
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var newMatch UpcomingMatch
		if err := json.NewDecoder(r.Body).Decode(&newMatch); err != nil {
			http.Error(w, `{"error": "Invalid request body"}`, http.StatusBadRequest)
			return
		}

		fileMutex.Lock()
		defer fileMutex.Unlock()

		filePath := "data/premier/nextGames.json"
		var container NextGamesContainer
		if fileData, err := os.ReadFile(filePath); err == nil {
			json.Unmarshal(fileData, &container)
		}
		container.Matches = append(container.Matches, newMatch)

		updatedData, err := json.MarshalIndent(container, "", "  ")
		if err != nil {
			http.Error(w, `{"error": "Failed to process data"}`, http.StatusInternalServerError)
			return
		}
		if err = os.WriteFile(filePath, updatedData, 0644); err != nil {
			http.Error(w, `{"error": "Failed to save to disk"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message": "Match scheduled successfully"}`))
	})

	// Create redeem code
	mux.HandleFunc("OPTIONS /api/admin/create-code", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/create-code", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			Code      string `json:"code"`
			RewardFT  int    `json:"reward_ft"`
			MaxUses   int    `json:"max_uses"`
			ExpiresAt string `json:"expires_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" || req.RewardFT <= 0 {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		if req.MaxUses <= 0 {
			req.MaxUses = 99
		}
		if req.ExpiresAt == "" {
			req.ExpiresAt = time.Now().UTC().AddDate(0, 0, 7).Format("2006-01-02T15:04:05Z")
		}
		code := strings.ToUpper(strings.TrimSpace(req.Code))
		_, err := db.DB.Exec(
			"INSERT INTO redeem_codes (code, reward_ft, max_uses, uses_so_far, expires_at) VALUES (?, ?, ?, 0, ?)",
			code, req.RewardFT, req.MaxUses, req.ExpiresAt)
		if err != nil {
			http.Error(w, `{"error": "code already exists or db error"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "code": "%s", "reward_ft": %d, "max_uses": %d}`,
			code, req.RewardFT, req.MaxUses)
	})

	// Add card
	mux.HandleFunc("OPTIONS /api/admin/cards", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/cards", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			Name     string `json:"name"`
			Rarity   string `json:"rarity"`
			ImageURL string `json:"image_url"`
			Season   string `json:"season"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		if req.Season == "" {
			req.Season = "Season 1"
		}
		result, err := db.DB.Exec(
			"INSERT INTO cards (name, rarity, image_url, season) VALUES (?, ?, ?, ?)",
			req.Name, req.Rarity, req.ImageURL, req.Season)
		if err != nil {
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}
		newID, _ := result.LastInsertId()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Card added!", "card_id": %d}`, newID)
	})

	// Add cosmetic (banner / title) — e.g. register a PNG profile background
	// as a purchasable banner. Price is derived from rarity in the shop.
	mux.HandleFunc("OPTIONS /api/admin/cosmetics", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/cosmetics", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			Type        string `json:"type"`        // 'banner' | 'title'
			Name        string `json:"name"`
			Value       string `json:"value"`       // banner: image path (/images/banners/x.png) or CSS gradient; title: display text
			Rarity      string `json:"rarity"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		req.Type = strings.ToLower(strings.TrimSpace(req.Type))
		req.Name = strings.TrimSpace(req.Name)
		req.Value = strings.TrimSpace(req.Value)
		if req.Type != "banner" && req.Type != "title" {
			http.Error(w, `{"error": "type must be 'banner' or 'title'"}`, http.StatusBadRequest)
			return
		}
		if req.Name == "" || req.Value == "" {
			http.Error(w, `{"error": "name and value are required"}`, http.StatusBadRequest)
			return
		}
		if req.Rarity == "" {
			req.Rarity = "bronze"
		}
		result, err := db.DB.Exec(
			"INSERT INTO cosmetics (type, name, value, rarity, description) VALUES (?, ?, ?, ?, ?)",
			req.Type, req.Name, req.Value, req.Rarity, req.Description)
		if err != nil {
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}
		newID, _ := result.LastInsertId()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "cosmetic_id": %d, "price": %d}`, newID, db.CosmeticPrice(req.Rarity))
	})

	// List cosmetics (banners + titles) for management
	mux.HandleFunc("GET /api/admin/cosmetics", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		rows, err := db.DB.Query(`SELECT id, type, name, value, COALESCE(rarity,'bronze'), COALESCE(description,'')
			FROM cosmetics WHERE type IN ('banner', 'title') ORDER BY type, id`)
		if err != nil {
			http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type Cosmetic struct {
			ID          int    `json:"id"`
			Type        string `json:"type"`
			Name        string `json:"name"`
			Value       string `json:"value"`
			Rarity      string `json:"rarity"`
			Description string `json:"description"`
			Price       int    `json:"price"`
		}
		out := make([]Cosmetic, 0)
		for rows.Next() {
			var c Cosmetic
			rows.Scan(&c.ID, &c.Type, &c.Name, &c.Value, &c.Rarity, &c.Description)
			c.Price = db.CosmeticPrice(c.Rarity)
			out = append(out, c)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})

	// Update a cosmetic (banner / title)
	mux.HandleFunc("OPTIONS /api/admin/update-cosmetic", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/update-cosmetic", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Value       string `json:"value"`
			Rarity      string `json:"rarity"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == 0 {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		req.Value = strings.TrimSpace(req.Value)
		if req.Name == "" || req.Value == "" {
			http.Error(w, `{"error": "name and value are required"}`, http.StatusBadRequest)
			return
		}
		if req.Rarity == "" {
			req.Rarity = "bronze"
		}
		res, err := db.DB.Exec(
			"UPDATE cosmetics SET name = ?, value = ?, rarity = ?, description = ? WHERE id = ?",
			req.Name, req.Value, req.Rarity, req.Description, req.ID)
		if err != nil {
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			http.Error(w, `{"error": "cosmetic not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "price": %d}`, db.CosmeticPrice(req.Rarity))
	})

	// Delete a cosmetic (banner / title) — also un-equips it and removes
	// ownership rows so nobody is left pointing at a deleted item.
	mux.HandleFunc("OPTIONS /api/admin/delete-cosmetic", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/delete-cosmetic", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			ID int `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == 0 {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		tx, err := db.DB.Begin()
		if err != nil {
			http.Error(w, `{"error": "server error"}`, http.StatusInternalServerError)
			return
		}
		tx.Exec("UPDATE user_profile SET banner_id = NULL WHERE banner_id = ?", req.ID)
		tx.Exec("UPDATE user_profile SET title_id = NULL WHERE title_id = ?", req.ID)
		tx.Exec("DELETE FROM user_cosmetics WHERE cosmetic_id = ?", req.ID)
		if _, err := tx.Exec("DELETE FROM cosmetics WHERE id = ?", req.ID); err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "db error on cosmetics"}`, http.StatusInternalServerError)
			return
		}
		tx.Commit()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Cosmetic deleted and un-equipped."}`)
	})

	// Delete card
	mux.HandleFunc("OPTIONS /api/admin/delete-card", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/delete-card", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct{ CardID int `json:"card_id"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		tx, _ := db.DB.Begin()
		_, err := tx.Exec("DELETE FROM inventory WHERE card_id = ?", req.CardID)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "db error on inventory"}`, 500)
			return
		}
		_, err = tx.Exec("DELETE FROM cards WHERE id = ?", req.CardID)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "db error on cards"}`, 500)
			return
		}
		tx.Commit()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Card permanently deleted!"}`)
	})

	// Give tokens
	mux.HandleFunc("OPTIONS /api/admin/tokens", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/tokens", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			DiscordID string `json:"discord_id"`
			Amount    int    `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		result, err := db.DB.Exec(
			"UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", req.Amount, req.DiscordID)
		if err != nil {
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Successfully minted %d FT for user %s"}`,
			req.Amount, req.DiscordID)
	})

	// Preview prop
	mux.HandleFunc("OPTIONS /api/admin/preview-prop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/preview-prop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct {
			Player   string `json:"player"`
			PropType string `json:"prop_type"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		// Resolve players the same way premier stats generation does: by
		// (aliased) "Name#Tag" rather than a live account-API PUUID lookup.
		// This keeps renamed players working as long as an alias entry exists,
		// instead of breaking when their old Riot ID stops resolving.
		aliasMap := premier.PublicAliasMap()

		dataBytes := matches.GetCachedData()
		if len(dataBytes) == 0 {
			http.Error(w, `{"error": "Match cache is empty. Wait a minute."}`, http.StatusBadRequest)
			return
		}

		var cacheData struct {
			Data []struct {
				Match map[string]interface{} `json:"match"`
			} `json:"data"`
		}
		json.Unmarshal(dataBytes, &cacheData)

		var statsHistory []float64
		totalMatches := 0.0
		wins := 0.0

		for _, m := range cacheData.Data {
			var allPlayers []interface{}
			if pMap, ok := m.Match["players"].(map[string]interface{}); ok {
				allPlayers, _ = pMap["all_players"].([]interface{})
			} else if pArr, ok := m.Match["players"].([]interface{}); ok {
				allPlayers = pArr
			}

			// Total rounds played in this match (needed for per-round stats
			// like ADR). The cache strips the `rounds` array, so derive it from
			// the team scoreboard instead.
			matchRounds := 0.0
			if teamsMap, ok := m.Match["teams"].(map[string]interface{}); ok {
				for _, t := range teamsMap {
					if td, ok := t.(map[string]interface{}); ok {
						rw, _ := td["rounds_won"].(float64)
						rl, _ := td["rounds_lost"].(float64)
						if rw+rl > matchRounds {
							matchRounds = rw + rl
						}
					}
				}
			} else if teamsArr, ok := m.Match["teams"].([]interface{}); ok {
				for _, t := range teamsArr {
					td, ok := t.(map[string]interface{})
					if !ok {
						continue
					}
					rw, _ := td["rounds_won"].(float64)
					rl, _ := td["rounds_lost"].(float64)
					if r, ok := td["rounds"].(map[string]interface{}); ok {
						rw, _ = r["won"].(float64)
						rl, _ = r["lost"].(float64)
					}
					if rw+rl > matchRounds {
						matchRounds = rw + rl
					}
				}
			}

			for _, p := range allPlayers {
				playerMap, ok := p.(map[string]interface{})
				if !ok {
					continue
				}
				name, _ := playerMap["name"].(string)
				tag, _ := playerMap["tag"].(string)
				if !resolvePlayerName(aliasMap, name, tag, req.Player) {
					continue
				}

				if req.PropType == "match_result" {
					totalMatches++
					teamName, _ := playerMap["team"].(string)
					if teamName == "" {
						teamName, _ = playerMap["team_id"].(string)
					}
					if teamsMap, ok := m.Match["teams"].(map[string]interface{}); ok {
						if teamData, ok := teamsMap[strings.ToLower(teamName)].(map[string]interface{}); ok {
							if won, _ := teamData["has_won"].(bool); won {
								wins++
							}
						}
					} else if teamsArr, ok := m.Match["teams"].([]interface{}); ok {
						for _, t := range teamsArr {
							tData, _ := t.(map[string]interface{})
							tID, _ := tData["team_id"].(string)
							if strings.EqualFold(tID, teamName) {
								if won, _ := tData["won"].(bool); won {
									wins++
								}
							}
						}
					}
				} else if req.PropType == "adr" {
					// Average Damage per Round = total damage / rounds played.
					if matchRounds > 0 {
						if dmg, ok := playerMap["damage_made"].(float64); ok {
							statsHistory = append(statsHistory, dmg/matchRounds)
						}
					}
				} else {
					stats, ok := playerMap["stats"].(map[string]interface{})
					if !ok {
						break
					}
					if req.PropType == "kd_ratio" {
						kills, ok1 := stats["kills"].(float64)
						deaths, ok2 := stats["deaths"].(float64)
						if ok1 && ok2 {
							if deaths == 0 {
								deaths = 1
							}
							statsHistory = append(statsHistory, kills/deaths)
						}
					} else {
						if val, ok := stats[req.PropType].(float64); ok {
							statsHistory = append(statsHistory, val)
						}
					}
				}
				break
			}
		}

		var overProb, underProb, line float64
		if req.PropType == "match_result" {
			if totalMatches == 0 {
				http.Error(w, `{"error": "Could not find recent matches for this player."}`, http.StatusBadRequest)
				return
			}
			overProb = wins / totalMatches
			underProb = 1.0 - overProb
		} else {
			if len(statsHistory) == 0 {
				http.Error(w, `{"error": "Could not find recent stats for this player."}`, http.StatusBadRequest)
				return
			}
			total := 0.0
			for _, val := range statsHistory {
				total += val
			}
			average := total / float64(len(statsHistory))
			if req.PropType == "kd_ratio" {
				line = float64(int(average*10))/10 + 0.05
			} else {
				line = float64(int(average)) + 0.5
			}
			overCount := 0.0
			for _, val := range statsHistory {
				if val > line {
					overCount++
				}
			}
			overProb = overCount / float64(len(statsHistory))
			underProb = 1.0 - overProb
		}

		if overProb < 0.15 {
			overProb = 0.15
		}
		if overProb > 0.85 {
			overProb = 0.85
		}
		if underProb < 0.15 {
			underProb = 0.15
		}
		if underProb > 0.85 {
			underProb = 0.85
		}

		preview := hub.PropMarket{
			Player: req.Player, PropType: req.PropType, Line: line,
			OverMult: (1.0 / overProb) * 0.90, UnderMult: (1.0 / underProb) * 0.90, IsOpen: false,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(preview)
	})

	// Link user to roster player
	mux.HandleFunc("OPTIONS /api/admin/link-user", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/link-user", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			DiscordID string `json:"discord_id"`
			Player    string `json:"player"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		_, err := db.DB.Exec("UPDATE users SET linked_player = ? WHERE discord_id = ?", req.Player, req.DiscordID)
		if err != nil {
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "User linked successfully!"}`)
	})

	// List all users
	mux.HandleFunc("OPTIONS /api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		rows, err := db.DB.Query("SELECT discord_id, username, linked_player FROM users")
		if err != nil {
			http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type UserObj struct{ DiscordID, Username, Linked string }
		users := make([]UserObj, 0)
		for rows.Next() {
			var u UserObj
			rows.Scan(&u.DiscordID, &u.Username, &u.Linked)
			users = append(users, u)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
	})

	// Publish prop
	mux.HandleFunc("OPTIONS /api/admin/publish-prop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/publish-prop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var marketToPublish hub.PropMarket
		json.NewDecoder(r.Body).Decode(&marketToPublish)
		marketToPublish.IsOpen = true
		hub.CurrentMarket = &marketToPublish
		hub.Broadcast <- hub.WSMessage{Type: "market_published"}
		if !hub.DevMode {
			go discordbot.SendMarketPublishedNotification(
				marketToPublish.Player, marketToPublish.PropType,
				marketToPublish.Line, marketToPublish.OverMult, marketToPublish.UnderMult)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Market is now LIVE!"}`)
	})

	// Cancel market and mass-refund
	mux.HandleFunc("OPTIONS /api/admin/cancel-market", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/cancel-market", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if hub.CurrentMarket == nil {
			http.Error(w, `{"error": "No active market to cancel."}`, http.StatusBadRequest)
			return
		}
		tx, err := db.DB.Begin()
		if err != nil {
			http.Error(w, `{"error": "Server error"}`, http.StatusInternalServerError)
			return
		}
		rows, err := tx.Query("SELECT id, discord_id, amount FROM bets WHERE status = 'pending' AND bet_category = 'prop'")
		if err == nil {
			type Bet struct {
				ID      int
				Discord string
				Amount  float64
			}
			var bets []Bet
			for rows.Next() {
				var b Bet
				rows.Scan(&b.ID, &b.Discord, &b.Amount)
				bets = append(bets, b)
			}
			rows.Close()
			for _, b := range bets {
				tx.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", b.Amount, b.Discord)
				tx.Exec("UPDATE bets SET status = 'cancelled' WHERE id = ?", b.ID)
			}
		}
		hub.CurrentMarket = nil
		tx.Commit()
		hub.Broadcast <- hub.WSMessage{Type: "market_cancelled"}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Market Aborted! All tokens refunded."}`)
	})

	// Lock market
	mux.HandleFunc("OPTIONS /api/admin/lock-prop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/lock-prop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if hub.CurrentMarket != nil {
			hub.CurrentMarket.IsOpen = false
		}
		hub.Broadcast <- hub.WSMessage{Type: "market_locked"}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Market Locked! No more bets allowed."}`)
	})

	// Resolve market and pay out winners
	mux.HandleFunc("OPTIONS /api/admin/resolve-prop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/resolve-prop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct{ Outcome string `json:"outcome"` }
		json.NewDecoder(r.Body).Decode(&req)
		if req.Outcome != "over" && req.Outcome != "under" {
			http.Error(w, `{"error": "invalid outcome"}`, http.StatusBadRequest)
			return
		}
		if hub.CurrentMarket == nil {
			http.Error(w, `{"error": "No active market to resolve"}`, http.StatusBadRequest)
			return
		}

		marketPlayer := hub.CurrentMarket.Player
		marketProp := hub.CurrentMarket.PropType

		tx, err := db.DB.Begin()
		if err != nil {
			http.Error(w, `{"error": "server error"}`, http.StatusInternalServerError)
			return
		}

		rows, err := tx.Query(`
			SELECT b.id, b.discord_id, b.choice, b.amount, b.locked_multiplier, u.username
			FROM bets b
			JOIN users u ON b.discord_id = u.discord_id
			WHERE b.status = 'pending' AND b.bet_category = 'prop'`)

		var winners []discordbot.BetResult
		var losers []discordbot.BetResult
		var winnerIDs []string

		if err == nil {
			type Bet struct {
				ID       int
				Discord  string
				Choice   string
				Amount   float64
				Mult     float64
				Username string
			}
			var bets []Bet
			for rows.Next() {
				var b Bet
				rows.Scan(&b.ID, &b.Discord, &b.Choice, &b.Amount, &b.Mult, &b.Username)
				bets = append(bets, b)
			}
			rows.Close()

			for _, b := range bets {
				newStatus := "lost"
				if b.Choice == req.Outcome {
					newStatus = "won"
					payout := b.Amount * b.Mult
					tx.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", payout, b.Discord)
					winners = append(winners, discordbot.BetResult{Username: b.Username, Amount: b.Amount, Payout: payout})
					winnerIDs = append(winnerIDs, b.Discord)
				} else {
					losers = append(losers, discordbot.BetResult{Username: b.Username, Amount: b.Amount})
				}
				tx.Exec("UPDATE bets SET status = ? WHERE id = ?", newStatus, b.ID)
			}
		}

		hub.CurrentMarket = nil
		tx.Commit()

		go func(ids []string) {
			for _, did := range ids {
				db.CheckAndAwardBetBadges(did)
			}
		}(winnerIDs)

		hub.Broadcast <- hub.WSMessage{Type: "market_resolved"}

		if !hub.DevMode {
			go discordbot.SendMarketResolvedNotification(marketPlayer, marketProp, req.Outcome, winners, losers)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Market resolved as %s! Paid out winners."}`,
			strings.ToUpper(req.Outcome))
	})

	// Set active pack season
	mux.HandleFunc("OPTIONS /api/admin/set-pack-season", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/admin/set-pack-season", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != adminToken() {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct{ Season string `json:"season"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Season) == "" {
			http.Error(w, `{"error": "season field required"}`, http.StatusBadRequest)
			return
		}
		_, err := db.DB.Exec(
			"INSERT OR REPLACE INTO server_config (key, value) VALUES ('current_pack_season', ?)",
			strings.TrimSpace(req.Season))
		if err != nil {
			http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "season": %q}`, strings.TrimSpace(req.Season))
	})
}
