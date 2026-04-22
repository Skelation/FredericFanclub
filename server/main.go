package main

import (
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"fmt"
)

func main() {
	loadDotEnv()

	base := strings.TrimSuffix(strings.TrimSpace(os.Getenv("VALORANT_API_BASE")), "/")
	fmt.Println(os.Getenv("VALORANT_API_KEY"))
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
	allowed := parseOrigins(os.Getenv("CORS_ORIGINS"))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("OPTIONS /api/matches", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("OPTIONS /api/matches/roster", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("OPTIONS /api/matches/roster/more", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /api/matches/roster/more", func(w http.ResponseWriter, r *http.Request) {
		handleRosterMatchesMore(w, r, allowed, base, matchPath, apiKey)
	})
	mux.HandleFunc("GET /api/matches/roster", func(w http.ResponseWriter, r *http.Request) {
		handleRosterMatches(w, r, allowed, base, matchPath, apiKey)
	})
	mux.HandleFunc("GET /api/matches", func(w http.ResponseWriter, r *http.Request) {
		applyCORS(w, r, allowed)
		region := strings.TrimSpace(r.URL.Query().Get("region"))
		if region == "" {
			region = strings.TrimSpace(os.Getenv("VALORANT_REGION"))
		}
		if region == "" {
			region = "eu"
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		tag := strings.TrimSpace(r.URL.Query().Get("tag"))
		if region == "" || name == "" || tag == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"missing query: region, name, tag"}`))
			return
		}
		platform := strings.TrimSpace(os.Getenv("VALORANT_PLATFORM"))
		if platform == "" {
			platform = "pc"
		}
		u := base + "/" + matchPath + "/" + url.PathEscape(region)
		if strings.Contains(matchPath, "v4/matches") {
			u += "/" + url.PathEscape(platform)
		}
		u += "/" + url.PathEscape(name) + "/" + url.PathEscape(tag)
		if qs := r.URL.RawQuery; qs != "" {
			u += "?" + qs
		}
		upstream := u
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstream, nil)
		if err != nil {
			http.Error(w, `{"error":"bad upstream url"}`, http.StatusInternalServerError)
			return
		}
		if apiKey != "" {
			req.Header.Set("Authorization", apiKey)
		}
		client := &http.Client{Timeout: 25 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("upstream: %v", err)
			http.Error(w, `{"error":"upstream unreachable"}`, http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vv := range resp.Header {
			if strings.EqualFold(k, "Content-Type") {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Printf("copy body: %v", err)
		}
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           logRequests(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s (VALORANT_API_BASE=%s)", srv.Addr, base)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

func parseOrigins(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func applyCORS(w http.ResponseWriter, r *http.Request, allowed []string) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return
	}
	for _, o := range allowed {
		if origin == o {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			return
		}
	}
}
