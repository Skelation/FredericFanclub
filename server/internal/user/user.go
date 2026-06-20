package user

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/middleware"
)

// cosmeticColumns maps a cosmetic type to its equip slot column in user_profile.
// Only banners and titles ("tags") are supported.
var cosmeticColumns = map[string]string{
	"banner": "banner_id",
	"title":  "title_id",
}

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

	// --- PUBLIC PROFILE ---

	mux.HandleFunc("OPTIONS /api/profile", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/profile", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)

		viewerID := middleware.GetUserIDFromCookie(r)
		targetID := strings.TrimSpace(r.URL.Query().Get("id"))
		if targetID == "" {
			targetID = viewerID
		}
		if targetID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var username, avatar, createdAt string
		var tokens float64
		err := db.DB.QueryRow(`SELECT username, avatar_url, fredtokens, created_at
			FROM users WHERE discord_id = ?`, targetID).
			Scan(&username, &avatar, &tokens, &createdAt)
		if err != nil {
			http.Error(w, `{"error": "user not found"}`, http.StatusNotFound)
			return
		}

		// Equipped cosmetics + bio from the profile table (one slot per type).
		var equippedBanner, equippedTitle sql.NullInt64
		var bio sql.NullString
		db.DB.QueryRow(`SELECT banner_id, title_id, bio
			FROM user_profile WHERE discord_id = ?`, targetID).
			Scan(&equippedBanner, &equippedTitle, &bio)

		// Map of currently-equipped cosmetic ID per type (only valid slots).
		equipped := map[string]int64{}
		for slot, n := range map[string]sql.NullInt64{
			"banner": equippedBanner, "title": equippedTitle,
		} {
			if n.Valid {
				equipped[slot] = n.Int64
			}
		}

		avatarURL := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", targetID, avatar)
		if avatar == "" {
			avatarURL = "https://cdn.discordapp.com/embed/avatars/0.png"
		}

		// Card stats: distinct cards owned + total copies.
		var distinctCards, totalCopies int
		db.DB.QueryRow(`SELECT COUNT(DISTINCT card_id), COALESCE(SUM(quantity), 0)
			FROM inventory WHERE discord_id = ? AND quantity > 0`, targetID).Scan(&distinctCards, &totalCopies)

		// Badges.
		badges := make([]string, 0)
		bRows, _ := db.DB.Query("SELECT badge_type FROM user_badges WHERE discord_id = ? ORDER BY earned_at ASC", targetID)
		if bRows != nil {
			for bRows.Next() {
				var b string
				bRows.Scan(&b)
				badges = append(badges, b)
			}
			bRows.Close()
		}

		// Showcase card.
		var showcase map[string]interface{}
		{
			var name, rarity, imageURL string
			var cardID int
			if err := db.DB.QueryRow(`SELECT us.card_id, c.name, c.rarity, c.image_url
				FROM user_showcase us JOIN cards c ON us.card_id = c.id
				WHERE us.discord_id = ?`, targetID).Scan(&cardID, &name, &rarity, &imageURL); err == nil {
				showcase = map[string]interface{}{"card_id": cardID, "name": name, "rarity": rarity, "image_url": imageURL}
			}
		}

		// Equipped cosmetics (joined to the cosmetics catalogue).
		var banner, title map[string]interface{}
		if equippedBanner.Valid {
			var name, rarity, value string
			if err := db.DB.QueryRow("SELECT name, COALESCE(rarity,'bronze'), value FROM cosmetics WHERE id = ? AND type = 'banner'", equippedBanner.Int64).
				Scan(&name, &rarity, &value); err == nil {
				banner = map[string]interface{}{"name": name, "rarity": rarity, "data": value}
			}
		}
		if equippedTitle.Valid {
			var name, rarity, value string
			if err := db.DB.QueryRow("SELECT name, COALESCE(rarity,'bronze'), value FROM cosmetics WHERE id = ? AND type = 'title'", equippedTitle.Int64).
				Scan(&name, &rarity, &value); err == nil {
				title = map[string]interface{}{"name": name, "rarity": rarity, "text": value, "color": db.RarityColor(rarity)}
			}
		}

		// Owned cosmetics (all types) — only exposed to the profile owner, who
		// uses this list to equip/unequip items on their own profile page.
		isSelf := targetID == viewerID
		ownedCosmetics := make([]map[string]interface{}, 0)
		if isSelf {
			ocRows, _ := db.DB.Query(`SELECT c.id, c.type, c.name, c.value, COALESCE(c.rarity,'bronze')
				FROM user_cosmetics uc JOIN cosmetics c ON uc.cosmetic_id = c.id
				WHERE uc.discord_id = ? AND c.type IN ('banner', 'title')
				ORDER BY c.type ASC, c.id ASC`, targetID)
			if ocRows != nil {
				for ocRows.Next() {
					var id int
					var ctype, name, value, rarity string
					ocRows.Scan(&id, &ctype, &name, &value, &rarity)
					ownedCosmetics = append(ownedCosmetics, map[string]interface{}{
						"id":       id,
						"type":     ctype,
						"name":     name,
						"data":     value,
						"rarity":   rarity,
						"color":    db.RarityColor(rarity),
						"equipped": equipped[ctype] == int64(id),
					})
				}
				ocRows.Close()
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"discord_id":      targetID,
			"username":        username,
			"avatar_url":      avatarURL,
			"fredtokens":      tokens,
			"distinct_cards":  distinctCards,
			"total_cards":     totalCopies,
			"joined_at":       createdAt,
			"is_self":         isSelf,
			"bio":             bio.String,
			"badges":          badges,
			"showcase_card":   showcase,
			"banner":          banner,
			"title":           title,
			"owned_cosmetics": ownedCosmetics,
		})
	})

	// --- EQUIP / UNEQUIP COSMETIC (from the profile page) ---

	mux.HandleFunc("OPTIONS /api/profile/equip", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/profile/equip", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct {
			ItemID   int    `json:"item_id"`   // 0 = unequip
			ItemType string `json:"item_type"` // banner | title | card_foil | case_skin | site_theme | theme
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		col, ok := cosmeticColumns[req.ItemType]
		if !ok {
			http.Error(w, `{"error": "invalid item type"}`, http.StatusBadRequest)
			return
		}
		db.DB.Exec("INSERT OR IGNORE INTO user_profile (discord_id) VALUES (?)", userID)

		// Unequip path.
		if req.ItemID == 0 {
			db.DB.Exec("UPDATE user_profile SET "+col+" = NULL WHERE discord_id = ?", userID)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"success": true}`))
			return
		}

		// Must own the item and its type must match the requested slot.
		var itemType string
		err := db.DB.QueryRow(`SELECT c.type FROM cosmetics c
			JOIN user_cosmetics uc ON uc.cosmetic_id = c.id AND uc.discord_id = ?
			WHERE c.id = ?`, userID, req.ItemID).Scan(&itemType)
		if err != nil {
			http.Error(w, `{"error": "Tu ne possèdes pas cet objet"}`, http.StatusBadRequest)
			return
		}
		if itemType != req.ItemType {
			http.Error(w, `{"error": "type mismatch"}`, http.StatusBadRequest)
			return
		}

		db.DB.Exec("UPDATE user_profile SET "+col+" = ? WHERE discord_id = ?", req.ItemID, userID)
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
		type BannerInfo struct {
			Rarity string `json:"rarity"`
			Data   string `json:"data"`
		}
		type TitleInfo struct {
			Rarity string `json:"rarity"`
			Text   string `json:"text"`
			Color  string `json:"color"`
		}
		type LeaderboardUser struct {
			ID           string            `json:"id"`
			Username     string            `json:"username"`
			Avatar       string            `json:"avatar"`
			Score        float64           `json:"score"`
			Badges       []string          `json:"badges"`
			ShowcaseCard *ShowcaseCardInfo `json:"showcase_card,omitempty"`
			Banner       *BannerInfo       `json:"banner,omitempty"`
			Title        *TitleInfo        `json:"title,omitempty"`
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
		bannerMap := map[string]*BannerInfo{}
		titleMap := map[string]*TitleInfo{}
		freshAvatars := map[string]string{}
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

			// Refresh possibly-stale avatar hashes for the displayed users
			// (parallel; each call is throttled and cached in refreshAvatar).
			hashByID := map[string]string{}
			for _, u := range allUsers {
				hashByID[u.DiscordID] = u.AvatarHash
			}
			var avatarWg sync.WaitGroup
			var avatarMapMu sync.Mutex
			for _, id := range allIDs {
				did := id.(string)
				avatarWg.Add(1)
				go func(id, hash string) {
					defer avatarWg.Done()
					fresh := refreshAvatar(id, hash)
					avatarMapMu.Lock()
					freshAvatars[id] = fresh
					avatarMapMu.Unlock()
				}(did, hashByID[did])
			}
			avatarWg.Wait()
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

			cRows, _ := db.DB.Query(`SELECT p.discord_id,
					b.rarity, b.value,
					t.rarity, t.value
				FROM user_profile p
				LEFT JOIN cosmetics b ON p.banner_id = b.id AND b.type = 'banner'
				LEFT JOIN cosmetics t ON p.title_id  = t.id AND t.type = 'title'
				WHERE p.discord_id IN (`+placeholders+`)`, allIDs...)
			if cRows != nil {
				for cRows.Next() {
					var did string
					var bRarity, bData, tRarity, tText sql.NullString
					cRows.Scan(&did, &bRarity, &bData, &tRarity, &tText)
					if bData.Valid && bData.String != "" {
						bannerMap[did] = &BannerInfo{Rarity: bRarity.String, Data: bData.String}
					}
					if tText.Valid && tText.String != "" {
						titleMap[did] = &TitleInfo{Rarity: tRarity.String, Text: tText.String, Color: db.RarityColor(tRarity.String)}
					}
				}
				cRows.Close()
			}
		}

		toLeaderboardUser := func(u tempUser) LeaderboardUser {
			hash := u.AvatarHash
			if fresh, ok := freshAvatars[u.DiscordID]; ok {
				hash = fresh
			}
			avatar := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", u.DiscordID, hash)
			if hash == "" {
				avatar = "https://cdn.discordapp.com/embed/avatars/0.png"
			}
			badges := badgeMap[u.DiscordID]
			if badges == nil {
				badges = []string{}
			}
			return LeaderboardUser{
				ID:           u.DiscordID,
				Username:     u.Username,
				Avatar:       avatar,
				Score:        u.Score,
				Badges:       badges,
				ShowcaseCard: showcaseMap[u.DiscordID],
				Banner:       bannerMap[u.DiscordID],
				Title:        titleMap[u.DiscordID],
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
