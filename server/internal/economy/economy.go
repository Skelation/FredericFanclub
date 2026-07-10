package economy

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, allowed []string) {
	// --- PACK ---

	mux.HandleFunc("/api/economy/buy-pack", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)

		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		currentUserID := middleware.GetUserIDFromCookie(r)
		if currentUserID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var userTokens float64
		err := db.DB.QueryRow("SELECT fredtokens FROM users WHERE discord_id = ?", currentUserID).Scan(&userTokens)
		if err != nil || userTokens < 250 {
			http.Error(w, `{"error": "insufficient funds"}`, http.StatusBadRequest)
			return
		}

		roll := rand.Float64()
		var targetRarity string
		if roll < 0.004 {
			targetRarity = "radiant"
		} else if roll < 0.015 {
			targetRarity = "immortal"
		} else if roll < 0.050 {
			targetRarity = "ascendant"
		} else if roll < 0.150 {
			targetRarity = "diamond"
		} else if roll < 0.500 {
			targetRarity = "bronze"
		} else {
			targetRarity = "iron"
		}

		var currentSeason string
		if err := db.DB.QueryRow("SELECT value FROM server_config WHERE key = 'current_pack_season'").Scan(&currentSeason); err != nil {
			currentSeason = "Season 1"
		}

		var cardID int
		var cardName, cardRarity, cardImage string
		err = db.DB.QueryRow(
			"SELECT id, name, rarity, image_url FROM cards WHERE rarity = ? AND season = ? ORDER BY RANDOM() LIMIT 1",
			targetRarity, currentSeason).Scan(&cardID, &cardName, &cardRarity, &cardImage)
		if err != nil {
			http.Error(w, `{"error": "No cards available for rarity: `+targetRarity+`"}`, http.StatusInternalServerError)
			return
		}

		tx, _ := db.DB.Begin()
		_, err = tx.Exec("UPDATE users SET fredtokens = fredtokens - 250 WHERE discord_id = ?", currentUserID)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "db update failed"}`, 500)
			return
		}
		_, err = tx.Exec(`
			INSERT INTO inventory (discord_id, card_id, quantity) VALUES (?, ?, 1)
			ON CONFLICT(discord_id, card_id) DO UPDATE SET quantity = quantity + 1
		`, currentUserID, cardID)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "inventory update failed"}`, 500)
			return
		}
		tx.Commit()

		go db.CheckAndAwardSeasonBadge(currentUserID, currentSeason)

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "card": {"name": "%s", "rarity": "%s", "image_url": "%s"}}`,
			cardName, cardRarity, cardImage)
	})

	// --- DAILY ---

	mux.HandleFunc("OPTIONS /api/economy/daily", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/economy/daily", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var lastClaimStr sql.NullString
		err := db.DB.QueryRow("SELECT last_daily_claim FROM users WHERE discord_id = ?", userID).Scan(&lastClaimStr)
		if err != nil {
			http.Error(w, `{"error": "user not found"}`, http.StatusInternalServerError)
			return
		}

		var lastClaim time.Time
		if lastClaimStr.Valid && lastClaimStr.String != "" {
			lastClaim, _ = time.Parse(time.RFC3339, lastClaimStr.String)
		}

		now := time.Now().UTC()
		timeSinceLastClaim := now.Sub(lastClaim)
		cooldown := 24 * time.Hour

		w.Header().Set("Content-Type", "application/json")
		if timeSinceLastClaim < cooldown {
			timeLeft := cooldown - timeSinceLastClaim
			hours := int(timeLeft.Hours())
			minutes := int(timeLeft.Minutes()) % 60
			fmt.Fprintf(w, `{"available": false, "hours": %d, "minutes": %d}`, hours, minutes)
			return
		}
		fmt.Fprintf(w, `{"available": true}`)
	})

	mux.HandleFunc("POST /api/economy/daily", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)

		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var lastClaimStr sql.NullString
		err := db.DB.QueryRow("SELECT last_daily_claim FROM users WHERE discord_id = ?", userID).Scan(&lastClaimStr)
		if err != nil {
			http.Error(w, `{"error": "user not found"}`, http.StatusInternalServerError)
			return
		}

		var lastClaim time.Time
		if lastClaimStr.Valid && lastClaimStr.String != "" {
			lastClaim, _ = time.Parse(time.RFC3339, lastClaimStr.String)
		}

		now := time.Now().UTC()
		timeSinceLastClaim := now.Sub(lastClaim)
		cooldown := 24 * time.Hour

		w.Header().Set("Content-Type", "application/json")

		if timeSinceLastClaim < cooldown {
			timeLeft := cooldown - timeSinceLastClaim
			hours := int(timeLeft.Hours())
			minutes := int(timeLeft.Minutes()) % 60
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error": "Come back in %d hours and %d minutes!"}`, hours, minutes)
			return
		}

		tx, _ := db.DB.Begin()
		_, err = tx.Exec(
			"UPDATE users SET fredtokens = fredtokens + 250, last_daily_claim = ? WHERE discord_id = ?",
			now.Format(time.RFC3339), userID)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}
		tx.Commit()

		fmt.Fprintf(w, `{"success": true, "message": "Daily Drop Claimed! +250 FT"}`)
	})

	// --- TRADE-UP ---

	mux.HandleFunc("OPTIONS /api/economy/trade-up", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/economy/trade-up", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct{ CardIDs []int `json:"card_ids"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		if len(req.CardIDs) != 5 {
			http.Error(w, `{"error": "You must provide exactly 5 cards!"}`, http.StatusBadRequest)
			return
		}

		required := make(map[int]int)
		for _, id := range req.CardIDs {
			required[id]++
		}

		tx, _ := db.DB.Begin()
		defer tx.Rollback()

		var commonRarity string
		for cardID, qtyNeeded := range required {
			var ownedQty int
			var rarity string
			err := tx.QueryRow(
				"SELECT i.quantity, c.rarity FROM inventory i JOIN cards c ON i.card_id = c.id WHERE i.discord_id = ? AND i.card_id = ?",
				userID, cardID).Scan(&ownedQty, &rarity)
			if err != nil || ownedQty < qtyNeeded {
				http.Error(w, `{"error": "You do not own enough of these cards!"}`, http.StatusBadRequest)
				return
			}
			if commonRarity == "" {
				commonRarity = rarity
			} else if commonRarity != rarity {
				http.Error(w, `{"error": "All 5 cards must be the exact same rarity!"}`, http.StatusBadRequest)
				return
			}
		}

		tierList := []string{"iron", "bronze", "diamond", "ascendant", "immortal", "radiant"}
		var nextRarity string
		for i, r := range tierList {
			if r == commonRarity {
				if i+1 < len(tierList) {
					nextRarity = tierList[i+1]
				}
				break
			}
		}
		if nextRarity == "" {
			http.Error(w, `{"error": "You cannot trade up Radiant cards!"}`, http.StatusBadRequest)
			return
		}

		var currentSeason string
		if err := db.DB.QueryRow("SELECT value FROM server_config WHERE key = 'current_pack_season'").Scan(&currentSeason); err != nil {
			currentSeason = "Season 1"
		}

		var winCardID int
		var winName, winRarity, winImage string
		err := tx.QueryRow(
			"SELECT id, name, rarity, image_url FROM cards WHERE rarity = ? AND season = ? ORDER BY RANDOM() LIMIT 1", nextRarity, currentSeason).
			Scan(&winCardID, &winName, &winRarity, &winImage)
		if err != nil {
			http.Error(w, `{"error": "The database has no cards in the next tier for the current season!"}`, http.StatusInternalServerError)
			return
		}

		for cardID, qtyNeeded := range required {
			_, err = tx.Exec(
				"UPDATE inventory SET quantity = quantity - ? WHERE discord_id = ? AND card_id = ?",
				qtyNeeded, userID, cardID)
			if err != nil {
				http.Error(w, `{"error": "Failed to burn cards"}`, http.StatusInternalServerError)
				return
			}
		}

		_, err = tx.Exec(`
			INSERT INTO inventory (discord_id, card_id, quantity) VALUES (?, ?, 1)
			ON CONFLICT(discord_id, card_id) DO UPDATE SET quantity = quantity + 1
		`, userID, winCardID)
		if err != nil {
			http.Error(w, `{"error": "Failed to grant new card"}`, http.StatusInternalServerError)
			return
		}

		tx.Commit()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "card": {"name": "%s", "rarity": "%s", "image_url": "%s"}}`,
			winName, winRarity, winImage)
	})

	// --- REDEEM CODE ---

	mux.HandleFunc("OPTIONS /api/economy/redeem-code", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/economy/redeem-code", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct{ Code string `json:"code"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Code) == "" {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		code := strings.TrimSpace(strings.ToUpper(req.Code))

		var rewardFT, maxUses, usesSoFar int
		var expiresAt string
		err := db.DB.QueryRow(
			"SELECT reward_ft, max_uses, uses_so_far, expires_at FROM redeem_codes WHERE code = ?", code).
			Scan(&rewardFT, &maxUses, &usesSoFar, &expiresAt)
		if err != nil {
			http.Error(w, `{"error": "Code invalide ou inexistant"}`, http.StatusBadRequest)
			return
		}

		expiry, parseErr := time.Parse("2006-01-02T15:04:05Z", expiresAt)
		if parseErr != nil {
			expiry, parseErr = time.Parse("2006-01-02 15:04:05", expiresAt)
		}
		if parseErr == nil && time.Now().UTC().After(expiry) {
			http.Error(w, `{"error": "Ce code a expiré"}`, http.StatusBadRequest)
			return
		}
		if usesSoFar >= maxUses {
			http.Error(w, `{"error": "Ce code a atteint son maximum d'utilisations"}`, http.StatusBadRequest)
			return
		}

		var alreadyUsed int
		db.DB.QueryRow("SELECT COUNT(*) FROM code_redemptions WHERE user_id = ? AND code = ?", userID, code).Scan(&alreadyUsed)
		if alreadyUsed > 0 {
			http.Error(w, `{"error": "Tu as déjà utilisé ce code"}`, http.StatusBadRequest)
			return
		}

		tx, err := db.DB.Begin()
		if err != nil {
			http.Error(w, `{"error": "server error"}`, http.StatusInternalServerError)
			return
		}
		tx.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", rewardFT, userID)
		tx.Exec("UPDATE redeem_codes SET uses_so_far = uses_so_far + 1 WHERE code = ?", code)
		tx.Exec("INSERT INTO code_redemptions (user_id, code) VALUES (?, ?)", userID, code)
		tx.Commit()

		var newBalance float64
		db.DB.QueryRow("SELECT fredtokens FROM users WHERE discord_id = ?", userID).Scan(&newBalance)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "reward_ft": %d, "new_balance": %g}`, rewardFT, newBalance)
	})

	// --- SHRED ---

	mux.HandleFunc("OPTIONS /api/economy/shred", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/economy/shred", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)

		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct{ CardID int `json:"card_id"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		var rarity string
		var quantity int
		err := db.DB.QueryRow(`
			SELECT c.rarity, i.quantity
			FROM inventory i
			JOIN cards c ON i.card_id = c.id
			WHERE i.discord_id = ? AND i.card_id = ? AND i.quantity > 0`,
			userID, req.CardID).Scan(&rarity, &quantity)
		if err != nil {
			http.Error(w, `{"error": "You do not own this card!"}`, http.StatusBadRequest)
			return
		}

		payouts := map[string]int{
			"iron":      20,
			"bronze":    50,
			"diamond":   200,
			"ascendant": 500,
			"immortal":  2500,
			"radiant":   10000,
		}
		payoutAmount := payouts[rarity]

		tx, _ := db.DB.Begin()
		_, err = tx.Exec("UPDATE inventory SET quantity = quantity - 1 WHERE discord_id = ? AND card_id = ?", userID, req.CardID)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "db failed"}`, 500)
			return
		}
		_, err = tx.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", payoutAmount, userID)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "db failed"}`, 500)
			return
		}
		tx.Commit()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "payout": %d, "remaining": %d}`, payoutAmount, quantity-1)
	})
}
