package radio

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dchest/captcha"

	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/middleware"
)

var captchaDropMap sync.Map // captcha_id → drop_id (int)

func radioDropReward() int {
	roll := rand.Float64()
	switch {
	case roll < 0.35:
		return 200
	case roll < 0.65:
		return 250
	case roll < 0.85:
		return 450
	case roll < 0.95:
		return 600
	default:
		return 500
	}
}

func StartRadioDropScheduler() {
	go func() {
		for {
			enabled := db.GetServerConfigInt("radio_drop_enabled", 1)
			if enabled == 0 {
				log.Println("[Radio Scheduler] Disabled via config — sleeping 5 min")
				time.Sleep(5 * time.Minute)
				continue
			}

			minInterval := db.GetServerConfigInt("radio_drop_min_interval", 30)
			maxInterval := db.GetServerConfigInt("radio_drop_max_interval", 60)
			windowSec := db.GetServerConfigInt("radio_drop_window_sec", 300)
			now := time.Now().UTC()

			var lastRevealAt string
			var lastWindowSec int
			err := db.DB.QueryRow(`SELECT reveal_at, window_seconds FROM radio_drops ORDER BY reveal_at DESC LIMIT 1`).
				Scan(&lastRevealAt, &lastWindowSec)

			var scheduleAfter time.Time
			if err != nil || lastRevealAt == "" {
				delay := time.Duration(5+rand.Intn(10)) * time.Minute
				scheduleAfter = now.Add(delay)
				log.Printf("[Radio Scheduler] No prior drops — first drop in %.0f min", delay.Minutes())
			} else {
				lastReveal, parseErr := db.ParseDropTime(lastRevealAt)
				if parseErr != nil {
					log.Printf("[Radio Scheduler] Cannot parse last reveal_at %q: %v — retrying in 1 min", lastRevealAt, parseErr)
					time.Sleep(time.Minute)
					continue
				}
				windowEnd := lastReveal.Add(time.Duration(lastWindowSec) * time.Second)
				if windowEnd.After(now) {
					wait := windowEnd.Sub(now) + 10*time.Second
					log.Printf("[Radio Scheduler] Last drop still live, waiting %.0fs", wait.Seconds())
					time.Sleep(wait)
					continue
				}
				gapMin := minInterval + rand.Intn(maxInterval-minInterval+1)
				scheduleAfter = windowEnd.Add(time.Duration(gapMin) * time.Minute)
			}

			if scheduleAfter.Before(now) {
				scheduleAfter = now.Add(time.Minute)
			}

			ft := radioDropReward()
			revealStr := scheduleAfter.Format(time.RFC3339)
			res, dbErr := db.DB.Exec(
				"INSERT INTO radio_drops (reward_ft, max_uses, reveal_at, window_seconds) VALUES (?, 1, ?, ?)",
				ft, revealStr, windowSec)
			if dbErr != nil {
				log.Printf("[Radio Scheduler] DB insert failed: %v — retrying in 1 min", dbErr)
				time.Sleep(time.Minute)
				continue
			}
			id, _ := res.LastInsertId()
			log.Printf("[Radio Scheduler] Drop #%d scheduled — %d FT at %s (window %ds)",
				id, ft, scheduleAfter.Format("2006-01-02 15:04 UTC"), windowSec)

			windowEnd := scheduleAfter.Add(time.Duration(windowSec) * time.Second)
			time.Sleep(windowEnd.Sub(now) + 10*time.Second)
		}
	}()
}

