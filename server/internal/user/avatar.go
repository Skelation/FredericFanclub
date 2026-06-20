package user

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"

	"fredericfanclub/server/internal/db"
)

// Discord avatar hashes are stored in our DB and only refreshed on OAuth
// login (see auth.go). A user who changes their avatar on Discord but keeps
// an existing session never re-triggers that update, so the stored hash goes
// stale and 404s on Discord's CDN. refreshAvatar re-fetches the current hash
// via the bot token so displayed avatars stay correct.
var (
	avatarMu      sync.Mutex
	avatarChecked = map[string]time.Time{}
)

const avatarRefreshTTL = 30 * time.Minute

// refreshAvatar returns the most up-to-date avatar hash for a Discord user,
// fetching it from Discord (and updating the DB) when our cached value may be
// stale. It is throttled per user by avatarRefreshTTL and is a no-op when no
// bot token is configured, falling back to currentHash on any failure.
func refreshAvatar(discordID, currentHash string) string {
	token := os.Getenv("DISCORD_BOT_TOKEN")
	if token == "" || discordID == "" {
		return currentHash
	}

	avatarMu.Lock()
	if t, ok := avatarChecked[discordID]; ok && time.Since(t) < avatarRefreshTTL {
		avatarMu.Unlock()
		return currentHash
	}
	avatarChecked[discordID] = time.Now()
	avatarMu.Unlock()

	req, err := http.NewRequest("GET", "https://discord.com/api/v10/users/"+discordID, nil)
	if err != nil {
		return currentHash
	}
	req.Header.Set("Authorization", "Bot "+token)

	client := &http.Client{Timeout: 5 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return currentHash
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return currentHash
	}

	var u struct {
		Avatar string `json:"avatar"` // null (no custom avatar) decodes to ""
	}
	if err := json.NewDecoder(res.Body).Decode(&u); err != nil {
		return currentHash
	}

	if u.Avatar != currentHash {
		db.DB.Exec("UPDATE users SET avatar_url = ? WHERE discord_id = ?", u.Avatar, discordID)
	}
	return u.Avatar
}
