package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dchest/captcha"

	"fredericfanclub/server/internal/admin"
	"fredericfanclub/server/internal/auth"
	"fredericfanclub/server/internal/betting"
	"fredericfanclub/server/internal/db"
	"fredericfanclub/server/internal/economy"
	"fredericfanclub/server/internal/hub"
	"fredericfanclub/server/internal/inventory"
	"fredericfanclub/server/internal/matches"
	"fredericfanclub/server/internal/middleware"
	"fredericfanclub/server/internal/premier"
	"fredericfanclub/server/internal/quests"
	"fredericfanclub/server/internal/radio"
	"fredericfanclub/server/internal/shop"
	"fredericfanclub/server/internal/user"
)

func main() {
	matches.LoadDotEnv()
	hub.RefreshDevMode() // re-read FRED_ENV now that .env is loaded
	db.Init()
	auth.Init()
	hub.StartBroadcaster()
	captcha.SetCustomStore(captcha.NewMemoryStore(captcha.CollectNum, 10*time.Minute))

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
		port = "3000"
	}
	frontendDir := strings.TrimSpace(os.Getenv("FRONTEND_DIR"))
	if frontendDir == "" {
		frontendDir = "../frontend"
	}
	allowed := middleware.ParseOrigins(os.Getenv("CORS_ORIGINS"))

	matches.StartMatchPoller(base, matchPath, apiKey)
	premier.StartPremierPoller(base, matchPath, apiKey)
	radio.StartRadioDropScheduler()

	mux := http.NewServeMux()
	premier.RegisterPremierRoutes(mux, allowed, middleware.ApplyCORS)
	premier.RegisterAdminRoutes(mux, allowed)

	auth.RegisterRoutes(mux)
	user.RegisterRoutes(mux, allowed)
	economy.RegisterRoutes(mux, allowed)
	quests.RegisterRoutes(mux, allowed)
	inventory.RegisterRoutes(mux, allowed)
	radio.RegisterRoutes(mux, allowed)
	betting.RegisterRoutes(mux, allowed)
	shop.RegisterRoutes(mux, allowed)
	admin.RegisterRoutes(mux, allowed, base, apiKey)
	matches.RegisterRoutes(mux, allowed, base, matchPath, apiKey)

	fs := http.FileServer(http.Dir(frontendDir))
	// login page and its assets are public so unauthenticated users can see the login UI
	mux.Handle("/login.html", fs)
	mux.Handle("/css/", fs)
	mux.Handle("/images/", fs)
	// admin page is restricted to heribio only
	mux.HandleFunc("GET /admin.html", func(w http.ResponseWriter, r *http.Request) {
		userID := middleware.GetUserIDFromCookie(r)
		if userID == "" {
			http.Redirect(w, r, "/login.html", http.StatusSeeOther)
			return
		}
		var username string
		db.DB.QueryRow("SELECT username FROM users WHERE discord_id = ?", userID).Scan(&username)
		if strings.ToLower(username) != "heribio" {
			http.Redirect(w, r, "/login.html?error=forbidden", http.StatusSeeOther)
			return
		}
		fs.ServeHTTP(w, r)
	})
	// everything else requires a valid session
	mux.Handle("/", middleware.RequireAuth(fs))

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           middleware.LogRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s (VALORANT_API_BASE=%s, dev_mode=%v)", srv.Addr, base, hub.DevMode)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
