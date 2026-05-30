package auth

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/oauth2"

	"fredericfanclub/server/internal/db"
)

var (
	oauthConfig *oauth2.Config
	oauthState  = "fred-secure-state-token"
)

func Init() {
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

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/discord", func(w http.ResponseWriter, r *http.Request) {
		url := oauthConfig.AuthCodeURL(oauthState)
		http.Redirect(w, r, url, http.StatusTemporaryRedirect)
	})

	mux.HandleFunc("GET /api/auth/discord/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.FormValue("state") != oauthState {
			http.Error(w, "State invalid", http.StatusBadRequest)
			return
		}

		token, err := oauthConfig.Exchange(context.Background(), r.FormValue("code"))
		if err != nil {
			http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
			return
		}

		res, err := oauthConfig.Client(context.Background(), token).Get("https://discord.com/api/users/@me")
		if err != nil || res.StatusCode != 200 {
			http.Error(w, "Failed to fetch user info", http.StatusInternalServerError)
			return
		}
		defer res.Body.Close()

		var discordUser struct {
			ID       string `json:"id"`
			Username string `json:"username"`
			Avatar   string `json:"avatar"`
		}
		if err := json.NewDecoder(res.Body).Decode(&discordUser); err != nil {
			http.Error(w, "Failed to decode user info", http.StatusInternalServerError)
			return
		}

		exists, err := db.UserExists(discordUser.ID)
		if err != nil {
			log.Println("DB Error checking user:", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		if !exists {
			http.Redirect(w, r, "/login.html?error=not_registered", http.StatusSeeOther)
			return
		}

		_, err = db.DB.Exec(
			`UPDATE users SET username = ?, avatar_url = ? WHERE discord_id = ?`,
			discordUser.Username, discordUser.Avatar, discordUser.ID)
		if err != nil {
			log.Println("DB Error:", err)
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		frontendURL := strings.TrimSpace(os.Getenv("FRONTEND_URL"))
		if frontendURL == "" {
			port := strings.TrimSpace(os.Getenv("PORT"))
			if port == "" {
				port = "3000"
			}
			frontendURL = "http://localhost:" + port
		}

		isDev := strings.Contains(frontendURL, "localhost") || strings.Contains(frontendURL, "127.0.0.1")

		cookieDomain := ".fredericfan.club"
		secureCookie := true
		sameSitePolicy := http.SameSiteNoneMode

		if isDev {
			cookieDomain = ""
			secureCookie = false
			sameSitePolicy = http.SameSiteLaxMode
		}

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

		http.Redirect(w, r, frontendURL+"/", http.StatusTemporaryRedirect)
	})
}