func RegisterRoutes(mux *http.ServeMux, allowed []string) {
	// Admin: schedule a manual radio drop
	mux.HandleFunc("OPTIONS /api/admin/radio-drop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Admin-Token")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/admin/radio-drop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		if r.Header.Get("X-Admin-Token") != strings.TrimSpace(os.Getenv("FRED_ADMIN_TOKEN")) {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			RewardFT      int    `json:"reward_ft"`
			MaxUses       int    `json:"max_uses"`
			RevealAt      string `json:"reveal_at"`
			WindowSeconds int    `json:"window_seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RewardFT <= 0 {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}
		if req.MaxUses <= 0 {
			req.MaxUses = 1
		}
		if req.WindowSeconds <= 0 {
			req.WindowSeconds = 420
		}
		if req.RevealAt == "" {
			req.RevealAt = time.Now().UTC().Format(time.RFC3339)
		}
		res, err := db.DB.Exec(
			"INSERT INTO radio_drops (reward_ft, max_uses, reveal_at, window_seconds) VALUES (?, ?, ?, ?)",
			req.RewardFT, req.MaxUses, req.RevealAt, req.WindowSeconds)
		if err != nil {
			http.Error(w, `{"error": "db error"}`, http.StatusInternalServerError)
			return
		}
		id, _ := res.LastInsertId()
		log.Printf("[Radio Drop] Created drop #%d — %d FT, max %d uses, reveals at %s, window %ds",
			id, req.RewardFT, req.MaxUses, req.RevealAt, req.WindowSeconds)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "drop_id": %d, "reward_ft": %d, "reveal_at": %q, "window_seconds": %d}`,
			id, req.RewardFT, req.RevealAt, req.WindowSeconds)
	})

	// Public: check if a drop is currently active
	mux.HandleFunc("GET /api/radio/active-drop", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		now := time.Now().UTC()

		inactive := func(reason string) {
			log.Printf("[Radio Drop] Poll — inactive: %s", reason)
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"active": false}`))
		}

		parseReveal := func(s string) (time.Time, error) {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t, nil
			}
			if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
				return t, nil
			}
			return time.Parse("2006-01-02 15:04:05", s)
		}

		rows, err := db.DB.Query(`SELECT id, reward_ft, reveal_at, window_seconds, uses_so_far, max_uses
			FROM radio_drops WHERE uses_so_far < max_uses ORDER BY reveal_at ASC`)
		if err != nil {
			inactive(fmt.Sprintf("DB query error: %v", err))
			return
		}
		defer rows.Close()

		type dropRow struct {
			ID, RewardFT, WindowSecs, UsesSoFar, MaxUses int
			RevealAt                                      string
		}
		var candidates []dropRow
		for rows.Next() {
			var d dropRow
			rows.Scan(&d.ID, &d.RewardFT, &d.RevealAt, &d.WindowSecs, &d.UsesSoFar, &d.MaxUses)
			candidates = append(candidates, d)
		}
		rows.Close()

		if len(candidates) == 0 {
			inactive("no drops in database")
			return
		}

		log.Printf("[Radio Drop] Poll — found %d unclaimed drop(s), checking windows (now=%s)",
			len(candidates), now.Format("15:04:05"))

		for _, d := range candidates {
			revealTime, parseErr := parseReveal(d.RevealAt)
			if parseErr != nil {
				log.Printf("[Radio Drop]   drop #%d: SKIP — cannot parse reveal_at %q: %v", d.ID, d.RevealAt, parseErr)
				continue
			}
			windowEnd := revealTime.Add(time.Duration(d.WindowSecs) * time.Second)
			if now.Before(revealTime) {
				log.Printf("[Radio Drop]   drop #%d: not yet (reveals in %.0fs)", d.ID, revealTime.Sub(now).Seconds())
				continue
			}
			if now.After(windowEnd) {
				log.Printf("[Radio Drop]   drop #%d: expired (%.0fs ago)", d.ID, now.Sub(windowEnd).Seconds())
				continue
			}
			expiresIn := int(windowEnd.Sub(now).Seconds())
			alreadyClaimed := false
			if userID != "" {
				var n int
				db.DB.QueryRow("SELECT COUNT(*) FROM radio_drop_claims WHERE drop_id = ? AND discord_id = ?", d.ID, userID).Scan(&n)
				alreadyClaimed = n > 0
			}
			log.Printf("[Radio Drop]   drop #%d: ACTIVE — %d FT, %ds left, %d/%d claimed",
				d.ID, d.RewardFT, expiresIn, d.UsesSoFar, d.MaxUses)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"active": true, "drop_id": %d, "reward_ft": %d, "expires_in": %d, "already_claimed": %v}`,
				d.ID, d.RewardFT, expiresIn, alreadyClaimed)
			return
		}

		inactive("all drops are outside their windows")
	})

	// Generate a CAPTCHA tied to a specific drop
	mux.HandleFunc("GET /api/radio/captcha/new", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}
		dropID := r.URL.Query().Get("drop_id")
		if dropID == "" {
			http.Error(w, `{"error": "drop_id required"}`, http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()
		var dID, rewardFT, windowSecs, maxUses, usesSoFar int
		var revealAt string
		err := db.DB.QueryRow(
			"SELECT id, reward_ft, reveal_at, window_seconds, max_uses, uses_so_far FROM radio_drops WHERE id = ?", dropID).
			Scan(&dID, &rewardFT, &revealAt, &windowSecs, &maxUses, &usesSoFar)
		if err != nil {
			http.Error(w, `{"error": "drop not found"}`, http.StatusNotFound)
			return
		}
		revealTime, _ := time.Parse(time.RFC3339, revealAt)
		if revealTime.IsZero() {
			revealTime, _ = time.Parse("2006-01-02 15:04:05", revealAt)
		}
		if now.Before(revealTime) || now.After(revealTime.Add(time.Duration(windowSecs)*time.Second)) {
			http.Error(w, `{"error": "drop is not active"}`, http.StatusBadRequest)
			return
		}
		if usesSoFar >= maxUses {
			http.Error(w, `{"error": "drop is fully claimed"}`, http.StatusBadRequest)
			return
		}
		var already int
		db.DB.QueryRow("SELECT COUNT(*) FROM radio_drop_claims WHERE drop_id = ? AND discord_id = ?", dID, userID).Scan(&already)
		if already > 0 {
			http.Error(w, `{"error": "already claimed"}`, http.StatusBadRequest)
			return
		}

		captchaID := captcha.New()
		captchaDropMap.Store(captchaID, dID)
		log.Printf("[Radio Drop] Captcha generated for drop #%d (user %s), captcha_id=%s", dID, userID, captchaID)

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"captcha_id": %q, "image_url": "/api/radio/captcha/image/%s", "audio_url": "/api/radio/captcha/audio/%s", "reward_ft": %d}`,
			captchaID, captchaID, captchaID, rewardFT)
	})

	// Serve captcha image
	mux.HandleFunc("GET /api/radio/captcha/image/{id}", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		id := r.PathValue("id")
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		if err := captcha.WriteImage(w, id, 240, 80); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	// Serve captcha audio
	mux.HandleFunc("GET /api/radio/captcha/audio/{id}", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		id := r.PathValue("id")
		w.Header().Set("Content-Type", "audio/wav")
		w.Header().Set("Cache-Control", "no-store")
		if err := captcha.WriteAudio(w, id, "en"); err != nil {
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	// Claim a drop by solving its CAPTCHA
	mux.HandleFunc("OPTIONS /api/radio/claim", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /api/radio/claim", func(w http.ResponseWriter, r *http.Request) {
		middleware.ApplyCORS(w, r, allowed)
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req struct {
			CaptchaID string `json:"captcha_id"`
			Solution  string `json:"solution"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error": "invalid request"}`, http.StatusBadRequest)
			return
		}

		if !captcha.VerifyString(req.CaptchaID, req.Solution) {
			log.Printf("[Radio Drop] Claim rejected — wrong captcha (id=%s, answer=%q)", req.CaptchaID, req.Solution)
			http.Error(w, `{"error": "Code incorrect — réessaie"}`, http.StatusBadRequest)
			return
		}

		rawDropID, ok := captchaDropMap.LoadAndDelete(req.CaptchaID)
		if !ok {
			http.Error(w, `{"error": "Session expirée — génère un nouveau défi"}`, http.StatusBadRequest)
			return
		}
		dropID := rawDropID.(int)

		now := time.Now().UTC()
		var rewardFT, windowSecs, maxUses, usesSoFar int
		var revealAt string
		err := db.DB.QueryRow(
			"SELECT reward_ft, reveal_at, window_seconds, max_uses, uses_so_far FROM radio_drops WHERE id = ?", dropID).
			Scan(&rewardFT, &revealAt, &windowSecs, &maxUses, &usesSoFar)
		if err != nil {
			http.Error(w, `{"error": "drop not found"}`, http.StatusInternalServerError)
			return
		}
		revealTime, _ := time.Parse(time.RFC3339, revealAt)
		if revealTime.IsZero() {
			revealTime, _ = time.Parse("2006-01-02 15:04:05", revealAt)
		}
		if now.After(revealTime.Add(time.Duration(windowSecs) * time.Second)) {
			http.Error(w, `{"error": "Le drop a expiré"}`, http.StatusBadRequest)
			return
		}
		if usesSoFar >= maxUses {
			http.Error(w, `{"error": "Drop entièrement réclamé"}`, http.StatusBadRequest)
			return
		}

		var already int
		db.DB.QueryRow("SELECT COUNT(*) FROM radio_drop_claims WHERE drop_id = ? AND discord_id = ?", dropID, userID).Scan(&already)
		if already > 0 {
			http.Error(w, `{"error": "Tu as déjà réclamé ce drop"}`, http.StatusBadRequest)
			return
		}

		tx, err := db.DB.Begin()
		if err != nil {
			http.Error(w, `{"error": "server error"}`, http.StatusInternalServerError)
			return
		}
		res, err := tx.Exec("UPDATE radio_drops SET uses_so_far = uses_so_far + 1 WHERE id = ? AND uses_so_far < max_uses", dropID)
		if err != nil {
			tx.Rollback()
			http.Error(w, `{"error": "server error"}`, http.StatusInternalServerError)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			tx.Rollback()
			http.Error(w, `{"error": "Drop entièrement réclamé"}`, http.StatusConflict)
			return
		}
		tx.Exec("UPDATE users SET fredtokens = fredtokens + ? WHERE discord_id = ?", rewardFT, userID)
		tx.Exec("INSERT INTO radio_drop_claims (drop_id, discord_id) VALUES (?, ?)", dropID, userID)
		tx.Commit()

		var newBalance float64
		db.DB.QueryRow("SELECT fredtokens FROM users WHERE discord_id = ?", userID).Scan(&newBalance)
		log.Printf("[Radio Drop] Claimed — drop #%d, user %s, +%d FT, new balance %.0f", dropID, userID, rewardFT, newBalance)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"success": true, "reward_ft": %d, "new_balance": %g}`, rewardFT, newBalance)
	})
}
