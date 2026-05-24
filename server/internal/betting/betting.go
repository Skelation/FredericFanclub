package betting

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"fredericfanclub/server/discordbot"
	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/hub"
	"fredericfanclub/server/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, allowed []string) {
	// WebSocket connection handler
	mux.HandleFunc("GET /api/ws/betting", func(w http.ResponseWriter, r *http.Request) {
		ws, err := hub.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		hub.Mu.Lock()
		hub.Clients[ws] = true
		hub.Mu.Unlock()

		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				hub.Mu.Lock()
				delete(hub.Clients, ws)
				hub.Mu.Unlock()
				break
			}
		}
	})

	// Get active market
	mux.HandleFunc("OPTIONS /api/betting/market", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/betting/market", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Content-Type", "application/json")

		if hub.CurrentMarket == nil {
			fmt.Fprintf(w, `{"exists": false}`)
			return
		}

		type ActiveBet struct {
			Username string  `json:"username"`
			Avatar   string  `json:"avatar"`
			Choice   string  `json:"choice"`
			Amount   float64 `json:"amount"`
		}
		activeBets := make([]ActiveBet, 0)

		rows, err := db.DB.Query(`
			SELECT u.username, u.avatar_url, u.discord_id, b.choice, b.amount
			FROM bets b
			JOIN users u ON b.discord_id = u.discord_id
			WHERE b.status = 'pending'
			  AND b.bet_category = 'prop'
			  AND b.target_player = ?
			  AND b.prop_type = ?
			ORDER BY b.id DESC`,
			hub.CurrentMarket.Player, hub.CurrentMarket.PropType)
		if err == nil {
			for rows.Next() {
				var ab ActiveBet
				var avatarHash, discordID string
				rows.Scan(&ab.Username, &avatarHash, &discordID, &ab.Choice, &ab.Amount)
				ab.Avatar = fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", discordID, avatarHash)
				if avatarHash == "" {
					ab.Avatar = "https://cdn.discordapp.com/embed/avatars/0.png"
				}
				activeBets = append(activeBets, ab)
			}
			rows.Close()
		}

		response := struct {
			*hub.PropMarket
			Exists     bool        `json:"exists"`
			ActiveBets []ActiveBet `json:"active_bets"`
		}{
			PropMarket: hub.CurrentMarket,
			Exists:     true,
			ActiveBets: activeBets,
		}
		json.NewEncoder(w).Encode(response)
	})

	// Place a bet
	mux.HandleFunc("OPTIONS /api/betting/place", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/betting/place", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)

		if hub.CurrentMarket == nil || !hub.CurrentMarket.IsOpen {
			http.Error(w, `{"error": "Market is currently closed"}`, http.StatusBadRequest)
			return
		}

		cookie, err := r.Cookie("fred_user_id")
		if err != nil {
			http.Error(w, `{"error": "Not logged in"}`, http.StatusUnauthorized)
			return
		}
		discordID := cookie.Value

		var req struct {
			Choice string  `json:"choice"`
			Amount float64 `json:"amount"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "Invalid request"}`, http.StatusBadRequest)
			return
		}
		if req.Choice != "over" && req.Choice != "under" {
			http.Error(w, `{"error": "Invalid choice"}`, http.StatusBadRequest)
			return
		}
		if req.Amount <= 0 {
			http.Error(w, `{"error": "Bet must be at least 1 FT"}`, http.StatusBadRequest)
			return
		}

		lockedMultiplier := hub.CurrentMarket.UnderMult
		if req.Choice == "over" {
			lockedMultiplier = hub.CurrentMarket.OverMult
		}

		tx, err := db.DB.Begin()
		if err != nil {
			http.Error(w, `{"error": "Server error"}`, http.StatusInternalServerError)
			return
		}

		var balance float64
		var linkedPlayer string
		err = tx.QueryRow("SELECT fredtokens, linked_player FROM users WHERE discord_id = ?", discordID).
			Scan(&balance, &linkedPlayer)
		if err != nil || balance < req.Amount {
			tx.Rollback()
			http.Error(w, `{"error": "Not enough Fredtokens!"}`, http.StatusBadRequest)
			return
		}

		if linkedPlayer != "none" {
			for _, vetoedPlayer := range hub.CurrentMarket.Vetoes {
				if strings.EqualFold(linkedPlayer, vetoedPlayer) || vetoedPlayer == "ALL" {
					tx.Rollback()
					http.Error(w, `{"error": "Conflict of Interest: You are vetoed from betting on this market!"}`, http.StatusForbidden)
					return
				}
			}
			if strings.EqualFold(linkedPlayer, hub.CurrentMarket.Player) {
				tx.Rollback()
				http.Error(w, `{"error": "Conflict of Interest: You cannot bet on your own performance!"}`, http.StatusForbidden)
				return
			}
		}

		_, err = tx.Exec("UPDATE users SET fredtokens = fredtokens - ? WHERE discord_id = ?", req.Amount, discordID)
		result, err := tx.Exec(`INSERT INTO bets
			(discord_id, bet_category, target_player, prop_type, line_value, choice, amount, locked_multiplier)
			VALUES (?, 'prop', ?, ?, ?, ?, ?, ?)`,
			discordID, hub.CurrentMarket.Player, hub.CurrentMarket.PropType, hub.CurrentMarket.Line,
			req.Choice, req.Amount, lockedMultiplier)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "Failed to place bet"}`, http.StatusInternalServerError)
			return
		}

		betID, _ := result.LastInsertId()
		tx.Commit()

		var username, avatarHash string
		db.DB.QueryRow("SELECT username, avatar_url FROM users WHERE discord_id = ?", discordID).Scan(&username, &avatarHash)
		avatarURL := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", discordID, avatarHash)
		if avatarHash == "" {
			avatarURL = "https://cdn.discordapp.com/embed/avatars/0.png"
		}

		hub.Broadcast <- hub.WSMessage{
			Type: "new_bet",
			Payload: map[string]interface{}{
				"id":       betID,
				"username": username,
				"avatar":   avatarURL,
				"choice":   req.Choice,
				"amount":   req.Amount,
			},
		}

		if !hub.DevMode {
			go discordbot.SendBetNotification(linkedPlayer, req.Choice, int(req.Amount))
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "new_balance": %g}`, balance-req.Amount)
	})
}
