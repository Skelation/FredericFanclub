package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type riotPlayer struct {
	Name string `json:"name"`
	Tag  string `json:"tag"`
}

type rosterMatchOut struct {
	Match      json.RawMessage  `json:"match"`
	Roster     []riotPlayer     `json:"roster"`
	RRByPlayer map[string]int64 `json:"rrByPlayer,omitempty"`
}

type playerResume struct {
	Name       string `json:"name"`
	Tag        string `json:"tag"`
	NextStart  int    `json:"nextStart"`
	Exhausted  bool   `json:"exhausted"`
}

type rosterAPIResponse struct {
	Data     []rosterMatchOut `json:"data"`
	Players  []riotPlayer     `json:"players"`
	Warnings []string         `json:"warnings,omitempty"`
	Resume   []playerResume   `json:"resume,omitempty"`
	HasMore  bool             `json:"hasMore"`
}

type rosterContinueRequest struct {
	Resume          []playerResume `json:"resume"`
	KnownMatchIDs   []string       `json:"knownMatchIds"`
	PagesPerRequest int            `json:"pagesPerRequest"`
}

type aggEntry struct {
	match      json.RawMessage
	playerK    map[string]riotPlayer
	rrByPlayer map[string]int64
}

func loadDotEnv() {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	dir := wd
	for range 8 {
		p := filepath.Join(dir, ".env")
		if _, err := os.Stat(p); err == nil {
			if err := godotenv.Overload(p); err != nil {
				log.Printf("env: %s: %v", p, err)
			} else {
				log.Printf("env: loaded %s", p)
			}
			return
		}
		next := filepath.Dir(dir)
		if next == dir {
			return
		}
		dir = next
	}
}

func parsePlayersList(raw string) []riotPlayer {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultRosterPlayers()
	}
	parts := strings.Split(raw, ",")
	out := make([]riotPlayer, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		i := strings.LastIndex(p, "#")
		if i <= 0 || i >= len(p)-1 {
			continue
		}
		out = append(out, riotPlayer{Name: p[:i], Tag: p[i+1:]})
	}
	if len(out) == 0 {
		return defaultRosterPlayers()
	}
	return out
}

func defaultRosterPlayers() []riotPlayer {
	return []riotPlayer{
		{Name: "Heri", Tag: "BLUB"},
		{Name: "TheMisterED", Tag: "0007"},
		{Name: "Graussbyt", Tag: "5629"},
		{Name: "Lal6s9gne", Tag: "6641"},
		{Name: "Djibはコリーヌ お あいして", Tag: "LOVE"},
		{Name: "hhj", Tag: "8769"},
		{Name: "小胖子vincent", Tag: "4397"},
	}
}

func playerKey(n, t string) string {
	return strings.ToLower(strings.TrimSpace(n)) + "#" + strings.ToLower(strings.TrimSpace(t))
}

