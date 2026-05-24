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
	"fredericfanclub/server/internal/user"
)

func main() {
	matches.LoadDotEnv()
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
		port = "8080"
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
	admin.RegisterRoutes(mux, allowed, base, apiKey)
	matches.RegisterRoutes(mux, allowed, base, matchPath, apiKey)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           middleware.LogRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s (VALORANT_API_BASE=%s)", srv.Addr, base)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
