package main

import (
	"database/sql"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"fmt"
	"sort"
	"sync"
	"context"
	"encoding/json"
	"golang.org/x/oauth2"
	
	_ "github.com/mattn/go-sqlite3"
)

var (
	CurrentMarket *PropMarket
	DB          *sql.DB 
	oauthConfig *oauth2.Config
	oauthState  = "fred-secure-state-token"
)

type PropMarket struct {
	Player       string  `json:"player"`
	PropType     string  `json:"prop_type"` // "kills" or "deaths"
	Line         float64 `json:"line"`      // e.g., 14.5
	OverMult     float64 `json:"over_multiplier"`
	UnderMult    float64 `json:"under_multiplier"`
	IsOpen       bool    `json:"is_open"`
}

func main() {
	loadDotEnv()
	initDB()
	initOAuth()

	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("VALORANT_API_BASE")), "/")
	if base == "" {
		base = "https://api.henrikdev.xyz/valorant"
	}
	apiKey := strings.TrimSpace(os.Getenv("VALORANT_API_KEY"))
	matchPath := strings.Trim(strings.TrimSpace(os.Getenv("VALORANT_MATCHES_PATH")), "/")
	if matchPath == "" {
		matchPath = "v4/matches"
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}
	allowed := parseOrigins(os.Getenv("CORS_ORIGINS"))

	startMatchPoller(base, matchPath, apiKey)

	mux := http.NewServeMux()

	// --- USER PROFILE ROUTE ---
	
	mux.HandleFunc("OPTIONS /api/user/me", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/user/me", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)

		// 1. Check if they have the login cookie
		cookie, err := r.Cookie("fred_user_id")
		if err != nil {
			http.Error(w, `{"error": "not logged in"}`, http.StatusUnauthorized)
			return
		}

		// 2. Look them up in the database
		var username, avatar string
		var tokens float64 // <--- FIX: Changed to float64 to prevent crashes!
		err = DB.QueryRow("SELECT username, avatar_url, fredtokens FROM users WHERE discord_id = ?", cookie.Value).Scan(&username, &avatar, &tokens)

		if err != nil {
			http.Error(w, `{"error": "user not found in db"}`, http.StatusNotFound)
			return
		}

		// 3. Format the Discord Avatar URL
		avatarURL := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", cookie.Value, avatar)
		if avatar == "" { 
			avatarURL = "https://cdn.discordapp.com/embed/avatars/0.png"
		}

		// 4. Send the data back to the frontend!
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"username": "%s", "avatar_url": "%s", "fredtokens": %g}`, username, avatarURL, tokens) // <--- FIX: Formats the decimal away!
	})

	// --- AUTHENTICATION ROUTES ---

	// 1. Send the user to Discord to log in
	mux.HandleFunc("GET /api/auth/discord", func(w http.ResponseWriter, r *http.Request) {
		url := oauthConfig.AuthCodeURL(oauthState)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	})

	// 2. Discord sends them back here after they approve
	mux.HandleFunc("GET /api/auth/discord/callback", func(w http.ResponseWriter, r *http.Request) {
		// Verify state to prevent CSRF attacks
		if r.FormValue("state") != oauthState {
			http.Error(w, "State invalid", http.StatusBadRequest)
			return
		}

		// Exchange the code Discord gave us for an access token
		token, err := oauthConfig.Exchange(context.Background(), r.FormValue("code"))
		if err != nil {
			http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
			return
		}

		// Use the token to fetch the user's profile from Discord
		res, err := oauthConfig.Client(context.Background(), token).Get("https://discord.com/api/users/@me")
		if err != nil || res.StatusCode != 200 {
			http.Error(w, "Failed to fetch user info", http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()

		// Decode the JSON profile
		var discordUser struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Avatar   string `json:"avatar"`
		}
		if err := json.NewDecoder(res.Body).Decode(&discordUser); err != nil {
			http.Error(w, "Failed to decode user info", http.StatusInternalServerError)
			return
		}

		// UPSERT INTO DATABASE: 
		// If they are new, give them 1000 tokens. If they exist, just update their username/avatar.
		upsertQuery := `
		INSERT INTO users (discord_id, username, avatar_url, fredtokens) 
		VALUES (?, ?, ?, 1000) 
		ON CONFLICT(discord_id) DO UPDATE SET 
			username=excluded.username, 
			avatar_url=excluded.avatar_url;`
		
		_, err = DB.Exec(upsertQuery, discordUser.ID, discordUser.Username, discordUser.Avatar)
		if err != nil {
			log.Println("DB Error:", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		// Set a secure cookie so your frontend knows who is logged in
		http.SetCookie(w, &http.Cookie{
			Name:     "fred_user_id",
			Value:    discordUser.ID,
			Path:     "/",
			Domain:   ".fredericfan.club", // NEW: Shares cookie with frontend
			HttpOnly: false,
			Secure:   true,                // NEW: Required for modern browsers
			SameSite: http.SameSiteNoneMode, // NEW: Allows cross-subdomain sharing
			MaxAge:   86400 * 30, 
		})

		// Redirect them back to the homepage
		http.Redirect(w, r, "https://fredericfan.club/", http.StatusTemporaryRedirect)
	})

	// ==========================================
	// --- PREDICTION MARKET / BETTING ROUTES ---
	// ==========================================

	// 1. PUBLIC: Get the currently active event market
	mux.HandleFunc("OPTIONS /api/betting/market", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/betting/market", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Content-Type", "application/json")
		
		if CurrentMarket == nil {
			fmt.Fprintf(w, `{"exists": false}`)
			return
		}

		// NEW: Fetch all live bets from the database!
		type ActiveBet struct {
			Username string  `json:"username"`
			Avatar   string  `json:"avatar"`
			Choice   string  `json:"choice"`
			Amount   float64 `json:"amount"`
		}
		activeBets := make([]ActiveBet, 0)
		query := `
			SELECT u.username, u.avatar_url, u.discord_id, b.choice, b.amount 
			FROM bets b 
			JOIN users u ON b.discord_id = u.discord_id 
			WHERE b.status = 'pending' 
			  AND b.bet_category = 'prop' 
			  AND b.target_player = ? 
			  AND b.prop_type = ? 
			ORDER BY b.id DESC`

		// Join the bets table with the users table to get their name and picture
		rows, err := DB.Query(query, CurrentMarket.Player, CurrentMarket.PropType)
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

		// Combine the market info with the live bets list
		response := struct {
			*PropMarket
			Exists     bool        `json:"exists"`
			ActiveBets []ActiveBet `json:"active_bets"`
		}{
			PropMarket: CurrentMarket,
			Exists:     true,
			ActiveBets: activeBets,
		}
		
		json.NewEncoder(w).Encode(response)
	})

	// 2. SECURE: Place a bet on the active player prop
	mux.HandleFunc("OPTIONS /api/betting/place", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/betting/place", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)

		if CurrentMarket == nil || !CurrentMarket.IsOpen {
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
			Choice string  `json:"choice"` // "over" or "under"
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

		// Lock in the specific Vegas multiplier they are betting on
		lockedMultiplier := CurrentMarket.UnderMult
		if req.Choice == "over" {
			lockedMultiplier = CurrentMarket.OverMult
		}

		tx, err := DB.Begin()
		if err != nil {
			http.Error(w, `{"error": "Server error"}`, http.StatusInternalServerError)
			return
		}

		var balance float64
		err = tx.QueryRow("SELECT fredtokens FROM users WHERE discord_id = ?", discordID).Scan(&balance)
		if err != nil || balance < req.Amount {
			tx.Rollback()
			http.Error(w, `{"error": "Not enough Fredtokens!"}`, http.StatusBadRequest)
			return
		}

		// Deduct Tokens
		_, err = tx.Exec("UPDATE users SET fredtokens = fredtokens - ? WHERE discord_id = ?", req.Amount, discordID)
		
		// Insert Detailed Bet Ticket!
		_, err = tx.Exec(`INSERT INTO bets 
			(discord_id, bet_category, target_player, prop_type, line_value, choice, amount, locked_multiplier) 
			VALUES (?, 'prop', ?, ?, ?, ?, ?, ?)`, 
			discordID, CurrentMarket.Player, CurrentMarket.PropType, CurrentMarket.Line, req.Choice, req.Amount, lockedMultiplier)
		
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "Failed to place bet"}`, http.StatusInternalServerError)
			return
		}

		tx.Commit()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "new_balance": %g}`, balance-req.Amount)
	})

	// ADMIN: Preview a randomly generated prop bet for a specific player
	mux.HandleFunc("OPTIONS /api/admin/preview-prop", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/admin/preview-prop", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// 1. Accept the exact Prop Type from the Admin
		var req struct { 
			Player   string `json:"player"`
			PropType string `json:"prop_type"` // NEW: Explicitly choose the bet type!
		}
		json.NewDecoder(r.Body).Decode(&req)

		cacheMutex.RLock()
		dataBytes := cachedMatchesData
		cacheMutex.RUnlock()

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

		
	// --- UNIFIED STATS & WIN/LOSS CALCULATOR ---
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
				if !ok { continue }
				
				name, _ := playerMap["name"].(string)
				if strings.EqualFold(name, req.Player) {
					if req.PropType == "match_result" {
						totalMatches++
						teamName, _ := playerMap["team"].(string)
						if teamName == "" { teamName, _ = playerMap["team_id"].(string) }
						
						if teamsMap, ok := m.Match["teams"].(map[string]interface{}); ok {
							if teamData, ok := teamsMap[strings.ToLower(teamName)].(map[string]interface{}); ok {
								if won, _ := teamData["has_won"].(bool); won { wins++ }
							}
						} else if teamsArr, ok := m.Match["teams"].([]interface{}); ok {
							for _, t := range teamsArr {
								tData, _ := t.(map[string]interface{})
								tID, _ := tData["team_id"].(string)
								if strings.EqualFold(tID, teamName) {
									if won, _ := tData["won"].(bool); won { wins++ }
								}
							}
						}
					} else {
						stats, ok := playerMap["stats"].(map[string]interface{})
						if !ok { continue }
						
						if req.PropType == "kd_ratio" {
							kills, ok1 := stats["kills"].(float64)
							deaths, ok2 := stats["deaths"].(float64)
							if ok1 && ok2 {
								if deaths == 0 { deaths = 1 } 
								statsHistory = append(statsHistory, kills/deaths)
							}
						} else {
							if val, ok := stats[req.PropType].(float64); ok {
								statsHistory = append(statsHistory, val)
							}
						}
					}
					break // Found player, move to next match
				}
			}
		}

		// Calculate Odds based on the type
		var overProb, underProb, line float64

		if req.PropType == "match_result" {
			if totalMatches == 0 {
				http.Error(w, `{"error": "Could not find recent matches for this player."}`, http.StatusBadRequest)
				return
			}
			overProb = wins / totalMatches
			underProb = 1.0 - overProb
			line = 0 // Line is irrelevant for win/loss
		} else {
			if len(statsHistory) == 0 {
				http.Error(w, `{"error": "Could not find recent stats for this player."}`, http.StatusBadRequest)
				return
			}
			total := 0.0
			for _, val := range statsHistory { total += val }
			average := total / float64(len(statsHistory))
			
			if req.PropType == "kd_ratio" {
				line = float64(int(average*10))/10 + 0.05 
			} else {
				line = float64(int(average)) + 0.5 
			}

			overCount := 0.0
			for _, val := range statsHistory {
				if val > line { overCount++ }
			}
			overProb = overCount / float64(len(statsHistory))
			underProb = 1.0 - overProb
		}

		// Safeguards
		if overProb < 0.15 { overProb = 0.15 }
		if overProb > 0.85 { overProb = 0.85 }
		if underProb < 0.15 { underProb = 0.15 }
		if underProb > 0.85 { underProb = 0.85 }

		overMult := (1.0 / overProb) * 0.90
		underMult := (1.0 / underProb) * 0.90

		preview := PropMarket{
			Player: req.Player, PropType: req.PropType, Line: line, 
			OverMult: overMult, UnderMult: underMult, IsOpen: false,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(preview)
	})

	// ADMIN: Publish the previewed bet to the public!
	mux.HandleFunc("OPTIONS /api/admin/publish-prop", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/admin/publish-prop", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var marketToPublish PropMarket
		json.NewDecoder(r.Body).Decode(&marketToPublish)

		marketToPublish.IsOpen = true
		CurrentMarket = &marketToPublish

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Market is now LIVE!"}`)
	})

	// ADMIN: Lock the market (Stop new bets without clearing it)
	mux.HandleFunc("OPTIONS /api/admin/lock-prop", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/admin/lock-prop", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		if CurrentMarket != nil {
			CurrentMarket.IsOpen = false
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Market Locked! No more bets allowed."}`)
	})

	// ADMIN: Resolve the market & Pay out the winners
	mux.HandleFunc("OPTIONS /api/admin/resolve-prop", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/admin/resolve-prop", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct { Outcome string `json:"outcome"` } // "over" or "under"
		json.NewDecoder(r.Body).Decode(&req)

		if req.Outcome != "over" && req.Outcome != "under" {
			http.Error(w, `{"error": "invalid outcome"}`, http.StatusBadRequest)
			return
		}

		tx, err := DB.Begin()
		if err != nil {
			http.Error(w, `{"error": "server error"}`, http.StatusInternalServerError)
			return
		}

		// Fetch all pending prop bets
		rows, err := tx.Query("SELECT id, discord_id, choice, amount, locked_multiplier FROM bets WHERE status = 'pending' AND bet_category = 'prop'")
		if err == nil {
			type Bet struct {
				ID      int
				Discord string
				Choice  string
				Amount  float64
				Mult    float64
			}
			var bets []Bet
			for rows.Next() {
				var b Bet
				rows.Scan(&b.ID, &b.Discord, &b.Choice, &b.Amount, &b.Mult)
				bets = append(bets, b)
			}
			rows.Close()

			// Pay out the winners!
			for _, b := range bets {
				newStatus := "lost"
				if b.Choice == req.Outcome {
					newStatus = "won"
					payout := b.Amount * b.Mult
					tx.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", payout, b.Discord)
				}
				tx.Exec("UPDATE bets SET status = ? WHERE id = ?", newStatus, b.ID)
			}
		}

		// Wipe the market so it goes back to the grey "Market Closed" screen
		CurrentMarket = nil 
		tx.Commit()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Market resolved as %s! Paid out winners."}`, strings.ToUpper(req.Outcome))
	})

		// --- STANDARD API ROUTES ---
	
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("OPTIONS /api/matches", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("OPTIONS /api/matches/roster", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("OPTIONS /api/matches/roster/more", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/matches/roster/more", func(w http.ResponseWriter, r *http.Request) {
		handleRosterMatchesMore(w, r, allowed, base, matchPath, apiKey)
	})
	// NEW CACHED ROUTE
	mux.HandleFunc("GET /api/matches/roster", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		
		cacheMutex.RLock()
		data := cachedMatchesData
		cacheMutex.RUnlock()

		if len(data) == 0 {
			// If the server just booted and hasn't finished the first fetch yet
			w.Header().Set("Retry-After", "5")
			http.Error(w, `{"error": "Warming up cache, please try again in a few seconds"}`, http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})
	
	mux.HandleFunc("GET /api/matches", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		region := strings.TrimSpace(r.URL.Query().Get("region"))
		if region == "" {
			region = strings.TrimSpace(os.Getenv("VALORANT_REGION"))
		}
		if region == "" {
			region = "eu"
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		tag := strings.TrimSpace(r.URL.Query().Get("tag"))
		if region == "" || name == "" || tag == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"missing query: region, name, tag"}`))
			return
		}
		platform := strings.TrimSpace(os.Getenv("VALORANT_PLATFORM"))
		if platform == "" {
			platform = "pc"
		}
		u := base + "/" + matchPath + "/" + url.PathEscape(region)
		if strings.Contains(matchPath, "v4/matches") {
			u += "/" + url.PathEscape(platform)
		}
		u += "/" + url.PathEscape(name) + "/" + url.PathEscape(tag)
		if qs := r.URL.RawQuery; qs != "" {
			u += "?" + qs
		}
		upstream := u
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream, nil)
		if err != nil {
			http.Error(w, `{"error":"bad upstream url"}`, http.StatusInternalServerError)
			return
		}
		if apiKey != "" {
			req.Header.Set("Authorization", apiKey)
		}
		client := &http.Client{Timeout: 25 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("upstream: %v", err)
			http.Error(w, `{"error":"upstream unreachable"}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vv := range resp.Header {
			if strings.EqualFold(k, "Content-Type") {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Printf("copy body: %v", err)
		}
	})

	// Server execution block
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s (VALORANT_API_BASE=%s)", srv.Addr, base)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// --- HELPER FUNCTIONS ---

func initOAuth() {
	oauthConfig = &oauth2.Config{
		RedirectURL:  os.Getenv("DISCORD_REDIRECT_URI"),
		ClientID:     os.Getenv("DISCORD_CLIENT_ID"),
		ClientSecret: os.Getenv("DISCORD_CLIENT_SECRET"),
		Scopes:       []string{"identify"}, 
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://discord.com/api/oauth2/authorize",
			TokenURL: "https://discord.com/api/oauth2/token",
		},
	}
}

func initDB() {
	var err error
	DB, err = sql.Open("sqlite3", "./fred.db")
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	createUsersTable := `
	CREATE TABLE IF NOT EXISTS users (
		discord_id TEXT PRIMARY KEY,
		username TEXT,
		avatar_url TEXT,
		fredtokens INTEGER DEFAULT 1000,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	createBetsTable := `
	CREATE TABLE IF NOT EXISTS bets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		discord_id TEXT,
		bet_category TEXT,       -- 'match' or 'prop'
		match_target TEXT,       -- 'win' or 'loss'
		target_player TEXT,      -- e.g., 'TheMisterED'
		prop_type TEXT,          -- e.g., 'kills' or 'deaths'
		line_value REAL,         -- e.g., 14.5
		choice TEXT,             -- 'over' or 'under'
		amount REAL,             -- amount of FT wagered
		locked_multiplier REAL,  -- The odds locked in at the time of betting (e.g., 1.85)
		status TEXT DEFAULT 'pending', 
		FOREIGN KEY(discord_id) REFERENCES users(discord_id)
	);`

	_, err = DB.Exec(createUsersTable)
	if err != nil {
		log.Fatal("Failed to create users table:", err)
	}

	_, err = DB.Exec(createBetsTable)
	if err != nil {
		log.Fatal("Failed to create bets table:", err)
	}

	log.Println("Database initialized successfully!")
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func parseOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://fredericfan.club:3000"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func applyCORS(w http.ResponseWriter, r *http.Request, allowed []string) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	for _, o := range allowed {
		if origin == o {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
			w.Header().Set("Access-Control-Allow-Credentials", "true") // <--- NEW LINE
			return
		}
	}
}

// --- BACKGROUND MATCH POLLER & CACHE ---

var (
	cachedMatchesData []byte
	cacheMutex        sync.RWMutex
)

func startMatchPoller(base, matchPath, apiKey string) {
	// 1. Define your roster
	roster := []struct{ Name, Tag string }{
		{"TheMisterED", "0007"},
		{"Heri", "BLUB"},
		{"hhj", "8769"},
		{"Djibはコリーヌ お あいして", "LOVE"},
		{"Graussbyt", "5629"},
		{"Lal6s9gne", "6641"},
		{"XTrixツ", "DREAM"},
		{"小胖子vincent", "4397"},
	}

	os.MkdirAll("./data/matches", 0755)
	ticker := time.NewTicker(1 * time.Minute)

	// UPGRADED STRUCT: Now includes the RR map!
	type MatchEntry struct {
		Match      map[string]interface{} `json:"match"`
		Roster     []map[string]string    `json:"roster"`
		RrByPlayer map[string]int         `json:"rrByPlayer"`
	}

	go func() {
		for ; true; <-ticker.C {
			monthStr := time.Now().Format("2006-01")
			filePath := fmt.Sprintf("./data/matches/%s.json", monthStr)

			monthlyMatches := make(map[string]*MatchEntry)
			existingFile, err := os.ReadFile(filePath)
			if err == nil {
				var existing struct {
					Data []MatchEntry `json:"data"`
				}
				if err := json.Unmarshal(existingFile, &existing); err == nil {
					for i := range existing.Data {
						entry := existing.Data[i]
						if meta, ok := entry.Match["metadata"].(map[string]interface{}); ok {
							// Support both v3 and v4 ID keys
							var matchID string
							if id, ok := meta["matchid"].(string); ok { matchID = id }
							if id, ok := meta["match_id"].(string); ok { matchID = id }
							if matchID != "" { monthlyMatches[matchID] = &entry }
						}
					}
				}
			}

			// 1. FETCH MATCHES
			for _, p := range roster {
				reqURL := base + "/" + matchPath + "/eu"
				if strings.Contains(matchPath, "v4/matches") {
					reqURL += "/pc"
				}
				reqURL += "/" + url.PathEscape(p.Name) + "/" + url.PathEscape(p.Tag) + "?mode=competitive&size=15"

				req, _ := http.NewRequest("GET", reqURL, nil)
				if apiKey != "" { req.Header.Set("Authorization", apiKey) }
				resp, err := http.DefaultClient.Do(req)
				
				if err != nil || resp.StatusCode != 200 {
					if resp != nil { resp.Body.Close() }
					continue
				}

				var result struct { Data []map[string]interface{} `json:"data"` }
				json.NewDecoder(resp.Body).Decode(&result)
				resp.Body.Close()

				for _, m := range result.Data {
					if meta, ok := m["metadata"].(map[string]interface{}); ok {
						var matchID string
						if id, ok := meta["matchid"].(string); ok { matchID = id }
						if id, ok := meta["match_id"].(string); ok { matchID = id }

						if matchID != "" {
							entry, exists := monthlyMatches[matchID]
							if !exists {
								entry = &MatchEntry{
									Match:      m,
									Roster:     make([]map[string]string, 0),
									RrByPlayer: make(map[string]int),
								}
								monthlyMatches[matchID] = entry
							}
							
							found := false
							for _, r := range entry.Roster {
								if r["name"] == p.Name && r["tag"] == p.Tag { found = true; break }
							}
							if !found {
								entry.Roster = append(entry.Roster, map[string]string{"name": p.Name, "tag": p.Tag})
							}
						}
					}
				}

				// 2. FETCH MMR HISTORY (To get the RR +/-)
				mmrURL := fmt.Sprintf("%s/v1/mmr-history/eu/%s/%s", base, url.PathEscape(p.Name), url.PathEscape(p.Tag))
				reqMmr, _ := http.NewRequest("GET", mmrURL, nil)
				if apiKey != "" { reqMmr.Header.Set("Authorization", apiKey) }
				respMmr, errMmr := http.DefaultClient.Do(reqMmr)
				
				if errMmr == nil && respMmr.StatusCode == 200 {
					var mmrResult struct {
						Data []struct {
							MatchID string `json:"match_id"`
							Change  int    `json:"mmr_change_to_last_game"`
						} `json:"data"`
					}
					if err := json.NewDecoder(respMmr.Body).Decode(&mmrResult); err == nil {
						playerKey := strings.ToLower(p.Name + "#" + p.Tag)
						for _, mmrItem := range mmrResult.Data {
							if entry, exists := monthlyMatches[mmrItem.MatchID]; exists {
								if entry.RrByPlayer == nil { entry.RrByPlayer = make(map[string]int) }
								entry.RrByPlayer[playerKey] = mmrItem.Change
							}
						}
					}
					respMmr.Body.Close()
				}
				time.Sleep(2 * time.Second)
			}

			// Convert to list
			finalData := make([]MatchEntry, 0)
			for _, entry := range monthlyMatches { finalData = append(finalData, *entry) }

			// 3. BULLETPROOF SORTING (Handles v3 epoch numbers and v4 timestamps)
			sort.Slice(finalData, func(i, j int) bool {
				metaI, _ := finalData[i].Match["metadata"].(map[string]interface{})
				metaJ, _ := finalData[j].Match["metadata"].(map[string]interface{})
				
				var timeI, timeJ float64
				if t, ok := metaI["game_start"].(float64); ok { timeI = t }
				if t, ok := metaJ["game_start"].(float64); ok { timeJ = t }
				
				if s, ok := metaI["started_at"].(string); ok {
					if p, e := time.Parse(time.RFC3339, s); e == nil { timeI = float64(p.Unix()) }
				}
				if s, ok := metaJ["started_at"].(string); ok {
					if p, e := time.Parse(time.RFC3339, s); e == nil { timeJ = float64(p.Unix()) }
				}
				return timeI > timeJ
			})

			responseObj := map[string]interface{}{ "data": finalData }
			newBytes, _ := json.Marshal(responseObj)

			cacheMutex.Lock()
			cachedMatchesData = newBytes
			cacheMutex.Unlock()

			os.WriteFile(filePath, newBytes, 0644)
			log.Printf("Background Poller: Updated %s with %d total matches", filePath, len(finalData))
		}
	}()
}