func matchIDFromJSON(raw json.RawMessage) string {
	var m struct {
		Metadata struct {
			MatchID string `json:"matchid"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	return strings.TrimSpace(m.Metadata.MatchID)
}

func matchStartMs(raw json.RawMessage) int64 {
	var m struct {
		Metadata struct {
			GameStart int64 `json:"game_start"`
		} `json:"metadata"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Metadata.GameStart
}

func matchPathSupportsStart(matchPath string) bool {
	return strings.Contains(matchPath, "v4/matches")
}

func buildMatchListURL(base, matchPath, region, platform, name, tag string, q url.Values) string {
	u := base + "/" + matchPath + "/" + url.PathEscape(region)
	if matchPathSupportsStart(matchPath) {
		u += "/" + url.PathEscape(platform)
	}
	u += "/" + url.PathEscape(name) + "/" + url.PathEscape(tag)
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func fetchPlayerMatchesQuery(ctx context.Context, base, matchPath, region, platform, apiKey, name, tag string, q url.Values) (int, []byte, error) {
	u := buildMatchListURL(base, matchPath, region, platform, name, tag, q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", apiKey)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func parseHenrikMatchList(body []byte) ([]json.RawMessage, error) {
	var outer struct {
		Status int               `json:"status"`
		Data   []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		return nil, err
	}
	if outer.Status != 200 {
		return nil, fmt.Errorf("upstream json status %d", outer.Status)
	}
	if outer.Data == nil {
		return []json.RawMessage{}, nil
	}
	return outer.Data, nil
}

func fetchPlayerMMRHistory(ctx context.Context, base, region, apiKey string, p riotPlayer) (map[string]int64, string) {
	u := base + "/v1/mmr-history/" + url.PathEscape(region) + "/" + url.PathEscape(p.Name) + "/" + url.PathEscape(p.Tag)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Sprintf("%s#%s: %v", p.Name, p.Tag, err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", apiKey)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Sprintf("%s#%s: %v", p.Name, p.Tag, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Sprintf("%s#%s: %v", p.Name, p.Tag, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Sprintf("%s#%s: mmr-history HTTP %d", p.Name, p.Tag, resp.StatusCode)
	}
	var outer struct {
		Status int `json:"status"`
		Data   []struct {
			MatchID     string `json:"match_id"`
			MMRChange   int64  `json:"mmr_change_to_last_game"`
			RankedDelta int64  `json:"ranked_rating_change"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		return nil, fmt.Sprintf("%s#%s: mmr-history decode: %v", p.Name, p.Tag, err)
	}
	if outer.Status != 200 {
		return nil, fmt.Sprintf("%s#%s: mmr-history status %d", p.Name, p.Tag, outer.Status)
	}
	out := make(map[string]int64, len(outer.Data))
	for _, row := range outer.Data {
		id := strings.TrimSpace(row.MatchID)
		if id == "" {
			continue
		}
		delta := row.MMRChange
		if delta == 0 && row.RankedDelta != 0 {
			delta = row.RankedDelta
		}
		out[id] = delta
	}
	return out, ""
}

func envInt(key string, def int) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}

// Fetches up to maxPages of competitive mode matches (size 10 each). Returns deduped matches for this player, next API start offset, exhausted, optional warning.
func fetchPlayerCompetitivePages(
	ctx context.Context,
	base, matchPath, region, platform, apiKey string,
	p riotPlayer,
	startFrom int,
	maxPages int,
) ([]json.RawMessage, int, bool, string) {
	pageSize := 10
	if maxPages < 1 {
		maxPages = 1
	}
	var collected []json.RawMessage
	seen := make(map[string]bool)
	start := startFrom
	useStart := matchPathSupportsStart(matchPath)

	for page := 0; page < maxPages; page++ {
		if !useStart && page > 0 {
			break
		}
		q := url.Values{}
		q.Set("mode", "competitive")
		q.Set("size", strconv.Itoa(pageSize))
		if useStart {
			q.Set("start", strconv.Itoa(start))
		}
		code, body, err := fetchPlayerMatchesQuery(ctx, base, matchPath, region, platform, apiKey, p.Name, p.Tag, q)
		if err != nil {
			return collected, start, true, fmt.Sprintf("%s#%s: %v", p.Name, p.Tag, err)
		}
		if code != http.StatusOK {
			return collected, start, true, fmt.Sprintf("%s#%s: HTTP %d", p.Name, p.Tag, code)
		}
		batch, err := parseHenrikMatchList(body)
		if err != nil {
			return collected, start, true, fmt.Sprintf("%s#%s: %v", p.Name, p.Tag, err)
		}
		if len(batch) == 0 {
			return collected, start, true, ""
		}
		for _, m := range batch {
			id := matchIDFromJSON(m)
			if id == "" {
				continue
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			collected = append(collected, m)
		}
		start += len(batch)
		if len(batch) < pageSize {
			return collected, start, true, ""
		}
		if !useStart {
			return collected, start, true, ""
		}
	}
	return collected, start, false, ""
}

func mergeMatchesIntoAgg(agg map[string]*aggEntry, p riotPlayer, matches []json.RawMessage, rrByMatchID map[string]int64, noID *int) {
	pk := playerKey(p.Name, p.Tag)
	for _, m := range matches {
		id := matchIDFromJSON(m)
		if id == "" {
			*noID++
			id = "noid-" + strconv.Itoa(*noID)
		}
		e := agg[id]
		if e == nil {
			e = &aggEntry{
				match:      m,
				playerK:    make(map[string]riotPlayer),
				rrByPlayer: make(map[string]int64),
			}
			agg[id] = e
		}
		e.playerK[pk] = riotPlayer{Name: p.Name, Tag: p.Tag}
		if rrByMatchID != nil {
			if delta, ok := rrByMatchID[id]; ok {
				e.rrByPlayer[pk] = delta
			}
		}
	}
}

func aggToSortedRoster(agg map[string]*aggEntry) []rosterMatchOut {
	out := make([]rosterMatchOut, 0, len(agg))
	for _, e := range agg {
		rst := make([]riotPlayer, 0, len(e.playerK))
		for _, pl := range e.playerK {
			rst = append(rst, pl)
		}
		sort.Slice(rst, func(i, j int) bool {
			if strings.EqualFold(rst[i].Name, rst[j].Name) {
				return strings.ToLower(rst[i].Tag) < strings.ToLower(rst[j].Tag)
			}
			return strings.ToLower(rst[i].Name) < strings.ToLower(rst[j].Name)
		})
		out = append(out, rosterMatchOut{Match: e.match, Roster: rst, RRByPlayer: e.rrByPlayer})
	}
	sort.Slice(out, func(i, j int) bool {
		return matchStartMs(out[i].Match) > matchStartMs(out[j].Match)
	})
	return out
}

func computeHasMore(resume []playerResume) bool {
	for _, r := range resume {
		if !r.Exhausted {
			return true
		}
	}
	return false
}

func handleRosterMatches(w http.ResponseWriter, r *http.Request, allowed []string, base, matchPath, apiKey string) {
	applyCORS(w, r, allowed)
	region := strings.TrimSpace(os.Getenv("VALORANT_REGION"))
	if region == "" {
		region = "eu"
	}
	platform := strings.TrimSpace(os.Getenv("VALORANT_PLATFORM"))
	if platform == "" {
		platform = "pc"
	}
	players := parsePlayersList(os.Getenv("VALORANT_PLAYERS"))
	initialPages := envInt("VALORANT_ROSTER_INITIAL_PAGES", 3)

	agg := make(map[string]*aggEntry)
	warnings := make([]string, 0)
	noID := 0
	resume := make([]playerResume, 0, len(players))

	for _, p := range players {
		rrByMatchID, rrWarn := fetchPlayerMMRHistory(r.Context(), base, region, apiKey, p)
		if rrWarn != "" {
			warnings = append(warnings, rrWarn)
		}
		matches, next, exhausted, w := fetchPlayerCompetitivePages(r.Context(), base, matchPath, region, platform, apiKey, p, 0, initialPages)
		if w != "" {
			warnings = append(warnings, w)
		}
		mergeMatchesIntoAgg(agg, p, matches, rrByMatchID, &noID)
		resume = append(resume, playerResume{Name: p.Name, Tag: p.Tag, NextStart: next, Exhausted: exhausted})
	}

	out := aggToSortedRoster(agg)
	resp := rosterAPIResponse{
		Data:     out,
		Players:  players,
		Warnings: warnings,
		Resume:   resume,
		HasMore:  computeHasMore(resume),
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(resp)
}

func handleRosterMatchesMore(w http.ResponseWriter, r *http.Request, allowed []string, base, matchPath, apiKey string) {
	applyCORS(w, r, allowed)
	region := strings.TrimSpace(os.Getenv("VALORANT_REGION"))
	if region == "" {
		region = "eu"
	}
	platform := strings.TrimSpace(os.Getenv("VALORANT_PLATFORM"))
	if platform == "" {
		platform = "pc"
	}
	players := parsePlayersList(os.Getenv("VALORANT_PLAYERS"))
	loadMorePages := envInt("VALORANT_ROSTER_LOAD_MORE_PAGES", 3)

	var reqBody rosterContinueRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&reqBody); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid JSON body"}`))
		return
	}
	if len(reqBody.Resume) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"missing resume"}`))
		return
	}
	if reqBody.PagesPerRequest > 0 {
		loadMorePages = reqBody.PagesPerRequest
	}

	known := make(map[string]struct{}, len(reqBody.KnownMatchIDs))
	for _, id := range reqBody.KnownMatchIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			known[id] = struct{}{}
		}
	}

	deltaAgg := make(map[string]*aggEntry)
	warnings := make([]string, 0)
	noID := 0
	newResume := make([]playerResume, 0, len(reqBody.Resume))

	for _, pr := range reqBody.Resume {
		if pr.Exhausted {
			newResume = append(newResume, pr)
			continue
		}
		p := riotPlayer{Name: pr.Name, Tag: pr.Tag}
		rrByMatchID, rrWarn := fetchPlayerMMRHistory(r.Context(), base, region, apiKey, p)
		if rrWarn != "" {
			warnings = append(warnings, rrWarn)
		}
		matches, next, exhausted, w := fetchPlayerCompetitivePages(r.Context(), base, matchPath, region, platform, apiKey, p, pr.NextStart, loadMorePages)
		if w != "" {
			warnings = append(warnings, w)
		}
		mergeMatchesIntoAgg(deltaAgg, p, matches, rrByMatchID, &noID)
		newResume = append(newResume, playerResume{Name: p.Name, Tag: p.Tag, NextStart: next, Exhausted: exhausted})
	}

	fullDelta := aggToSortedRoster(deltaAgg)
	delta := make([]rosterMatchOut, 0, len(fullDelta))
	for _, row := range fullDelta {
		id := matchIDFromJSON(row.Match)
		if id == "" {
			continue
		}
		if _, ok := known[id]; ok {
			continue
		}
		delta = append(delta, row)
	}

	resp := rosterAPIResponse{
		Data:     delta,
		Players:  players,
		Warnings: warnings,
		Resume:   newResume,
		HasMore:  computeHasMore(newResume),
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(resp)
}
