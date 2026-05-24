package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"fredericfanclub/server/discordbot"
	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/hub"
	"fredericfanclub/server/internal/matches"
	"fredericfanclub/server/internal/middleware"
)

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
			req.MaxUses = 1
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

		roster := []struct{ Name, Tag string }{
			{"TheMisterED", "0007"}, {"Heri", "BLUB"}, {"hhj", "8769"},
			{"Djibはコリーヌ お あいして", "LOVE"}, {"Graussbyt", "5629"},
			{"Lal6s9gne", "6641"}, {"XTrixツ", "DREAM"}, {"小胖子vincent", "4397"},
		}

		var playerTag string
		for _, rp := range roster {
			if strings.EqualFold(rp.Name, req.Player) {
				playerTag = rp.Tag
				break
			}
		}

		cacheKey := strings.ToLower(req.Player + "#" + playerTag)
		targetPuuid := ""

		if val, ok := hub.PuuidCache.Load(cacheKey); ok {
			targetPuuid = val.(string)
		} else {
			accountURL := fmt.Sprintf("%s/v1/account/%s/%s", base, url.PathEscape(req.Player), url.PathEscape(playerTag))
			reqAcc, _ := http.NewRequest("GET", accountURL, nil)
			if apiKey != "" {
				reqAcc.Header.Set("Authorization", apiKey)
			}
			respAcc, errAcc := http.DefaultClient.Do(reqAcc)
			if errAcc == nil && respAcc.StatusCode == 200 {
				var accData struct {
					Data struct {
						Puuid string `json:"puuid"`
					} `json:"data"`
				}
				json.NewDecoder(respAcc.Body).Decode(&accData)
				targetPuuid = accData.Data.Puuid
				if targetPuuid != "" {
					hub.PuuidCache.Store(cacheKey, targetPuuid)
				}
				respAcc.Body.Close()
			}
		}

		if targetPuuid == "" {
			http.Error(w, `{"error": "Failed to look up player PUUID for preview."}`, http.StatusBadRequest)
			return
		}

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

			for _, p := range allPlayers {
				playerMap, ok := p.(map[string]interface{})
				if !ok {
					continue
				}
				puuid, _ := playerMap["puuid"].(string)
				if !strings.EqualFold(puuid, targetPuuid) {
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
