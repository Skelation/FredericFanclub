# Frederic Fanclub — CLAUDE.md

## Project Overview

Full-stack esports fan engagement platform for a Valorant team called "FRED" (Frederic Fanclub / Fred Esports). Combines match tracking, a trading card economy, live prop betting, and a community radio.

- **Frontend:** Vanilla JS + HTML/CSS, served statically (Node on port 3000)
- **Backend:** Go server (split across `server/*.go`, ~3977 total lines) on port 8080, SQLite (`server/database/fred.db`)
- **Auth:** Discord OAuth2 — session cookie, `credentials: 'include'` on all API calls
- **API base:** `http://localhost:8080` (local) / `https://api.fredericfan.club` (prod) — auto-detected in `getApiBase()`

---

## Directory Structure

```
FredericFanclub/
├── frontend/
│   ├── *.html              — all pages (matches, premier, packs, inventory, catalog, radio, betting,
│   │                         leaderboard, tradeup, admin, player-*)
│   ├── css/
│   │   ├── styles.css      — global dark theme (Orbitron + Rajdhani fonts)
│   │   └── premier.css     — premier page-specific styles (self-contained, includes nav/hero/layout)
│   ├── js/
│   │   ├── site.js         — main app logic (~2007 lines): auth, matches, packs, betting, quests, radio
│   │   └── premier.js      — premier dashboard: player stats, map win rates, match scoreboard
│   ├── images/             — player photos, logos
│   └── audio/              — radio tracks (15) + intermissions (6)
└── server/
    ├── main.go             — startup, globals, WebSocket broadcaster (99 lines)
    ├── db.go               — database init, all CREATE TABLE / migrations
    ├── auth.go             — Discord OAuth2 routes
    ├── user.go             — /api/user/me, badges, showcase, leaderboard
    ├── economy.go          — buy-pack, daily, trade-up, shred, redeem-code
    ├── inventory.go        — /api/inventory, /api/catalog
    ├── radio.go            — radio drop scheduler, active-drop, claim, captcha
    ├── betting.go          — WebSocket, market, place-bet
    ├── matches.go          — roster, upcoming matches, /api/packs/season
    ├── quests.go           — quests list + verify
    ├── admin.go            — all /api/admin/* endpoints
    ├── util.go             — shared helpers (CORS, logging, dotenv, etc.)
    ├── roster.go           — Valorant API integration, match syncing
    ├── internal/premier/   — premier stats module (premier.go, stats.go)
    ├── database/           — SQL migrations + fred.db (path from DB_PATH env, default ./database/fred.db)
    └── discordbot/         — Discord bot
```

---

## Design System

**Colors (CSS variables in `premier.css` / `styles.css`):**
- `--cyan: #00d4ff` — primary accent (highlights, active nav, borders)
- `--red: #ff4655` / `#ff3c3c` — losses, danger
- `--green: #00ff64` / `#39ff85` — wins, good stats
- `--gold: #ffd94a` — premium/radiant
- `--bg: #050810` — page background
- `--card: #0d1428` — card/panel background
- `--border: rgba(0,212,255,.13)` — subtle borders

**Fonts:** `Orbitron` (headings/labels), `Rajdhani` (body/UI)

**Rarity Colors:**
- Iron: `#aaa` | Bronze: `#cd7f32` | Diamond: `#b982ff`
- Ascendant: `#2bc97e` | Immortal: `#ff4655` | Radiant: `#ffea82`

**CSS pattern:** `premier.css` is fully self-contained (includes nav, hero, container, section-heading). `styles.css` is the global sheet used by all other pages.

---

## Key Systems

### Freddy Token (FT) Economy

