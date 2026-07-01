package shop

import (
	"encoding/json"
	"hash/fnv"
	"math/rand"
	"net/http"
	"time"

	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/hub"
	"fredericfanclub/server/internal/middleware"
)

type Item struct {
	ID     int    `json:"id"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Price  int    `json:"price"`
	Rarity string `json:"rarity"`
	Data   string `json:"data"`  // banner: CSS background or image path; title ("tag"): display text
	Color  string `json:"color"` // title ("tag"): colour (from rarity); else ""
	Desc   string `json:"description"`
	Owned  bool   `json:"owned"`
}

// dailyCount is how many cosmetics each user is offered per day.
const dailyCount = 1

// dailySeed returns a stable per-user, per-day seed so the rotation is random
// across users but constant for a given user throughout a calendar day (UTC).
func dailySeed(userID string) int64 {
	h := fnv.New64a()
	h.Write([]byte(userID + ":" + time.Now().UTC().Format("2006-01-02")))
	return int64(h.Sum64())
}

// dailyItemIDs deterministically selects up to dailyCount cosmetic IDs for a
// user for the current day. Drawn from banners and titles ("tags").
func dailyItemIDs(userID string) []int {
	ids := make([]int, 0)
	rows, err := db.DB.Query("SELECT id FROM cosmetics WHERE type IN ('banner', 'title') ORDER BY id ASC")
	if err != nil {
		return ids
	}
	for rows.Next() {
		var id int
		rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()

	if len(ids) <= dailyCount {
		return ids
	}
	r := rand.New(rand.NewSource(dailySeed(userID)))
	perm := r.Perm(len(ids))
	out := make([]int, 0, dailyCount)
	for i := 0; i < dailyCount; i++ {
		out = append(out, ids[perm[i]])
	}
	return out
}

func RegisterRoutes(mux *http.ServeMux, allowed []string) {
	options := func(path string) {
		mux.HandleFunc("OPTIONS "+path, func(w http.ResponseWriter, r *http.Request) {
			middleware.ApplyCORS(w, r, allowed)
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
		})
	}
	options("/api/shop")
	options("/api/shop/buy")

	// --- LIST SHOP (today's rotation) ---

	mux.HandleFunc("GET /api/shop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var balance float64
		db.DB.QueryRow("SELECT fredtokens FROM users WHERE discord_id = ?", userID).Scan(&balance)

		owned := map[int]bool{}
		oRows, _ := db.DB.Query("SELECT cosmetic_id FROM user_cosmetics WHERE discord_id = ?", userID)
		if oRows != nil {
			for oRows.Next() {
				var id int
				oRows.Scan(&id)
				owned[id] = true
			}
			oRows.Close()
		}

		// Build the items for today's selection, preserving rotation order.
		// `all` keeps every cosmetic in a stable display order (used in dev
		// mode); `byID` lets us assemble the daily rotation by ID.
		pick := dailyItemIDs(userID)
		byID := map[int]Item{}
		all := make([]Item, 0)
		rows, err := db.DB.Query(`SELECT id, type, name, value, COALESCE(rarity,'bronze'), COALESCE(description,'')
			FROM cosmetics WHERE type IN ('banner', 'title') ORDER BY type, id`)
		if err != nil {
			http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
			return
		}
		for rows.Next() {
			var it Item
			rows.Scan(&it.ID, &it.Type, &it.Name, &it.Data, &it.Rarity, &it.Desc)
			it.Price = db.CosmeticPrice(it.Rarity)
			it.Owned = owned[it.ID]
			if it.Type == "title" {
				it.Color = db.RarityColor(it.Rarity)
			}
			byID[it.ID] = it
			all = append(all, it)
		}
		rows.Close()

		// Dev mode: expose the whole catalog so anything can be bought for
		// testing, instead of the daily 1-item rotation.
		var items []Item
		if hub.DevMode {
			items = all
		} else {
			items = make([]Item, 0, len(pick))
			for _, id := range pick {
				if it, ok := byID[id]; ok {
					items = append(items, it)
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"balance":  balance,
			"items":    items,
			"dev_mode": hub.DevMode,
		})
	})

	// --- BUY ---

	mux.HandleFunc("POST /api/shop/buy", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct {
			ItemID int `json:"item_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ItemID == 0 {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		// The item must be part of today's rotation for this user — unless
		// we're in dev mode, where the whole catalog is purchasable.
		if !hub.DevMode {
			inRotation := false
			for _, id := range dailyItemIDs(userID) {
				if id == req.ItemID {
					inRotation = true
					break
				}
			}
			if !inRotation {
				http.Error(w, `{"error": "Cet objet n'est pas en vente aujourd'hui"}`, http.StatusBadRequest)
				return
			}
		}

		var rarity string
		if err := db.DB.QueryRow("SELECT COALESCE(rarity,'bronze') FROM cosmetics WHERE id = ?", req.ItemID).
			Scan(&rarity); err != nil {
			http.Error(w, `{"error": "Item introuvable"}`, http.StatusBadRequest)
			return
		}
		price := db.CosmeticPrice(rarity)

		var already int
		db.DB.QueryRow("SELECT COUNT(*) FROM user_cosmetics WHERE discord_id = ? AND cosmetic_id = ?", userID, req.ItemID).Scan(&already)
		if already > 0 {
			http.Error(w, `{"error": "Tu possèdes déjà cet objet"}`, http.StatusBadRequest)
			return
		}

		var balance float64
		db.DB.QueryRow("SELECT fredtokens FROM users WHERE discord_id = ?", userID).Scan(&balance)
		if balance < float64(price) {
			http.Error(w, `{"error": "Solde insuffisant"}`, http.StatusBadRequest)
			return
		}

		tx, err := db.DB.Begin()
		if err != nil {
			http.Error(w, `{"error": "server error"}`, http.StatusInternalServerError)
			return
		}
		tx.Exec("UPDATE users SET fredtokens = fredtokens - ? WHERE discord_id = ?", price, userID)
		tx.Exec("INSERT INTO user_cosmetics (discord_id, cosmetic_id) VALUES (?, ?)", userID, req.ItemID)
		tx.Commit()

		var newBalance float64
		db.DB.QueryRow("SELECT fredtokens FROM users WHERE discord_id = ?", userID).Scan(&newBalance)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "new_balance": newBalance})
	})
}
