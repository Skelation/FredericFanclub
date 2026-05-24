package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func Init() {
	var err error

	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	fmt.Println(dbPath)
	if dbPath == "" {
		dbPath = "./database/fred.db"
	}

	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal("Failed to open database:", err)
	}

	_, err = DB.Exec(`CREATE TABLE IF NOT EXISTS users (
		discord_id TEXT PRIMARY KEY,
		username TEXT,
		avatar_url TEXT,
		fredtokens INTEGER DEFAULT 1000,
		linked_player TEXT DEFAULT 'none',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		log.Fatal("Failed to create users table:", err)
	}

	_, err = DB.Exec(`CREATE TABLE IF NOT EXISTS bets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		discord_id TEXT,
		bet_category TEXT,
		match_target TEXT,
		target_player TEXT,
		prop_type TEXT,
		line_value REAL,
		choice TEXT,
		amount REAL,
		locked_multiplier REAL,
		status TEXT DEFAULT 'pending',
		FOREIGN KEY(discord_id) REFERENCES users(discord_id)
	)`)
	if err != nil {
		log.Fatal("Failed to create bets table:", err)
	}

	_, err = DB.Exec(`CREATE TABLE IF NOT EXISTS cards (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		rarity TEXT,
		image_url TEXT
	)`)
	if err != nil {
		log.Fatal("Failed to create cards table:", err)
	}

	_, err = DB.Exec(`CREATE TABLE IF NOT EXISTS inventory (
		discord_id TEXT,
		card_id INTEGER,
		quantity INTEGER DEFAULT 0,
		PRIMARY KEY (discord_id, card_id),
		FOREIGN KEY(discord_id) REFERENCES users(discord_id),
		FOREIGN KEY(card_id) REFERENCES cards(id)
	)`)
	if err != nil {
		log.Fatal("Failed to create inventory table:", err)
	}

	_, err = DB.Exec(`CREATE TABLE IF NOT EXISTS user_quests (
		discord_id TEXT,
		quest_date TEXT,
		easy_claimed BOOLEAN DEFAULT 0,
		med_claimed BOOLEAN DEFAULT 0,
		hard_claimed BOOLEAN DEFAULT 0,
		PRIMARY KEY (discord_id, quest_date)
	)`)
	if err != nil {
		log.Fatal("Failed to create user_quests table:", err)
	}

	// Migrations
	DB.Exec("ALTER TABLE cards ADD COLUMN season TEXT DEFAULT 'Season 1'")
	DB.Exec("ALTER TABLE users ADD COLUMN last_daily_claim TEXT DEFAULT ''")

	DB.Exec(`CREATE TABLE IF NOT EXISTS server_config (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`)
	DB.Exec(`INSERT OR IGNORE INTO server_config (key, value) VALUES ('current_pack_season', 'Season 1')`)
	DB.Exec(`INSERT OR IGNORE INTO server_config (key, value) VALUES ('radio_drop_enabled',      'true')`)
	DB.Exec(`INSERT OR IGNORE INTO server_config (key, value) VALUES ('radio_drop_min_interval', '15')`)
	DB.Exec(`INSERT OR IGNORE INTO server_config (key, value) VALUES ('radio_drop_max_interval', '40')`)
	DB.Exec(`INSERT OR IGNORE INTO server_config (key, value) VALUES ('radio_drop_window_sec',   '420')`)
	DB.Exec(`UPDATE server_config SET value = '15' WHERE key = 'radio_drop_min_interval' AND CAST(value AS INTEGER) > 15`)
	DB.Exec(`UPDATE server_config SET value = '40' WHERE key = 'radio_drop_max_interval' AND CAST(value AS INTEGER) > 40`)
	DB.Exec(`UPDATE server_config SET value = '420' WHERE key = 'radio_drop_window_sec' AND CAST(value AS INTEGER) < 420`)

	DB.Exec(`CREATE TABLE IF NOT EXISTS redeem_codes (
		code        TEXT PRIMARY KEY,
		reward_ft   INTEGER NOT NULL,
		max_uses    INTEGER NOT NULL DEFAULT 1,
		uses_so_far INTEGER NOT NULL DEFAULT 0,
		expires_at  TEXT NOT NULL
	)`)

	DB.Exec(`CREATE TABLE IF NOT EXISTS code_redemptions (
		user_id     TEXT NOT NULL,
		code        TEXT NOT NULL,
		redeemed_at TEXT DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (user_id, code)
	)`)

	DB.Exec(`CREATE TABLE IF NOT EXISTS user_badges (
		discord_id TEXT NOT NULL,
		badge_type TEXT NOT NULL,
		earned_at  TEXT DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (discord_id, badge_type)
	)`)

	DB.Exec(`CREATE TABLE IF NOT EXISTS user_showcase (
		discord_id TEXT PRIMARY KEY,
		card_id    INTEGER NOT NULL,
		FOREIGN KEY(discord_id) REFERENCES users(discord_id),
		FOREIGN KEY(card_id) REFERENCES cards(id)
	)`)

	DB.Exec(`CREATE TABLE IF NOT EXISTS radio_drops (
		id             INTEGER PRIMARY KEY AUTOINCREMENT,
		reward_ft      INTEGER NOT NULL,
		max_uses       INTEGER NOT NULL DEFAULT 100,
		uses_so_far    INTEGER NOT NULL DEFAULT 0,
		reveal_at      TEXT NOT NULL,
		window_seconds INTEGER NOT NULL DEFAULT 420,
		created_at     TEXT DEFAULT CURRENT_TIMESTAMP
	)`)

	DB.Exec(`CREATE TABLE IF NOT EXISTS radio_drop_claims (
		drop_id    INTEGER NOT NULL,
		discord_id TEXT NOT NULL,
		claimed_at TEXT DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (drop_id, discord_id)
	)`)

	log.Println("Database initialized successfully!")
}

func GetServerConfigInt(key string, def int) int {
	var val string
	if err := DB.QueryRow("SELECT value FROM server_config WHERE key = ?", key).Scan(&val); err != nil {
		return def
	}
	if n, err := strconv.Atoi(val); err == nil {
		return n
	}
	return def
}

func ParseDropTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", s)
}

func AwardBadgeIfNew(discordID, badgeType string) {
	DB.Exec(`INSERT OR IGNORE INTO user_badges (discord_id, badge_type) VALUES (?, ?)`, discordID, badgeType)
}

func CheckAndAwardBetBadges(discordID string) {
	var totalWins int
	DB.QueryRow("SELECT COUNT(*) FROM bets WHERE discord_id = ? AND status = 'won'", discordID).Scan(&totalWins)
	if totalWins >= 25 {
		AwardBadgeIfNew(discordID, "bet_wins_25")
	}
}

func CheckAndAwardSeasonBadge(discordID, season string) {
	var totalCards, ownedCards int
	DB.QueryRow("SELECT COUNT(*) FROM cards WHERE season = ?", season).Scan(&totalCards)
	if totalCards == 0 {
		return
	}
	DB.QueryRow(`SELECT COUNT(DISTINCT c.id) FROM inventory i
		JOIN cards c ON i.card_id = c.id
		WHERE i.discord_id = ? AND c.season = ? AND i.quantity > 0`, discordID, season).Scan(&ownedCards)
	if ownedCards >= totalCards {
		AwardBadgeIfNew(discordID, "season_complete_"+season)
	}
}
