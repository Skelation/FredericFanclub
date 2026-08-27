package quests

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/matches"
	"fredericfanclub/server/internal/middleware"
)

type Quest struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Reward      int    `json:"reward"`
	Difficulty  string `json:"difficulty"`
}

var easyQuests = []Quest{
	{ID: "e1", Title: "Warmup Routine", Description: "Play 2 Competitive matches.", Reward: 150, Difficulty: "easy"},
	{ID: "e2", Title: "Supporting Cast", Description: "Get 15 total Assists across all games.", Reward: 150, Difficulty: "easy"},
	{ID: "e3", Title: "Chip Damage", Description: "Deal 2,500 total damage to enemies.", Reward: 150, Difficulty: "easy"},
	{ID: "e4", Title: "Consistent", Description: "Achieve an Average Combat Score of 150+ in one match.", Reward: 150, Difficulty: "easy"},
}

var medQuests = []Quest{
	{ID: "m1", Title: "Clicking Heads", Description: "Accumulate 20 total Headshots.", Reward: 250, Difficulty: "medium"},
	{ID: "m2", Title: "Lethal Force", Description: "Get 15+ Kills in a single match.", Reward: 250, Difficulty: "medium"},
	{ID: "m3", Title: "Securing the Bag", Description: "Win 15 rounds total.", Reward: 250, Difficulty: "medium"},
	{ID: "m4", Title: "Positive Impact", Description: "Finish a match with a K/D Ratio above 1.0.", Reward: 250, Difficulty: "medium"},
}

var hardQuests = []Quest{
	{ID: "h1", Title: "Hard Carry", Description: "Get 25+ Kills in a single match.", Reward: 500, Difficulty: "hard"},
	{ID: "h2", Title: "Flawless Victory", Description: "Win a match by a margin of 5 or more rounds.", Reward: 500, Difficulty: "hard"},
	{ID: "h3", Title: "Combat Medic", Description: "Get 10 Assists and survive 10 rounds in one match.", Reward: 500, Difficulty: "hard"},
	{ID: "h4", Title: "Top Fragger", Description: "Accumulate 50 total kills today.", Reward: 500, Difficulty: "hard"},
}