| Action | Amount | Endpoint |
|--------|--------|----------|
| Daily Supply Drop | +250 FT | `POST /api/economy/daily` |
| Open Pack | −250 FT | `POST /api/economy/buy-pack` |
| Quest completion | +50–200 FT | `POST /api/quests/verify` |
| Shred card (Iron) | +20 FT | `POST /api/economy/shred` |
| Shred card (Bronze) | +50 FT | |
| Shred card (Diamond) | +200 FT | |
| Shred card (Ascendant) | +500 FT | |
| Shred card (Immortal) | +2,500 FT | |
| Shred card (Radiant) | +10,000 FT | |
| Betting win | +amount × odds | `POST /api/betting/place` |

Wallet is displayed in navbar and refreshed via `window.loadUserProfile()`.

### Pack / Card System

- Cost: 250 FT per pack → `POST /api/economy/buy-pack`
- 6-second spinner animation (60 mystery boxes, winning card at position 45)
- Server controls what card drops; frontend just animates
- Card actions: View (full-screen modal), Shred (→ FT), Trade-Up (5 same-rarity → 1 higher)
- Catalog at `/api/catalog` — cards have `{id, name, rarity, image_url, season, unlocked, quantity}`
- **Season field on cards controls which season a card belongs to**

### Radio

- Global sync: all users hear the same track simultaneously, derived from server time
- 15 music tracks + 6 intermission clips, 1 intermission injected every 3 songs
- `audio/` directory holds all `.mp3` files
- Volume persisted to `localStorage`

**Radio Drops** (auto-scheduled FT rewards for live listeners):
- Server schedules random drops every 15–40 min (configurable via `server_config` table)
- Drop is active for a 7-minute claim window; users solve a CAPTCHA then call `POST /api/radio/claim`
- Frontend polls `GET /api/radio/active-drop`; shows claim UI when a drop is live
- Admin can manually trigger a drop via `POST /api/admin/radio-drop`
- Claims tracked in `radio_drops` + `radio_drop_claims` tables (prevents double-claim)

### Matches (matches.html vs premier.html)

- `matches.html` — roster match history, uses `styles.css`, fetches `/api/matches/roster`
- `premier.html` — advanced stats dashboard, uses `premier.css` (self-contained), fetches `/api/matches/premier` + `/api/matches/stats`
- Both share `site.js`; `premier.html` also loads `premier.js`

### User Badges & Showcase

- Badges awarded automatically (e.g., milestones) and stored in `user_badges` table
- Users set a showcase card via `POST /api/user/showcase`; displayed on leaderboard entries
- `GET /api/user/badges` returns earned badge list; `GET /api/user/showcase` returns current showcase card

### Redemption Codes

- Admin creates codes via `POST /api/admin/create-code` with `{ code, reward_ft, max_uses, expires_at }`
- Users redeem via `POST /api/economy/redeem-code` with `{ code }`
- Tracked in `redeem_codes` + `code_redemptions` tables — one claim per user per code
- Economy balance: keep codes in 50–300 FT range (daily is 250 FT)

### Betting

- Admin publishes prop markets → users bet FT on over/under lines
- WebSocket at `/api/ws/betting` for live feed
- Admin controls: preview prop → publish → lock → resolve/cancel

---

## API Endpoints (Selected)

