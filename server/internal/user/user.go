package user

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, allowed []string) {
	mux.HandleFunc("OPTIONS /api/user/me", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/user/me", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)

		cookie, err := r.Cookie("fred_user_id")
		if err != nil {
			http.Error(w, `{"error": "not logged in"}`, http.StatusUnauthorized)
			return
		}

		var username, avatar, linkedPlayer string
		var tokens float64
		err = db.DB.QueryRow("SELECT username, avatar_url, fredtokens, linked_player FROM users WHERE discord_id = ?", cookie.Value).
			Scan(&username, &avatar, &tokens, &linkedPlayer)
		if err != nil {
			http.Error(w, `{"error": "user not found in db"}`, http.StatusNotFound)
			return
		}

		avatarURL := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", cookie.Value, avatar)
		if avatar == "" {
			avatarURL = "https://cdn.discordapp.com/embed/avatars/0.png"
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"username": "%s", "avatar_url": "%s", "fredtokens": %g, "linked_player": "%s"}`,
			username, avatarURL, tokens, linkedPlayer)
	})

	// --- BADGES ---

	mux.HandleFunc("GET /api/user/badges", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		rows, err := db.DB.Query("SELECT badge_type, earned_at FROM user_badges WHERE discord_id = ? ORDER BY earned_at ASC", userID)
		if err != nil {
			http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		type Badge struct {
			Type     string `json:"type"`
			EarnedAt string `json:"earned_at"`
		}
		badges := make([]Badge, 0)
		for rows.Next() {
			var b Badge
			rows.Scan(&b.Type, &b.EarnedAt)
			badges = append(badges, b)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"badges": badges})
	})

	// --- SHOWCASE ---

	mux.HandleFunc("OPTIONS /api/user/showcase", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/user/showcase", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var name, rarity, imageURL string
		var cardID int
		err := db.DB.QueryRow(`SELECT us.card_id, c.name, c.rarity, c.image_url
			FROM user_showcase us JOIN cards c ON us.card_id = c.id
			WHERE us.discord_id = ?`, userID).Scan(&cardID, &name, &rarity, &imageURL)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"showcase_card": null}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"showcase_card": {"card_id": %d, "name": %q, "rarity": %q, "image_url": %q}}`,
			cardID, name, rarity, imageURL)
	})

	mux.HandleFunc("POST /api/user/showcase", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct{ CardID int `json:"card_id"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CardID == 0 {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		var qty int
		db.DB.QueryRow("SELECT quantity FROM inventory WHERE discord_id = ? AND card_id = ?", userID, req.CardID).Scan(&qty)
		if qty <= 0 {
			http.Error(w, `{"error": "You don't own this card"}`, http.StatusBadRequest)
			return
		}
		db.DB.Exec(`INSERT INTO user_showcase (discord_id, card_id) VALUES (?, ?)
			ON CONFLICT(discord_id) DO UPDATE SET card_id = excluded.card_id`, userID, req.CardID)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"success": true}`))
	})

	// --- LEADERBOARD ---

	mux.HandleFunc("OPTIONS /api/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)

		type ShowcaseCardInfo struct {
			Name     string `json:"name"`
			Rarity   string `json:"rarity"`
			ImageURL string `json:"image_url"`
		}
		type LeaderboardUser struct {
			Username     string           `json:"username"`
			Avatar       string           `json:"avatar"`
			Score        float64          `json:"score"`
			Badges       []string         `json:"badges"`
			ShowcaseCard *ShowcaseCardInfo `json:"showcase_card,omitempty"`
		}
		type tempUser struct {
			DiscordID  string
			Username   string
			AvatarHash string
			Score      float64
		}
		season := strings.TrimSpace(r.URL.Query().Get("season"))
		if season == "" {
			db.DB.QueryRow("SELECT value FROM server_config WHERE key = 'current_pack_season'").Scan(&season)
		}

		availableSeasons := make([]string, 0)
		sRows, _ := db.DB.Query("SELECT DISTINCT season FROM cards WHERE season != '' ORDER BY season ASC")
		if sRows != nil {
			for sRows.Next() {
				var s string
				sRows.Scan(&s)
				availableSeasons = append(availableSeasons, s)
			}
			sRows.Close()
		}

		var tempTokens, tempCards []tempUser

		rows1, err := db.DB.Query("SELECT discord_id, username, avatar_url, fredtokens FROM users ORDER BY fredtokens DESC LIMIT 10")
		if err == nil {
			for rows1.Next() {
				var u tempUser
				rows1.Scan(&u.DiscordID, &u.Username, &u.AvatarHash, &u.Score)
				tempTokens = append(tempTokens, u)
			}
			rows1.Close()
		}

		rows2, err := db.DB.Query(`
			SELECT u.discord_id, u.username, u.avatar_url, COUNT(i.card_id) as total_cards
			FROM users u
			JOIN inventory i ON u.discord_id = i.discord_id
			JOIN cards c ON i.card_id = c.id
			WHERE i.quantity > 0 AND c.season = ?
			GROUP BY u.discord_id
			ORDER BY total_cards DESC
			LIMIT 10
		`, season)
		if err == nil {
			for rows2.Next() {
				var u tempUser
				rows2.Scan(&u.DiscordID, &u.Username, &u.AvatarHash, &u.Score)
				tempCards = append(tempCards, u)
			}
			rows2.Close()
		}

		badgeMap := map[string][]string{}
		showcaseMap := map[string]*ShowcaseCardInfo{}
		allUsers := append(tempTokens, tempCards...)
		if len(allUsers) > 0 {
			seenIDs := map[string]bool{}
			allIDs := make([]interface{}, 0)
			for _, u := range allUsers {
				if !seenIDs[u.DiscordID] {
					allIDs = append(allIDs, u.DiscordID)
					seenIDs[u.DiscordID] = true
				}
			}
			placeholders := strings.Repeat("?,", len(allIDs))
			placeholders = placeholders[:len(placeholders)-1]

			bRows, _ := db.DB.Query("SELECT discord_id, badge_type FROM user_badges WHERE discord_id IN ("+placeholders+")", allIDs...)
			if bRows != nil {
				for bRows.Next() {
					var did, bt string
					bRows.Scan(&did, &bt)
					badgeMap[did] = append(badgeMap[did], bt)
				}
				bRows.Close()
			}

			scRows, _ := db.DB.Query(`SELECT us.discord_id, c.name, c.rarity, c.image_url
				FROM user_showcase us JOIN cards c ON us.card_id = c.id
				WHERE us.discord_id IN (`+placeholders+`)`, allIDs...)
			if scRows != nil {
				for scRows.Next() {
					var did, name, rarity, imageURL string
					scRows.Scan(&did, &name, &rarity, &imageURL)
					showcaseMap[did] = &ShowcaseCardInfo{Name: name, Rarity: rarity, ImageURL: imageURL}
				}
				scRows.Close()
			}
		}

		toLeaderboardUser := func(u tempUser) LeaderboardUser {
			avatar := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", u.DiscordID, u.AvatarHash)
			if u.AvatarHash == "" {
				avatar = "https://cdn.discordapp.com/embed/avatars/0.png"
			}
			badges := badgeMap[u.DiscordID]
			if badges == nil {
				badges = []string{}
			}
			return LeaderboardUser{
				Username:     u.Username,
				Avatar:       avatar,
				Score:        u.Score,
				Badges:       badges,
				ShowcaseCard: showcaseMap[u.DiscordID],
			}
		}

		res := struct {
			TopTokens        []LeaderboardUser `json:"top_tokens"`
			TopCards         []LeaderboardUser `json:"top_cards"`
			Season           string            `json:"season"`
			AvailableSeasons []string          `json:"available_seasons"`
		}{
			TopTokens:        make([]LeaderboardUser, 0),
			TopCards:         make([]LeaderboardUser, 0),
			Season:           season,
			AvailableSeasons: availableSeasons,
		}
		for _, u := range tempTokens {
			res.TopTokens = append(res.TopTokens, toLeaderboardUser(u))
		}
		for _, u := range tempCards {
			res.TopCards = append(res.TopCards, toLeaderboardUser(u))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	})
}
