package discordbot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

const AlertRoleID = "1501675597233918122"

// --- 1. THE BET NOTIFICATION (UPGRADED) ---
func SendBetNotification(username string, choice string, amount int) {
	token := os.Getenv("DISCORD_BOT_TOKEN")
	channelID := os.Getenv("DISCORD_CHANNEL_ID")
	if token == "" || channelID == "" { return }

	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)

	color := 0x00ff64 // Green for Over/Win
	if choice == "under" || choice == "loss" { color = 0xff4655 } // Red for Under/Loss

	embed := map[string]interface{}{
		"title":       "🎰 NOUVELLE MISE!",
		"color":       color,
		"description": fmt.Sprintf("**%s** a mis **%d Freddy Tokens** sur **%s**!", username, amount, choice),
		"footer":      map[string]interface{}{"text": "FRED"},
	}

	payload := map[string]interface{}{
		"embeds":  []interface{}{embed},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	client.Do(req) 
}

// --- 2. THE NEW MARKET PUBLISHED NOTIFICATION ---
func SendMarketPublishedNotification(player string, propType string, line float64, overOdds float64, underOdds float64) {
	token := os.Getenv("DISCORD_BOT_TOKEN")
	channelID := os.Getenv("DISCORD_CHANNEL_ID")
	if token == "" || channelID == "" { return }

	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)

	// Format the display text nicely
	displayType := propType
	if propType == "match_result" { displayType = "MATCH WIN/LOSS" }

	embed := map[string]interface{}{
		"title":       "🚨 NOUVEAU BET OUVERT! 🚨",
		"color":       0xffaa00, 
		"description": fmt.Sprintf("Un nouveau market est apparu sur **%s**! allez sur fredericfan.club/betting pour mettre vos paris.", player),
		"fields": []map[string]interface{}{
			{
				"name":   "🎯 Proposition",
				"value":  fmt.Sprintf("**Total %s**", displayType),
				"inline": true,
			},
			{
				"name":   "📈 Line",
				"value":  fmt.Sprintf("**%.1f**", line),
				"inline": true,
			},
			{
				"name":   "💰 Les côtes",
				"value":  fmt.Sprintf("🟢 OVER: **%.2fx**\n🔴 UNDER: **%.2fx**", overOdds, underOdds),
				"inline": false,
			},
		},
		"footer": map[string]interface{}{"text": "Fred"},
	}

	payload := map[string]interface{}{
		"content": fmt.Sprintf("<@&%s> Allez on parie ! fredericfan.club/betting", AlertRoleID), // PING!
		"embeds":  []interface{}{embed},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	client.Do(req)
}

type BetResult struct {
	Username string
	Amount   float64
	Payout   float64
}

// --- 3. THE MARKET RESOLVED (POST-GAME SUMMARY) ---
func SendMarketResolvedNotification(player string, propType string, outcome string, winners []BetResult, losers []BetResult) {
	token := os.Getenv("DISCORD_BOT_TOKEN")
	channelID := os.Getenv("DISCORD_CHANNEL_ID")
	if token == "" || channelID == "" { return }

	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)

	displayType := propType
	if propType == "match_result" { displayType = "MATCH WIN/LOSS" }

	// Format the Winner and Loser text blocks
	winnerText := ""
	for _, w := range winners {
		winnerText += fmt.Sprintf("✅ **%s**: +%.2f FT (Bet: %.0f)\n", w.Username, w.Payout, w.Amount)
	}
	if winnerText == "" { winnerText = "*Personne n'a cru en le script...*" }

	loserText := ""
	for _, l := range losers {
		loserText += fmt.Sprintf("💀 **%s**: -%.0f Freddy Tokens\n", l.Username, l.Amount)
	}
	if loserText == "" { loserText = "*Personne n'a perdu d'argent!*" }

	// Choose color based on the outcome
	color := 0x00ff64 // Green
	if outcome == "under" { color = 0xff4655 } // Red

	embed := map[string]interface{}{
		"title":       "🏁 BET FINI & ARGENT ENVOYE!",
		"color":       color,
		"description": fmt.Sprintf("Les résultats sont en ligne pour **%s** (%s)!\nLe résultat est: **%s**.", player, displayType, outcome),
		"fields": []map[string]interface{}{
			{
				"name":   "🏆 LES GAGNANTS",
				"value":  winnerText,
				"inline": false,
			},
			{
				"name":   "📉 LES PERDANTS",
				"value":  loserText,
				"inline": false,
			},
		},
		"footer": map[string]interface{}{"text": "FRED - LES PORTEFEUILLES ONT ETE MIS A JOUR."},
	}

	payload := map[string]interface{}{
		"content": fmt.Sprintf("<@&%s> VERIFIEZ VOS COMPTES! fredericfan.club/betting", AlertRoleID),
		"embeds":  []interface{}{embed},
	}

	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", url, bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bot "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	client.Do(req)
}