```
GET  /api/user/me                  — current user profile + FT balance
GET  /api/user/badges              — earned badges list
GET  /api/user/showcase            — user's showcase card
POST /api/user/showcase            — set showcase card
GET  /api/leaderboard              — FT leaderboard
GET  /api/matches/roster           — match history (paginated, player-filtered)
POST /api/matches/roster/more      — load more roster matches
GET  /api/matches/upcoming         — scheduled upcoming matches
GET  /api/matches/premier          — premier matches with full scoreboards
GET  /api/matches/stats            — aggregated player stats
GET  /api/packs/season             — current pack season
POST /api/economy/buy-pack         — open a pack (costs 250 FT, returns card)
POST /api/economy/daily            — claim daily reward
POST /api/economy/shred            — shred card for FT
POST /api/economy/trade-up         — trade 5 cards → 1 higher rarity
POST /api/economy/redeem-code      — redeem a radio code for FT
GET  /api/inventory                — user's owned cards
GET  /api/catalog                  — all cards with unlock status
GET  /api/quests                   — daily missions
POST /api/quests/verify            — check and complete a quest
POST /api/betting/place            — place a bet
GET  /api/betting/market           — active market
WS   /api/ws/betting               — live bet stream
GET  /api/radio/active-drop        — current radio drop (if any)
GET  /api/radio/captcha/new        — get a CAPTCHA challenge for claiming
POST /api/radio/claim              — claim active radio drop (requires CAPTCHA)

Admin (X-Admin-Token header required):
POST /api/admin/create-code        — create a redemption code
POST /api/admin/radio-drop         — manually trigger a radio drop
POST /api/admin/schedule-match     — add an upcoming match
POST /api/admin/cards              — add a card to the catalog
POST /api/admin/delete-card        — remove a card
POST /api/admin/tokens             — adjust a user's FT balance
POST /api/admin/link-user          — link Discord user to Valorant player
GET  /api/admin/users              — list all users
POST /api/admin/preview-prop       — create a preview prop market
POST /api/admin/publish-prop       — publish prop to users
POST /api/admin/lock-prop          — lock betting on active market
POST /api/admin/resolve-prop       — settle bets on active market
POST /api/admin/cancel-market      — cancel active market (refund bets)
POST /api/admin/set-pack-season    — change the active pack season
```

---

## Next Steps

### 1. Minigames Page (`/minigames`)

A dedicated minigames page where players earn Freddy Tokens. One minigame is **featured each week** (rotates every Monday, set in `server_config` or hardcoded schedule) and awards 1.5× FT. All minigames enforce daily limits server-side.

#### Card Flip Memory
- Classic 4×4 memory card matching game using FRED cards pulled from `/api/catalog`
- Timed — faster completion = more FT
- Daily attempt limit: 3 tries, best score counts
- Rewards: 50–200 FT based on completion time

#### Callout Drop
- A Valorant map screenshot is shown with a red dot marking a location
- Player types the callout name; server checks against a stored answer list
- 5 locations per session, one session per day
- Rewards: 20 FT per correct answer (max 100 FT/day)

#### Last Click Standing
- A shared 2-hour countdown timer visible to all players
- FT tiers unlock as time ticks down — less time remaining = higher reward:
  | Time Remaining | Reward |
  |----------------|--------|
  | 2:00 – 1:00    | 75 FT  |
  | 1:00 – 0:30    | 150 FT |
  | 0:30 – 0:10    | 350 FT |
  | 0:10 – 0:02    | 750 FT |
  | Last 2 min     | 1500 FT |
- When any player claims, they lock in their tier and the timer resets to 2 hours for everyone else
- **Only runs during active hours (09:00–23:00)** — timer pauses outside this window so it doesn't idle overnight
- Each player can claim once per day (20-hour cooldown enforced server-side via timestamp)
- Live panel shows who has already claimed and which tier they received
- New tables: `last_click_claims` (user_id, claimed_at, ft_earned)


---

## Development Notes

- All pages share `site.js` for auth, wallet, and shared utilities
- `premier.html` is the most visually polished page — use it as the design reference
- Backend is split into domain files — search for endpoint strings (e.g., `"buy-pack"`) within the relevant domain file (`economy.go`, `radio.go`, etc.) to find handler locations; `main.go` is now only startup/globals
- No build step for frontend — edit HTML/CSS/JS directly, hard-refresh to see changes
- DB is SQLite; path set via `DB_PATH` env var, defaults to `./database/fred.db` (relative to the `server/` directory)
- Radio drop config lives in the `server_config` table (`radio_drop_enabled`, `radio_drop_min_interval`, `radio_drop_max_interval`, `radio_drop_window_sec`)
- Version cache-bust: JS files use `?v=` query params (e.g., `site.js?v=mono8`) — increment when deploying changes
