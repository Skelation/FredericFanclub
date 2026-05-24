package inventory

import (
	"encoding/json"
	"net/http"

	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/middleware"
)

func RegisterRoutes(mux *http.ServeMux, allowed []string) {
	mux.HandleFunc("OPTIONS /api/inventory", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/inventory", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)

		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		rows, err := db.DB.Query(`
			SELECT c.id, c.name, c.rarity, c.image_url, c.season, i.quantity
			FROM inventory i
			JOIN cards c ON i.card_id = c.id
			WHERE i.discord_id = ? AND i.quantity > 0
			ORDER BY
				CASE c.rarity
					WHEN 'radiant'   THEN 1
					WHEN 'immortal'  THEN 2
					WHEN 'ascendant' THEN 3
					WHEN 'diamond'   THEN 4
					WHEN 'bronze'    THEN 5
					WHEN 'iron'      THEN 6
					ELSE 7
				END, c.name ASC`, userID)
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
			Season   string `json:"season"`
			Quantity int    `json:"quantity"`
		}

		var items []InventoryItem
		for rows.Next() {
			var item InventoryItem
			rows.Scan(&item.ID, &item.Name, &item.Rarity, &item.ImageURL, &item.Season, &item.Quantity)
			items = append(items, item)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(items)
	})

	mux.HandleFunc("OPTIONS /api/catalog", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/catalog", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)

		userID := middleware.GetUserIDFromCookie(r)

		rows, err := db.DB.Query(`
			SELECT c.id, c.name, c.rarity, c.image_url, c.season, COALESCE(i.quantity, 0) as quantity
			FROM cards c
			LEFT JOIN inventory i ON c.id = i.card_id AND i.discord_id = ?
			ORDER BY c.season ASC,
				CASE c.rarity
					WHEN 'radiant'   THEN 1
					WHEN 'immortal'  THEN 2
					WHEN 'ascendant' THEN 3
					WHEN 'diamond'   THEN 4
					WHEN 'bronze'    THEN 5
					WHEN 'iron'      THEN 6
					ELSE 7
				END, c.name ASC`, userID)
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
			Season   string `json:"season"`
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
}
