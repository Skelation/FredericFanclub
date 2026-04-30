package main

import (
	"database/sql"
	"io"
	"log"
	"net/http"
	"net/url"
	"math/rand"
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
	"github.com/gorilla/websocket"
)

var (
	CurrentMarket *PropMarket
	DB          *sql.DB 
	oauthConfig *oauth2.Config
	oauthState  = "fred-secure-state-token"

	puuidCache sync.Map // Remembers Player PUUIDs so Riot doesn't rate-limit us
)

type PropMarket struct {
	Player       string  `json:"player"`
	PropType     string  `json:"prop_type"` // "kills" or "deaths"
	Line         float64 `json:"line"`      // e.g., 14.5
	OverMult     float64 `json:"over_multiplier"`
	UnderMult    float64 `json:"under_multiplier"`
	IsOpen       bool    `json:"is_open"`
}

type WSMessage struct {
	Type    string      `json:"type"`              // e.g., "new_bet", "market_resolved"
	Payload interface{} `json:"payload,omitempty"` // The actual data
}

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins for now
	}
	wsClients = make(map[*websocket.Conn]bool) 
	wsMutex   sync.Mutex                       // Prevents concurrent map read/write crashes
	broadcast = make(chan WSMessage)           // The channel we shout events into
)

func main() {
	go func() {
		for {
			msg := <-broadcast // Wait for a new message
			
			wsMutex.Lock() // Lock the doors while we loop through users
			for client := range wsClients {
				err := client.WriteJSON(msg)
				if err != nil {
					client.Close()
					delete(wsClients, client)
				}
			}
			wsMutex.Unlock() // Unlock the doors
		}
	}()

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
		var username, avatar, linkedPlayer string
		var tokens float64
		err = DB.QueryRow("SELECT username, avatar_url, fredtokens, linked_player FROM users WHERE discord_id = ?", cookie.Value).Scan(&username, &avatar, &tokens, &linkedPlayer)

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
		fmt.Fprintf(w, `{"username": "%s", "avatar_url": "%s", "fredtokens": %g, "linked_player": "%s"}`, username, avatarURL, tokens, linkedPlayer)
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

		// --- DYNAMIC COOKIE & REDIRECT FIX ---
		frontendURL := strings.TrimSpace(os.Getenv("FRONTEND_URL"))
		if frontendURL == "" {
			frontendURL = "https://fredericfan.club" // Fallback
		}

		// Check if we are in the Dev Universe
		isDev := strings.Contains(frontendURL, "localhost") || strings.Contains(frontendURL, "127.0.0.1")
		
		cookieDomain := ".fredericfan.club"
		secureCookie := true
		sameSitePolicy := http.SameSiteNoneMode // Default to Prod

		if isDev {
			cookieDomain = ""                     // Localhost doesn't like strict domains
			secureCookie = false                  // Localhost isn't HTTPS
			sameSitePolicy = http.SameSiteLaxMode // CRITICAL: Browsers block SameSite=None without Secure=true!
		}

		// Set the secure (or dev) cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "fred_user_id",
			Value:    discordUser.ID,
			Path:     "/",
			Domain:   cookieDomain,
			HttpOnly: false,
			Secure:   secureCookie,
			SameSite: sameSitePolicy,
			MaxAge:   86400 * 30, 
		})

		// Dynamically redirect them back to wherever they came from
		http.Redirect(w, r, frontendURL+"/", http.StatusTemporaryRedirect)
	})

	// POST: Buy and Open a Card Pack (500 FT)
	// POST: Buy and Open a Card Pack (500 FT)
	mux.HandleFunc("/api/economy/buy-pack", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)

		// 1. Handle Browser Preflight
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// 2. Only allow POST
		if r.Method != http.MethodPost {
			http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		currentUserID := getUserIDFromCookie(r)
		if currentUserID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// 3. Database Column Sync: Use 'fredtokens'
		var userTokens float64
		err := DB.QueryRow("SELECT fredtokens FROM users WHERE discord_id = ?", currentUserID).Scan(&userTokens)
		if err != nil || userTokens < 250 {
			http.Error(w, `{"error": "insufficient funds"}`, http.StatusBadRequest)
			return
		}

		// 4. RNG Logic (Using math/rand)
		roll := rand.Float64()
		fmt.Println(roll)
		var targetRarity string
		// THE TWEAKED ODDS: 0.4% for Radiant
		if roll < 0.004 {         // 0.4% chance (About 1 in 250 packs)
			targetRarity = "radiant" 
		} else if roll < 0.015 {  // 1.1% chance (0.015 - 0.004)
			targetRarity = "immortal" 
		} else if roll < 0.050 {  // 3.5% chance
			targetRarity = "ascendant" 
		} else if roll < 0.150 {  // 10.0% chance
			targetRarity = "diamond" 
		} else if roll < 0.500 {  // 35.0% chance
			targetRarity = "bronze" 
		} else {                  // 50.0% chance
			targetRarity = "iron" 
		}

				// 5. Pull Card
		var cardID int
		var cardName, cardRarity, cardImage string
		err = DB.QueryRow("SELECT id, name, rarity, image_url FROM cards WHERE rarity = ? ORDER BY RANDOM() LIMIT 1", targetRarity).Scan(&cardID, &cardName, &cardRarity, &cardImage)
		
		if err != nil {
			http.Error(w, `{"error": "No cards available for rarity: `+targetRarity+`"}`, http.StatusInternalServerError)
			return
		}

		// 6. Transaction
		tx, _ := DB.Begin()
		_, err = tx.Exec("UPDATE users SET fredtokens = fredtokens - 250 WHERE discord_id = ?", currentUserID)
		if err != nil { tx.Rollback(); http.Error(w, `{"error": "db update failed"}`, 500); return }

		_, err = tx.Exec(`
			INSERT INTO inventory (discord_id, card_id, quantity) 
			VALUES (?, ?, 1)
			ON CONFLICT(discord_id, card_id) DO UPDATE SET quantity = quantity + 1
		`, currentUserID, cardID)
		if err != nil { tx.Rollback(); http.Error(w, `{"error": "inventory update failed"}`, 500); return }

		tx.Commit()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "card": {"name": "%s", "rarity": "%s", "image_url": "%s"}}`, cardName, cardRarity, cardImage)
	})

	// POST: Claim Daily Reward (250 FT every 24 hours)
	mux.HandleFunc("OPTIONS /api/economy/daily", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	// GET: Check if the Daily Reward is available right now
	mux.HandleFunc("GET /api/economy/daily", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		userID := getUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var lastClaimStr sql.NullString
		err := DB.QueryRow("SELECT last_daily_claim FROM users WHERE discord_id = ?", userID).Scan(&lastClaimStr)
		if err != nil {
			http.Error(w, `{"error": "user not found"}`, http.StatusInternalServerError)
			return
		}

		var lastClaim time.Time
		if lastClaimStr.Valid && lastClaimStr.String != "" {
			lastClaim, _ = time.Parse(time.RFC3339, lastClaimStr.String)
		}

		now := time.Now().UTC()
		cooldown := 24 * time.Hour
		timeSinceLastClaim := now.Sub(lastClaim)

		w.Header().Set("Content-Type", "application/json")
		
		// If they are on cooldown, send back the remaining time
		if timeSinceLastClaim < cooldown {
			timeLeft := cooldown - timeSinceLastClaim
			hours := int(timeLeft.Hours())
			minutes := int(timeLeft.Minutes()) % 60
			fmt.Fprintf(w, `{"available": false, "hours": %d, "minutes": %d}`, hours, minutes)
			return
		}

		// Otherwise, it's ready!
		fmt.Fprintf(w, `{"available": true}`)
	})

	mux.HandleFunc("POST /api/economy/daily", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)

		userID := getUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// 1. Get the exact time they last claimed it
		var lastClaimStr sql.NullString
		err := DB.QueryRow("SELECT last_daily_claim FROM users WHERE discord_id = ?", userID).Scan(&lastClaimStr)
		if err != nil {
			http.Error(w, `{"error": "user not found"}`, http.StatusInternalServerError)
			return
		}

		// 2. The Time-Lock Math
		var lastClaim time.Time
		if lastClaimStr.Valid && lastClaimStr.String != "" {
			lastClaim, _ = time.Parse(time.RFC3339, lastClaimStr.String)
		} else {
			lastClaim = time.Time{} // If it's empty, they've never claimed it!
		}

		now := time.Now().UTC()
		cooldown := 24 * time.Hour
		timeSinceLastClaim := now.Sub(lastClaim)

		w.Header().Set("Content-Type", "application/json")

		// 3. If they are too early, reject them with the remaining time
		if timeSinceLastClaim < cooldown {
			timeLeft := cooldown - timeSinceLastClaim
			hours := int(timeLeft.Hours())
			minutes := int(timeLeft.Minutes()) % 60
			
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error": "Come back in %d hours and %d minutes!"}`, hours, minutes)
			return
		}

		// 4. Grant 250 FT and stamp the new time!
		tx, _ := DB.Begin()
		_, err = tx.Exec("UPDATE users SET fredtokens = fredtokens + 250, last_daily_claim = ? WHERE discord_id = ?", now.Format(time.RFC3339), userID)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}
		tx.Commit()

		fmt.Fprintf(w, `{"success": true, "message": "Daily Drop Claimed! +250 FT"}`)
	})

	// --- QUEST DEFINITIONS ---
	type Quest struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Reward      int    `json:"reward"`
		Difficulty  string `json:"difficulty"`
	}

	easyQuests := []Quest{
		{ID: "e1", Title: "Warmup Routine", Description: "Play 2 Competitive matches.", Reward: 150, Difficulty: "easy"},
		{ID: "e2", Title: "Supporting Cast", Description: "Get 15 total Assists across all games.", Reward: 150, Difficulty: "easy"},
		{ID: "e3", Title: "Chip Damage", Description: "Deal 2,500 total damage to enemies.", Reward: 150, Difficulty: "easy"},
		{ID: "e4", Title: "Consistent", Description: "Achieve an Average Combat Score of 150+ in one match.", Reward: 150, Difficulty: "easy"},
	}
	medQuests := []Quest{
		{ID: "m1", Title: "Clicking Heads", Description: "Accumulate 20 total Headshots.", Reward: 250, Difficulty: "medium"},
		{ID: "m2", Title: "Lethal Force", Description: "Get 15+ Kills in a single match.", Reward: 250, Difficulty: "medium"},
		{ID: "m3", Title: "Securing the Bag", Description: "Win 15 rounds total.", Reward: 250, Difficulty: "medium"},
		{ID: "m4", Title: "Positive Impact", Description: "Finish a match with a K/D Ratio above 1.0.", Reward: 250, Difficulty: "medium"},
	}
	hardQuests := []Quest{
		{ID: "h1", Title: "Hard Carry", Description: "Get 25+ Kills in a single match.", Reward: 500, Difficulty: "hard"},
		{ID: "h2", Title: "Flawless Victory", Description: "Win a match by a margin of 5 or more rounds.", Reward: 500, Difficulty: "hard"},
		{ID: "h3", Title: "Combat Medic", Description: "Get 10 Assists and survive 10 rounds in one match.", Reward: 500, Difficulty: "hard"},
		{ID: "h4", Title: "Top Fragger", Description: "Accumulate 50 total kills today.", Reward: 500, Difficulty: "hard"},
	}

	// GET: Fetch Today's Global Quests & User Progress
	mux.HandleFunc("OPTIONS /api/quests", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/quests", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		userID := getUserIDFromCookie(r)

		// 1. THE 2:00 AM RESET LOGIC
		// We take the current UTC time, subtract 2 hours, and format it as a Date.
		// If it is 1:59 AM, subtracting 2 hours makes it yesterday.
		// If it is 2:00 AM, it rolls over to today!
		now := time.Now().UTC()
		effectiveDate := now.Add(-2 * time.Hour).Format("2006-01-02")

		// 2. DETERMINISTIC GENERATION (Everyone gets the same quests)
		// We convert the date string (e.g. "2026-04-27") into a mathematical seed
		seedInt := int64(0)
		for _, char := range effectiveDate { seedInt += int64(char) }
		
		dailyRand := rand.New(rand.NewSource(seedInt))
		
		todaysEasy := easyQuests[dailyRand.Intn(len(easyQuests))]
		todaysMed := medQuests[dailyRand.Intn(len(medQuests))]
		todaysHard := hardQuests[dailyRand.Intn(len(hardQuests))]

		// 3. CHECK USER'S CLAIM STATUS
		easyClaimed, medClaimed, hardClaimed := false, false, false
		if userID != "" {
			// Insert a blank row for them today if they don't have one
			DB.Exec("INSERT OR IGNORE INTO user_quests (discord_id, quest_date) VALUES (?, ?)", userID, effectiveDate)
			
			DB.QueryRow("SELECT easy_claimed, med_claimed, hard_claimed FROM user_quests WHERE discord_id = ? AND quest_date = ?", userID, effectiveDate).Scan(&easyClaimed, &medClaimed, &hardClaimed)
		}

		// 4. SEND RESPONSE
		response := map[string]interface{}{
			"date": effectiveDate,
			"quests": []map[string]interface{}{
				{"quest": todaysEasy, "claimed": easyClaimed},
				{"quest": todaysMed, "claimed": medClaimed},
				{"quest": todaysHard, "claimed": hardClaimed},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	// POST: Verify Matches and Claim Mission Rewards
	mux.HandleFunc("OPTIONS /api/quests/verify", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/quests/verify", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		userID := getUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct { Difficulty string `json:"difficulty"` }
		json.NewDecoder(r.Body).Decode(&req)

		// 1. Get the user's Linked Riot Account
		var linkedPlayer string
		err := DB.QueryRow("SELECT linked_player FROM users WHERE discord_id = ?", userID).Scan(&linkedPlayer)
		if err != nil || linkedPlayer == "none" || linkedPlayer == "" {
			http.Error(w, `{"error": "Discord not linked! Press Shift+A to open the Admin Panel and link your account."}`, http.StatusBadRequest)
			return
		}

		// 2. Determine today's date and the EXACT 2:00 AM UTC cutoff timestamp
		now := time.Now().UTC()
		effectiveDate := now.Add(-2 * time.Hour).Format("2006-01-02")
		resetTime, _ := time.Parse("2006-01-02", effectiveDate)
		resetTime = resetTime.Add(2 * time.Hour)

		// 3. Check if they already claimed this difficulty today
		var claimed bool
		claimColumn := ""
		if req.Difficulty == "easy" { claimColumn = "easy_claimed" }
		if req.Difficulty == "medium" { claimColumn = "med_claimed" }
		if req.Difficulty == "hard" { claimColumn = "hard_claimed" }
		
		if claimColumn == "" {
			http.Error(w, `{"error": "Invalid difficulty"}`, http.StatusBadRequest)
			return
		}

		DB.QueryRow(fmt.Sprintf("SELECT %s FROM user_quests WHERE discord_id = ? AND quest_date = ?", claimColumn), userID, effectiveDate).Scan(&claimed)
		if claimed {
			http.Error(w, `{"error": "You already claimed this mission today!"}`, http.StatusBadRequest)
			return
		}

		// 4. Figure out WHAT the quest is today using the Deterministic Seed
		seedInt := int64(0)
		for _, char := range effectiveDate { seedInt += int64(char) }
		dailyRand := rand.New(rand.NewSource(seedInt))
		
		todaysEasy := easyQuests[dailyRand.Intn(len(easyQuests))]
		todaysMed := medQuests[dailyRand.Intn(len(medQuests))]
		todaysHard := hardQuests[dailyRand.Intn(len(hardQuests))]

		var activeQuest Quest
		if req.Difficulty == "easy" { activeQuest = todaysEasy }
		if req.Difficulty == "medium" { activeQuest = todaysMed }
		if req.Difficulty == "hard" { activeQuest = todaysHard }

		// --- 5. THE CENSORSHIP BYPASS (PUUID LOOKUP CACHE) ---
		roster := []struct{ Name, Tag string }{
			{"TheMisterED", "0007"}, {"Heri", "BLUB"}, {"hhj", "8769"},
			{"Djibはコリーヌ お あいして", "LOVE"}, {"Graussbyt", "5629"},
			{"Lal6s9gne", "6641"}, {"XTrixツ", "DREAM"}, {"小胖子vincent", "4397"},
		}

		var playerTag string
		for _, r := range roster {
			if strings.EqualFold(r.Name, linkedPlayer) {
				playerTag = r.Tag
				break
			}
		}

		cacheKey := strings.ToLower(linkedPlayer + "#" + playerTag)
		targetPuuid := ""
		
		// Check Memory Cache first so Riot doesn't block us!
		if val, ok := puuidCache.Load(cacheKey); ok {
			targetPuuid = val.(string)
		} else {
			accountURL := fmt.Sprintf("%s/v1/account/%s/%s", base, url.PathEscape(linkedPlayer), url.PathEscape(playerTag))
			reqAcc, _ := http.NewRequest("GET", accountURL, nil)
			if apiKey != "" { reqAcc.Header.Set("Authorization", apiKey) }
			respAcc, errAcc := http.DefaultClient.Do(reqAcc)
			if errAcc == nil && respAcc.StatusCode == 200 {
				var accData struct { Data struct { Puuid string `json:"puuid"` } `json:"data"` }
				json.NewDecoder(respAcc.Body).Decode(&accData)
				targetPuuid = accData.Data.Puuid
				puuidCache.Store(cacheKey, targetPuuid) // Save it for next time!
				respAcc.Body.Close()
			}
		}

		if targetPuuid == "" {
			http.Error(w, `{"error": "Riot API is currently overloaded. Please try verifying again in 60 seconds."}`, http.StatusInternalServerError)
			return
		}
		// -----------------------------------------------------

		// 6. READ THE LOCAL CACHE (No slow API calls!)
		cacheMutex.RLock()
		dataBytes := cachedMatchesData
		cacheMutex.RUnlock()

		if len(dataBytes) == 0 {
			http.Error(w, `{"error": "Match history is warming up. Try again in 1 minute."}`, http.StatusServiceUnavailable)
			return
		}

		var cacheData struct { Data []struct { Match map[string]interface{} `json:"match"` } `json:"data"` }
		json.Unmarshal(dataBytes, &cacheData)

		// --- STAT AGGREGATORS ---
		totalMatches := 0
		totalAssists, totalDamage, totalHeadshots, totalKills, totalRoundsWon, totalScore, totalRoundsPlayed := 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0
		lethalForce, positiveImpact, hardCarry, flawlessVictory, combatMedic := false, false, false, false, false

		// 7. SCAN MATCHES
		for _, m := range cacheData.Data {
			meta, _ := m.Match["metadata"].(map[string]interface{})
			
			// Extract time
			var matchTime float64
			if t, ok := meta["game_start"].(float64); ok { matchTime = t }
			if s, ok := meta["started_at"].(string); ok {
				if p, e := time.Parse(time.RFC3339, s); e == nil { matchTime = float64(p.Unix()) }
			}

			// ONLY count matches played AFTER 2:00 AM today!
			if matchTime < float64(resetTime.Unix()) {
				continue 
			}

			var allPlayers []interface{}
			if pMap, ok := m.Match["players"].(map[string]interface{}); ok { allPlayers, _ = pMap["all_players"].([]interface{})
			} else if pArr, ok := m.Match["players"].([]interface{}); ok { allPlayers = pArr }

			for _, p := range allPlayers {
				playerMap, ok := p.(map[string]interface{})
				if !ok { continue }

				puuid, _ := playerMap["puuid"].(string)
				
				if strings.EqualFold(puuid, targetPuuid) {
					totalMatches++
					stats, _ := playerMap["stats"].(map[string]interface{})
					
					k, _ := stats["kills"].(float64)
					d, _ := stats["deaths"].(float64)
					a, _ := stats["assists"].(float64)
					dmg, _ := stats["damage"].(float64)
					hs, _ := stats["headshots"].(float64)
					score, _ := stats["score"].(float64)

					// --- THE FIX: Look for team_id first, then fallback to team ---
					myTeamName, _ := playerMap["team_id"].(string)
					if myTeamName == "" {
						myTeamName, _ = playerMap["team"].(string)
					}
					// --------------------------------------------------------------

					myRoundsWon, enemyRoundsWon := 0.0, 0.0
					
					// --- THE V3 vs V4 FIX: Handle both Array and Map API formats! ---
					if teamsArr, ok := m.Match["teams"].([]interface{}); ok {
						// Newer V4 API Format (Array)
						for _, t := range teamsArr {
							tData, _ := t.(map[string]interface{})
							tID, _ := tData["team_id"].(string)
							rWon, _ := tData["rounds_won"].(float64)
							if strings.EqualFold(tID, myTeamName) { 
								myRoundsWon = rWon 
							} else { 
								enemyRoundsWon = rWon 
							}
						}
					} else if teamsMap, ok := m.Match["teams"].(map[string]interface{}); ok {
						// Older V3 API Format (Object/Map)
						if redTeam, ok := teamsMap["red"].(map[string]interface{}); ok {
							if blueTeam, ok := teamsMap["blue"].(map[string]interface{}); ok {
								redRounds, _ := redTeam["rounds_won"].(float64)
								blueRounds, _ := blueTeam["rounds_won"].(float64)

								if strings.EqualFold(myTeamName, "Red") {
									myRoundsWon = redRounds
									enemyRoundsWon = blueRounds
								} else {
									myRoundsWon = blueRounds
									enemyRoundsWon = redRounds
								}
							}
						}
					}
					// ----------------------------------------------------------------
					
					// --- THE TRIPWIRES: Print the exact math to the console ---
					log.Printf("[DEBUG] Match Found! PUUID: %s", puuid)
					log.Printf("[DEBUG] K/D/A: %.0f/%.0f/%.0f | Team: %s", k, d, a, myTeamName)
					log.Printf("[DEBUG] Score: My Team %.0f - %.0f Enemy Team", myRoundsWon, enemyRoundsWon)
					// ----------------------------------------------------------
					
					totalKills += k
					totalAssists += a
					totalDamage += dmg
					totalHeadshots += hs
					totalScore += score

					totalRoundsWon += myRoundsWon
					roundsPlayed := myRoundsWon + enemyRoundsWon
					totalRoundsPlayed += roundsPlayed

					if k >= 15 { lethalForce = true }
					if k >= 25 { hardCarry = true }
					if d == 0 { d = 1 } 
					if (k / d) > 1.0 { positiveImpact = true }
					if (myRoundsWon - enemyRoundsWon) >= 5 { flawlessVictory = true }
					if a >= 10 && (roundsPlayed - d) >= 10 { combatMedic = true }
					
					break 
				}
			}
			
		}

		// 8. EVALUATE THE QUEST
		passed := false
		progressMsg := ""

		switch activeQuest.Title {
		case "Warmup Routine": passed = totalMatches >= 2; progressMsg = fmt.Sprintf("%d/2 Matches Played Today", totalMatches)
		case "Supporting Cast": passed = totalAssists >= 15; progressMsg = fmt.Sprintf("%.0f/15 Assists Today", totalAssists)
		case "Chip Damage": passed = totalDamage >= 2500; progressMsg = fmt.Sprintf("%.0f/2500 Damage Today", totalDamage)
		case "Consistent": 
			acs := 0.0
			if totalRoundsPlayed > 0 { acs = totalScore / totalRoundsPlayed }
			passed = acs >= 150; progressMsg = fmt.Sprintf("Current ACS: %.0f (Need 150)", acs)
		
		case "Clicking Heads": passed = totalHeadshots >= 20; progressMsg = fmt.Sprintf("%.0f/20 Headshots Today", totalHeadshots)
		case "Lethal Force": passed = lethalForce; progressMsg = "Requires 15+ kills in one match today."
		case "Securing the Bag": passed = totalRoundsWon >= 15; progressMsg = fmt.Sprintf("%.0f/15 Rounds Won Today", totalRoundsWon)
		case "Positive Impact": passed = positiveImpact; progressMsg = "Requires a >1.0 KD in a match today."

		case "Hard Carry": passed = hardCarry; progressMsg = "Requires 25+ kills in one match today."
		case "Flawless Victory": passed = flawlessVictory; progressMsg = "Requires winning by 5+ rounds today."
		case "Combat Medic": passed = combatMedic; progressMsg = "Requires 10 assists & 10 rounds survived today."
		case "Top Fragger": passed = totalKills >= 50; progressMsg = fmt.Sprintf("%.0f/50 Kills Today", totalKills)
		}

			// 9. PAYOUT OR REJECT
		if !passed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error": "Mission not complete. %s"}`, progressMsg)
			return
		}

		tx, _ := DB.Begin()
		tx.Exec(fmt.Sprintf("UPDATE user_quests SET %s = 1 WHERE discord_id = ? AND quest_date = ?", claimColumn), userID, effectiveDate)
		tx.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", activeQuest.Reward, userID)
		tx.Commit()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Mission Accomplished! +%d FT", "reward": %d}`, activeQuest.Reward, activeQuest.Reward)
	})

	// GET: Fetch a user's card inventory
	mux.HandleFunc("OPTIONS /api/inventory", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/inventory", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		
		userID := getUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// ADDED: c.id (so we can shred it) AND i.quantity > 0 (so empty cards vanish)
		query := `
			SELECT c.id, c.name, c.rarity, c.image_url, i.quantity
			FROM inventory i
			JOIN cards c ON i.card_id = c.id
			WHERE i.discord_id = ? AND i.quantity > 0
			ORDER BY 
				CASE c.rarity 
					WHEN 'radiant' THEN 1 
					WHEN 'immortal' THEN 2 
					WHEN 'ascendant' THEN 3 
					WHEN 'diamond' THEN 4 
					WHEN 'bronze' THEN 5 
					WHEN 'iron' THEN 6 
					ELSE 7 
				END, c.name ASC`

		rows, err := DB.Query(query, userID)
		if err != nil {
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type InventoryItem struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			Rarity   string `json:"rarity"`
			ImageURL string `json:"image_url"`
			Quantity int    `json:"quantity"`
		}

		var items []InventoryItem
		for rows.Next() {
			var item InventoryItem
			rows.Scan(&item.ID, &item.Name, &item.Rarity, &item.ImageURL, &item.Quantity)
			items = append(items, item)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})

	// GET: Fetch the COMPLETE catalog of all cards and check if the user owns them
	mux.HandleFunc("OPTIONS /api/catalog", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/catalog", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		
		userID := getUserIDFromCookie(r)
		// Note: If userID is empty (not logged in), the LEFT JOIN still works perfectly, 
		// it will just return 0 quantity for everything (all locked)!

		query := `
			SELECT c.id, c.name, c.rarity, c.image_url, c.season, COALESCE(i.quantity, 0) as quantity
			FROM cards c
			LEFT JOIN inventory i ON c.id = i.card_id AND i.discord_id = ?
			ORDER BY c.season ASC, 
				CASE c.rarity 
					WHEN 'radiant' THEN 1 
					WHEN 'immortal' THEN 2 
					WHEN 'ascendant' THEN 3 
					WHEN 'diamond' THEN 4 
					WHEN 'bronze' THEN 5 
					WHEN 'iron' THEN 6 
					ELSE 7 
				END, c.name ASC`

		rows, err := DB.Query(query, userID)
		if err != nil {
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type CatalogItem struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			Rarity   string `json:"rarity"`
			ImageURL string `json:"image_url"`
			Season   string `json:"season"` // NEW!
			Unlocked bool   `json:"unlocked"`
		}

		var items []CatalogItem
		for rows.Next() {
			var item CatalogItem
			var quantity int
			rows.Scan(&item.ID, &item.Name, &item.Rarity, &item.ImageURL, &item.Season, &quantity)
			item.Unlocked = quantity > 0
			items = append(items, item)
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})

	// POST: CS:GO Style Trade-Up Contract (5 -> 1)
	mux.HandleFunc("OPTIONS /api/economy/trade-up", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/economy/trade-up", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		userID := getUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct { CardIDs []int `json:"card_ids"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		if len(req.CardIDs) != 5 {
			http.Error(w, `{"error": "You must provide exactly 5 cards!"}`, http.StatusBadRequest)
			return
		}

		// Count how many of each specific card they are trying to burn
		required := make(map[int]int)
		for _, id := range req.CardIDs {
			required[id]++
		}

		tx, _ := DB.Begin()
		defer tx.Rollback() // Safety net: aborts everything if we don't hit tx.Commit()

		var commonRarity string

		// 1. Verify ownership, quantity, and uniform rarity
		for cardID, qtyNeeded := range required {
			var ownedQty int
			var rarity string
			err := tx.QueryRow("SELECT i.quantity, c.rarity FROM inventory i JOIN cards c ON i.card_id = c.id WHERE i.discord_id = ? AND i.card_id = ?", userID, cardID).Scan(&ownedQty, &rarity)
			
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

		// 2. Determine the NEXT rarity tier
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

		// 3. Pull 1 random card of the next rarity
		var winCardID int
		var winName, winRarity, winImage string
		err := tx.QueryRow("SELECT id, name, rarity, image_url FROM cards WHERE rarity = ? ORDER BY RANDOM() LIMIT 1", nextRarity).Scan(&winCardID, &winName, &winRarity, &winImage)
		
		if err != nil {
			http.Error(w, `{"error": "The database has no cards in the next tier to give you!"}`, http.StatusInternalServerError)
			return
		}

		// 4. Burn the 5 sacrificed cards
		for cardID, qtyNeeded := range required {
			_, err = tx.Exec("UPDATE inventory SET quantity = quantity - ? WHERE discord_id = ? AND card_id = ?", qtyNeeded, userID, cardID)
			if err != nil {
				http.Error(w, `{"error": "Failed to burn cards"}`, http.StatusInternalServerError)
				return
			}
		}

		// 5. Add the new Upgraded card
		_, err = tx.Exec(`
			INSERT INTO inventory (discord_id, card_id, quantity) 
			VALUES (?, ?, 1)
			ON CONFLICT(discord_id, card_id) DO UPDATE SET quantity = quantity + 1
		`, userID, winCardID)
		
		if err != nil {
			http.Error(w, `{"error": "Failed to grant new card"}`, http.StatusInternalServerError)
			return
		}

		tx.Commit()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "card": {"name": "%s", "rarity": "%s", "image_url": "%s"}}`, winName, winRarity, winImage)
	})

	// GET: Fetch the Leaderboards (Top Tokens and Top Collectors)
	mux.HandleFunc("OPTIONS /api/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)

		type LeaderboardUser struct {
			Username string  `json:"username"`
			Avatar   string  `json:"avatar"`
			Score    float64 `json:"score"`
		}

		type LeaderboardRes struct {
			TopTokens []LeaderboardUser `json:"top_tokens"`
			TopCards  []LeaderboardUser `json:"top_cards"`
		}

		res := LeaderboardRes{
			TopTokens: make([]LeaderboardUser, 0),
			TopCards:  make([]LeaderboardUser, 0),
		}

		// 1. Get Top 10 Richest Users (Fredtokens)
		rows1, err := DB.Query("SELECT discord_id, username, avatar_url, fredtokens FROM users ORDER BY fredtokens DESC LIMIT 10")
		if err == nil {
			for rows1.Next() {
				var id, name, avatarHash string
				var tokens float64
				rows1.Scan(&id, &name, &avatarHash, &tokens)
				
				avatar := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", id, avatarHash)
				if avatarHash == "" { avatar = "https://cdn.discordapp.com/embed/avatars/0.png" }
				
				res.TopTokens = append(res.TopTokens, LeaderboardUser{Username: name, Avatar: avatar, Score: tokens})
			}
			rows1.Close()
		}

		// 2. Get Top 10 Card Collectors (Sum of all quantities in inventory)
		// 2. Get Top 10 Card Collectors (Count UNIQUE cards they currently own)
		rows2, err := DB.Query(`
			SELECT u.discord_id, u.username, u.avatar_url, COUNT(i.card_id) as total_cards 
			FROM users u 
			JOIN inventory i ON u.discord_id = i.discord_id 
			WHERE i.quantity > 0
			GROUP BY u.discord_id 
			ORDER BY total_cards DESC 
			LIMIT 10
		`)
		if err == nil {
			for rows2.Next() {
				var id, name, avatarHash string
				var cards float64 // COUNT() returns a number we can store as float64 to match our struct
				rows2.Scan(&id, &name, &avatarHash, &cards)
				
				avatar := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", id, avatarHash)
				if avatarHash == "" { avatar = "https://cdn.discordapp.com/embed/avatars/0.png" }
				
				res.TopCards = append(res.TopCards, LeaderboardUser{Username: name, Avatar: avatar, Score: cards})
			}
			rows2.Close()
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	})

	// POST: Shred a card for Fredtokens
	mux.HandleFunc("OPTIONS /api/economy/shred", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/economy/shred", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		
		userID := getUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct { CardID int `json:"card_id"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		// 1. Verify they own the card and get its rarity
		var rarity string
		var quantity int
		err := DB.QueryRow(`
			SELECT c.rarity, i.quantity 
			FROM inventory i 
			JOIN cards c ON i.card_id = c.id 
			WHERE i.discord_id = ? AND i.card_id = ? AND i.quantity > 0`, 
			userID, req.CardID).Scan(&rarity, &quantity)

		if err != nil {
			http.Error(w, `{"error": "You do not own this card!"}`, http.StatusBadRequest)
			return
		}

		// 2. The Exchange Rates
		payouts := map[string]int{
			"iron": 20,
			"bronze": 50,
			"diamond": 200,
			"ascendant": 500,
			"immortal": 2500,
			"radiant": 10000,
		}
		payoutAmount := payouts[rarity]

		// 3. The Transaction
		tx, _ := DB.Begin()
		
		// Deduct 1 card
		_, err = tx.Exec("UPDATE inventory SET quantity = quantity - 1 WHERE discord_id = ? AND card_id = ?", userID, req.CardID)
		if err != nil { tx.Rollback(); http.Error(w, `{"error": "db failed"}`, 500); return }

		// Add Tokens
		_, err = tx.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", payoutAmount, userID)
		if err != nil { tx.Rollback(); http.Error(w, `{"error": "db failed"}`, 500); return }

		tx.Commit()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "payout": %d, "remaining": %d}`, payoutAmount, quantity-1)
	})

	mux.HandleFunc("GET /api/ws/betting", func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil { return }
		defer ws.Close()
		
		// Register the new user safely
		wsMutex.Lock()       
		wsClients[ws] = true 
		wsMutex.Unlock()     
		
		// Keep the connection alive until they leave the page
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				// User disconnected, remove them safely
				wsMutex.Lock()        
				delete(wsClients, ws) 
				wsMutex.Unlock()      
				break
			}
		}
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
		var linkedPlayer string // Check who they are!
		err = tx.QueryRow("SELECT fredtokens, linked_player FROM users WHERE discord_id = ?", discordID).Scan(&balance, &linkedPlayer)
		if err != nil || balance < req.Amount {
			tx.Rollback()
			http.Error(w, `{"error": "Not enough Fredtokens!"}`, http.StatusBadRequest)
			return
		}

		// --- ANTI-CORRUPTION ENGINE ---
		if linkedPlayer != "none" {
			// Rule 1: Cannot bet on yourself
			if strings.EqualFold(linkedPlayer, CurrentMarket.Player) {
				tx.Rollback()
				http.Error(w, `{"error": "Conflict of Interest: You cannot bet on your own performance!"}`, http.StatusForbidden)
				return
			}
			// Rule 2: Roster players cannot bet on Team Matches
			if CurrentMarket.Player == "FRED ESPORTS" {
				tx.Rollback()
				http.Error(w, `{"error": "Conflict of Interest: Roster players cannot bet on team matches!"}`, http.StatusForbidden)
				return
			}
		}

		// --- EXECUTE THE BET ---
		// 1. Deduct Tokens
		_, err = tx.Exec("UPDATE users SET fredtokens = fredtokens - ? WHERE discord_id = ?", req.Amount, discordID)
		
		// 2. Insert Detailed Bet Ticket
		result, err := tx.Exec(`INSERT INTO bets 
			(discord_id, bet_category, target_player, prop_type, line_value, choice, amount, locked_multiplier) 
			VALUES (?, 'prop', ?, ?, ?, ?, ?, ?)`, 
			discordID, CurrentMarket.Player, CurrentMarket.PropType, CurrentMarket.Line, req.Choice, req.Amount, lockedMultiplier)
		
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "Failed to place bet"}`, http.StatusInternalServerError)
			return
		}

		betID, _ := result.LastInsertId()
		tx.Commit()

		// --- NEW: WEBSOCKET BROADCAST ---
		var username, avatarHash string
		DB.QueryRow("SELECT username, avatar_url FROM users WHERE discord_id = ?", discordID).Scan(&username, &avatarHash)
		avatarURL := fmt.Sprintf("https://cdn.discordapp.com/avatars/%s/%s.png", discordID, avatarHash)
		if avatarHash == "" { avatarURL = "https://cdn.discordapp.com/embed/avatars/0.png" }

		broadcast <- WSMessage{
			Type: "new_bet",
			Payload: map[string]interface{}{
				"id":       betID,
				"username": username,
				"avatar":   avatarURL,
				"choice":   req.Choice,
				"amount":   req.Amount,
			},
		}
		// --------------------------------

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "new_balance": %g}`, balance-req.Amount)
	})

// ADMIN: Add a new trading card to the catalog
	mux.HandleFunc("OPTIONS /api/admin/cards", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/admin/cards", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		
		// Security Check: Only the Admin can add cards!
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct {
			Name     string `json:"name"`
			Rarity   string `json:"rarity"` 
			ImageURL string `json:"image_url"`
			Season   string `json:"season"` // NEW!
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		// Default to Season 1 if they forget to type it
		if req.Season == "" { req.Season = "Season 1" }

		// Insert the new card into the catalog
		result, err := DB.Exec("INSERT INTO cards (name, rarity, image_url, season) VALUES (?, ?, ?, ?)", req.Name, req.Rarity, req.ImageURL, req.Season)
		if err != nil {
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}

		newID, _ := result.LastInsertId()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Card added!", "card_id": %d}`, newID)
	})

	// ADMIN: Delete a trading card permanently
	mux.HandleFunc("OPTIONS /api/admin/delete-card", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/admin/delete-card", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct { CardID int `json:"card_id"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		// Use a transaction to safely delete it from BOTH tables
		tx, _ := DB.Begin()
		
		// 1. Remove from all user inventories so the app doesn't crash trying to load a ghost card
		_, err := tx.Exec("DELETE FROM inventory WHERE card_id = ?", req.CardID)
		if err != nil { tx.Rollback(); http.Error(w, `{"error": "db error on inventory"}`, 500); return }

		// 2. Delete the card itself from the catalog
		_, err = tx.Exec("DELETE FROM cards WHERE id = ?", req.CardID)
		if err != nil { tx.Rollback(); http.Error(w, `{"error": "db error on cards"}`, 500); return }

		tx.Commit()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Card permanently deleted!"}`)
	})

	// ADMIN: Give Fredtokens to a specific user
	mux.HandleFunc("OPTIONS /api/admin/tokens", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/admin/tokens", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		
		// Security Check: Only the Admin can mint tokens!
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
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

		// Inject the tokens into the user's balance
		// Note: Ensure your column is actually named 'balance' or 'tokens' based on your DB schema!
		result, err := DB.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", req.Amount, req.DiscordID)
		if err != nil {
			log.Println("Maybe Balance should be changed to fredtokens")
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}

		// Check if the user actually exists
		rowsAffected, _ := result.RowsAffected()
		if rowsAffected == 0 {
			http.Error(w, `{"error": "User not found"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Successfully minted %d FT for user %s"}`, req.Amount, req.DiscordID)
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

		// --- NEW: THE CENSORSHIP BYPASS (PUUID LOOKUP) ---
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
		
		var playerTag string
		for _, r := range roster {
			if strings.EqualFold(r.Name, req.Player) {
				playerTag = r.Tag
				break
			}
		}

		accountURL := fmt.Sprintf("%s/v1/account/%s/%s", base, url.PathEscape(req.Player), url.PathEscape(playerTag))
		reqAcc, _ := http.NewRequest("GET", accountURL, nil)
		if apiKey != "" { reqAcc.Header.Set("Authorization", apiKey) }
		
		respAcc, errAcc := http.DefaultClient.Do(reqAcc)
		var targetPuuid string
		if errAcc == nil && respAcc.StatusCode == 200 {
			var accData struct { Data struct { Puuid string `json:"puuid"` } `json:"data"` }
			json.NewDecoder(respAcc.Body).Decode(&accData)
			targetPuuid = accData.Data.Puuid
			respAcc.Body.Close()
		}

		if targetPuuid == "" {
			http.Error(w, `{"error": "Failed to look up player PUUID for preview."}`, http.StatusBadRequest)
			return
		}

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
				
				// --- CHANGED: Check PUUID instead of Name! ---
				puuid, _ := playerMap["puuid"].(string)
				if strings.EqualFold(puuid, targetPuuid) {
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

	// ADMIN: Link a Discord User to a Roster Player
	mux.HandleFunc("OPTIONS /api/admin/link-user", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/admin/link-user", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct {
			DiscordID string `json:"discord_id"`
			Player    string `json:"player"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		_, err := DB.Exec("UPDATE users SET linked_player = ? WHERE discord_id = ?", req.Player, req.DiscordID)
		if err != nil {
			http.Error(w, `{"error": "database error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "User linked successfully!"}`)
	})

	// ADMIN: Get all users to populate the dropdown
	
	// --- FIX: Added the missing CORS Preflight route! ---
	mux.HandleFunc("OPTIONS /api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/admin/users", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		rows, err := DB.Query("SELECT discord_id, username, linked_player FROM users")
		if err != nil {
			http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		type UserObj struct { DiscordID, Username, Linked string }
		users := make([]UserObj, 0) // FIX: Prevents a 'null' array crash
		for rows.Next() {
			var u UserObj
			rows.Scan(&u.DiscordID, &u.Username, &u.Linked)
			users = append(users, u)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(users)
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

		broadcast <- WSMessage{Type: "market_published"}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Market is now LIVE!"}`)
	})

	// ADMIN: Abort the entire market and Mass Refund everyone!
	mux.HandleFunc("OPTIONS /api/admin/cancel-market", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/admin/cancel-market", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		if CurrentMarket == nil {
			http.Error(w, `{"error": "No active market to cancel."}`, http.StatusBadRequest)
			return
		}

		tx, err := DB.Begin()
		if err != nil {
			http.Error(w, `{"error": "Server error"}`, http.StatusInternalServerError)
			return
		}

		// Fetch ALL pending bets for the active market
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

			// Mass Refund! Loop through and give everyone their money back
			for _, b := range bets {
				tx.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", b.Amount, b.Discord)
				tx.Exec("UPDATE bets SET status = 'cancelled' WHERE id = ?", b.ID)
			}
		}

		// Wipe the active market from the server
		CurrentMarket = nil
		tx.Commit()

		broadcast <- WSMessage{Type: "market_cancelled"}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Market Aborted! All tokens refunded."}`)
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

		broadcast <- WSMessage{Type: "market_locked"}
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

		broadcast <- WSMessage{Type: "market_resolved"}
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
    
	// NEW: Check the .env file for a database path. Default to fred.db if none exists.
	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	fmt.Println(dbPath)
	if dbPath == "" {
		dbPath = "./database/fred.db"
	}

	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	createUsersTable := `
	CREATE TABLE IF NOT EXISTS users (
		discord_id TEXT PRIMARY KEY,
		username TEXT,
		avatar_url TEXT,
		fredtokens INTEGER DEFAULT 1000,
		linked_player TEXT DEFAULT 'none', -- NEW: Links account to roster player!
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

	createCardsTable := `
	CREATE TABLE IF NOT EXISTS cards (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		rarity TEXT,       -- 'blue', 'purple', 'pink', 'red'
		image_url TEXT
	);`

	createInventoryTable := `
	CREATE TABLE IF NOT EXISTS inventory (
		discord_id TEXT,
		card_id INTEGER,
		quantity INTEGER DEFAULT 0,
		PRIMARY KEY (discord_id, card_id),
		FOREIGN KEY(discord_id) REFERENCES users(discord_id),
		FOREIGN KEY(card_id) REFERENCES cards(id)
	);`

	createQuestsTable := `
	CREATE TABLE IF NOT EXISTS user_quests (
		discord_id TEXT,
		quest_date TEXT,          -- Stores the specific day (e.g. "2026-04-27")
		easy_claimed BOOLEAN DEFAULT 0,
		med_claimed BOOLEAN DEFAULT 0,
		hard_claimed BOOLEAN DEFAULT 0,
		PRIMARY KEY (discord_id, quest_date)
	);`


	_, err = DB.Exec(createUsersTable)
	if err != nil {
		log.Fatal("Failed to create users table:", err)
	}

	_, err = DB.Exec(createBetsTable)
	if err != nil {
		log.Fatal("Failed to create bets table:", err)
	}

	// --- NEW CARD TABLES ---
	_, err = DB.Exec(createCardsTable)
	if err != nil { log.Fatal("Failed to create cards table:", err) }

	_, err = DB.Exec(createInventoryTable)
	if err != nil { log.Fatal("Failed to create inventory table:", err) }

	_, err = DB.Exec(createQuestsTable)
	if err != nil { log.Fatal("Failed to create quests table:", err) }

	// --- DATABASE MIGRATION ---
	DB.Exec("ALTER TABLE cards ADD COLUMN season TEXT DEFAULT 'Season 1'")
	// NEW: Add a column to track the exact time of their last daily claim
	DB.Exec("ALTER TABLE users ADD COLUMN last_daily_claim TEXT DEFAULT ''")

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
		return []string{
			"http://localhost:3000", 
			"http://127.0.0.1:3000", 
			"https://fredericfan.club",
			"https://www.fredericfan.club",
		}
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

func getUserIDFromCookie(r *http.Request) string {
	cookie, err := r.Cookie("fred_user_id")
	if err != nil {
		return ""
	}
	return cookie.Value
}