func RegisterRoutes(mux *http.ServeMux, allowed []string) {
	mux.HandleFunc("OPTIONS /api/quests", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/quests", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)

		now := time.Now().UTC()
		effectiveDate := now.Add(-2 * time.Hour).Format("2006-01-02")

		seedInt := int64(0)
		for _, char := range effectiveDate {
			seedInt += int64(char)
		}
		dailyRand := rand.New(rand.NewSource(seedInt))

		todaysEasy := easyQuests[dailyRand.Intn(len(easyQuests))]
		todaysMed := medQuests[dailyRand.Intn(len(medQuests))]
		todaysHard := hardQuests[dailyRand.Intn(len(hardQuests))]

		easyClaimed, medClaimed, hardClaimed := false, false, false
		if userID != "" {
			db.DB.Exec("INSERT OR IGNORE INTO user_quests (discord_id, quest_date) VALUES (?, ?)", userID, effectiveDate)
			db.DB.QueryRow(
				"SELECT easy_claimed, med_claimed, hard_claimed FROM user_quests WHERE discord_id = ? AND quest_date = ?",
				userID, effectiveDate).Scan(&easyClaimed, &medClaimed, &hardClaimed)
		}

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

	mux.HandleFunc("OPTIONS /api/quests/verify", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/quests/verify", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct{ Difficulty string `json:"difficulty"` }
		json.NewDecoder(r.Body).Decode(&req)

		var linkedPlayer string
		err := db.DB.QueryRow("SELECT linked_player FROM users WHERE discord_id = ?", userID).Scan(&linkedPlayer)
		if err != nil || linkedPlayer == "none" || linkedPlayer == "" {
			http.Error(w, `{"error": "Discord not linked! Press Shift+A to open the Admin Panel and link your account."}`, http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()
		effectiveDate := now.Add(-2 * time.Hour).Format("2006-01-02")
		resetTime, _ := time.Parse("2006-01-02", effectiveDate)
		resetTime = resetTime.Add(2 * time.Hour)

		var claimed bool
		claimColumn := ""
		if req.Difficulty == "easy" {
			claimColumn = "easy_claimed"
		}
		if req.Difficulty == "medium" {
			claimColumn = "med_claimed"
		}
		if req.Difficulty == "hard" {
			claimColumn = "hard_claimed"
		}
		if claimColumn == "" {
			http.Error(w, `{"error": "Invalid difficulty"}`, http.StatusBadRequest)
			return
		}

		db.DB.QueryRow(
			fmt.Sprintf("SELECT %s FROM user_quests WHERE discord_id = ? AND quest_date = ?", claimColumn),
			userID, effectiveDate).Scan(&claimed)
		if claimed {
			http.Error(w, `{"error": "You already claimed this mission today!"}`, http.StatusBadRequest)
			return
		}

		seedInt := int64(0)
		for _, char := range effectiveDate {
			seedInt += int64(char)
		}
		dailyRand := rand.New(rand.NewSource(seedInt))
		todaysEasy := easyQuests[dailyRand.Intn(len(easyQuests))]
		todaysMed := medQuests[dailyRand.Intn(len(medQuests))]
		todaysHard := hardQuests[dailyRand.Intn(len(hardQuests))]

		var activeQuest Quest
		if req.Difficulty == "easy" {
			activeQuest = todaysEasy
		}
		if req.Difficulty == "medium" {
			activeQuest = todaysMed
		}
		if req.Difficulty == "hard" {
			activeQuest = todaysHard
		}

		targetPuuid := ""
		switch strings.ToLower(linkedPlayer) {
		case "themistered":
			targetPuuid = "be655b44-568d-521c-a1b3-bbdee59dcc56"
		case "heri":
			targetPuuid = "381185ae-9f51-55b9-951a-215949c35e02"
		case "hhj":
			targetPuuid = "e1908072-8690-5298-a41c-c0eecb154bfd"
		case "djibはコリーヌ お あいして", "djib", "djibhouuuuuu":
			targetPuuid = "00b75608-fd12-57c2-a4cf-23466ff42c71"
		case "graussbyt":
			targetPuuid = "8adc42f1-6806-5036-9a3e-94c07349851d"
		case "lal6s9gne":
			targetPuuid = "97d7be3c-67c4-5d9d-9a1d-c382774cba20"
		case "xtrixツ", "xtrix":
			targetPuuid = "7f94422d-6397-5d35-b21c-da98e467f339"
		case "小胖子vincent", "vincent":
			targetPuuid = "e0454220-fc45-5447-82a4-a6e94d326738"
		case "riboox":
			targetPuuid = "c06d3828-ff76-5cab-a282-6a95aaae0215"
		}

		if targetPuuid == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error": "Could not identify your linked player. Check the roster names."}`)
			return
		}

		dataBytes := matches.GetCachedData()

		if len(dataBytes) == 0 {
			http.Error(w, `{"error": "Match history is warming up. Try again in 1 minute."}`, http.StatusServiceUnavailable)
			return
		}

		var cacheData struct {
			Data []struct {
				Match map[string]interface{} `json:"match"`
			} `json:"data"`
		}
		json.Unmarshal(dataBytes, &cacheData)

		totalMatches := 0
		totalAssists, totalDamage, totalHeadshots, totalKills, totalRoundsWon, totalScore, totalRoundsPlayed := 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0
		lethalForce, positiveImpact, hardCarry, flawlessVictory, combatMedic := false, false, false, false, false

		for _, m := range cacheData.Data {
			meta, _ := m.Match["metadata"].(map[string]interface{})

			var matchTime float64
			if t, ok := meta["game_start"].(float64); ok {
				matchTime = t
			}
			if s, ok := meta["started_at"].(string); ok {
				if p, e := time.Parse(time.RFC3339, s); e == nil {
					matchTime = float64(p.Unix())
				}
			}
			if matchTime < float64(resetTime.Unix()) {
				continue
			}

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

				totalMatches++
				stats, _ := playerMap["stats"].(map[string]interface{})
				k, _ := stats["kills"].(float64)
				d, _ := stats["deaths"].(float64)
				a, _ := stats["assists"].(float64)
				hs, _ := stats["headshots"].(float64)
				score, _ := stats["score"].(float64)

				var dmg float64
				if dVal, ok := playerMap["damage_made"].(float64); ok {
					dmg = dVal
				}

				myTeamName, _ := playerMap["team_id"].(string)
				if myTeamName == "" {
					myTeamName, _ = playerMap["team"].(string)
				}

				myRoundsWon, enemyRoundsWon := 0.0, 0.0
				if teamsArr, ok := m.Match["teams"].([]interface{}); ok {
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

				log.Printf("[Quest] Match found — PUUID: %s, K/D/A: %.0f/%.0f/%.0f, Score: %.0f-%.0f",
					puuid, k, d, a, myRoundsWon, enemyRoundsWon)

				totalKills += k
				totalAssists += a
				totalDamage += dmg
				totalHeadshots += hs
				totalScore += score
				totalRoundsWon += myRoundsWon
				roundsPlayed := myRoundsWon + enemyRoundsWon
				totalRoundsPlayed += roundsPlayed

				if k >= 15 {
					lethalForce = true
				}
				if k >= 25 {
					hardCarry = true
				}
				if d == 0 {
					d = 1
				}
				if (k / d) > 1.0 {
					positiveImpact = true
				}
				if (myRoundsWon - enemyRoundsWon) >= 5 {
					flawlessVictory = true
				}
				if a >= 10 && (roundsPlayed-d) >= 10 {
					combatMedic = true
				}
				break
			}
		}

		passed := false
		progressMsg := ""
		switch activeQuest.Title {
		case "Warmup Routine":
			passed = totalMatches >= 2
			progressMsg = fmt.Sprintf("%d/2 Matches Played Today", totalMatches)
		case "Supporting Cast":
			passed = totalAssists >= 15
			progressMsg = fmt.Sprintf("%.0f/15 Assists Today", totalAssists)
		case "Chip Damage":
			passed = totalDamage >= 2500
			progressMsg = fmt.Sprintf("%.0f/2500 Damage Today", totalDamage)
		case "Consistent":
			acs := 0.0
			if totalRoundsPlayed > 0 {
				acs = totalScore / totalRoundsPlayed
			}
			passed = acs >= 150
			progressMsg = fmt.Sprintf("Current ACS: %.0f (Need 150)", acs)
		case "Clicking Heads":
			passed = totalHeadshots >= 15
			progressMsg = fmt.Sprintf("%.0f/15 Headshots Today", totalHeadshots)
		case "Lethal Force":
			passed = lethalForce
			progressMsg = "Requires 15+ kills in one match today."
		case "Securing the Bag":
			passed = totalRoundsWon >= 15
			progressMsg = fmt.Sprintf("%.0f/15 Rounds Won Today", totalRoundsWon)
		case "Positive Impact":
			passed = positiveImpact
			progressMsg = "Requires a >1.0 KD in a match today."
		case "Hard Carry":
			passed = hardCarry
			progressMsg = "Requires 25+ kills in one match today."
		case "Flawless Victory":
			passed = flawlessVictory
			progressMsg = "Requires winning by 5+ rounds today."
		case "Combat Medic":
			passed = combatMedic
			progressMsg = "Requires 10 assists & 10 rounds survived today."
		case "Top Fragger":
			passed = totalKills >= 50
			progressMsg = fmt.Sprintf("%.0f/50 Kills Today", totalKills)
		}

		if !passed {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error": "Mission not complete. %s"}`, progressMsg)
			return
		}

		tx, _ := db.DB.Begin()
		tx.Exec(fmt.Sprintf("UPDATE user_quests SET %s = 1 WHERE discord_id = ? AND quest_date = ?", claimColumn), userID, effectiveDate)
		tx.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", activeQuest.Reward, userID)
		tx.Commit()

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "message": "Mission Accomplished! +%d FT", "reward": %d}`,
			activeQuest.Reward, activeQuest.Reward)
	})
}
