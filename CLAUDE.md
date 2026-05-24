# Frederic Fanclub — CLAUDE.md

## Project Overview

Full-stack esports fan engagement platform for a Valorant team called "FRED" (Frederic Fanclub / Fred Esports). Combines match tracking, a trading card economy, live prop betting, and a community radio.

- **Frontend:** Vanilla JS + HTML/CSS, served statically (Node on port 3000)
- **Backend:** Go server (`server/main.go`, 8600+ lines) on port 8080, SQLite (`server/fred-dev.db`)
- **Auth:** Discord OAuth2 — session cookie, `credentials: 'include'` on all API calls
- **API base:** `http://localhost:8080` (local) / `https://api.fredericfan.club` (prod) — auto-detected in `getApiBase()`

---

## Directory Structure

```
FredericFanclub/
├── frontend/
│   ├── *.html              — all pages (matches, premier, packs, inventory, catalog, radio, betting, leaderboard, tradeup, player-*)
│   ├── css/
│   │   ├── styles.css      — global dark theme (Orbitron + Rajdhani fonts)
│   │   └── premier.css     — premier page-specific styles (self-contained, includes nav/hero/layout)
│   ├── js/
│   │   ├── site.js         — main app logic (~1922 lines): auth, matches, packs, betting, quests, radio
│   │   └── premier.js      — premier dashboard: player stats, map win rates, match scoreboard
│   ├── images/             — player photos, logos
│   └── audio/              — radio tracks (15) + intermissions (6)
└── server/
    ├── main.go             — all API endpoints, WebSocket, economy logic
    ├── roster.go           — Valorant API integration, match syncing
    ├── database/           — SQL migrations
    ├── discordbot/         — Discord bot
    └── fred-dev.db         — SQLite database
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

### Matches (matches.html vs premier.html)

- `matches.html` — roster match history, uses `styles.css`, fetches `/api/matches/roster`
- `premier.html` — advanced stats dashboard, uses `premier.css` (self-contained), fetches `/api/matches/premier` + `/api/matches/stats`
- Both share `site.js`; `premier.html` also loads `premier.js`

### Betting

- Admin publishes prop markets → users bet FT on over/under lines
- WebSocket at `/api/ws/betting` for live feed
- Admin controls: preview prop → publish → lock → resolve/cancel

---

## API Endpoints (Selected)

```
GET  /api/user/me              — current user profile + FT balance
GET  /api/matches/roster       — match history (paginated, player-filtered)
GET  /api/matches/premier      — premier matches with full scoreboards
GET  /api/matches/stats        — aggregated player stats
POST /api/economy/buy-pack     — open a pack (costs 250 FT, returns card)
POST /api/economy/daily        — claim daily reward
POST /api/economy/shred        — shred card for FT
POST /api/economy/trade-up     — trade 5 cards → 1 higher rarity
GET  /api/inventory            — user's owned cards
GET  /api/catalog              — all cards with unlock status
GET  /api/quests               — daily missions
POST /api/quests/verify        — check and complete a quest
POST /api/betting/place        — place a bet
GET  /api/betting/market       — active market
WS   /api/ws/betting           — live bet stream
```

Admin endpoints require `X-Admin-Token` header (defined in Go server env).

---

## Next Steps

### 1. Sync `matches.html` look with `premier.html`

`premier.html` uses `premier.css` which is a fully self-contained stylesheet with a polished dark-cyber aesthetic (self-contained nav, hero, section headings, card grid styles). `matches.html` uses `styles.css` which has an older/different look.

**What needs to change:**
- Switch `matches.html` to use `premier.css` instead of (or in addition to) `styles.css`, OR port the relevant design tokens and component styles into `styles.css`
- Align the hero section (`<header class="page-hero">`) — `premier.html` hero has subtitle text (`<p>`) under the `<h1>`, `matches.html` does not
- Match card styling should use the same card/border/shadow pattern as premier's match list
- Section headings should use the `.section-heading` class from `premier.css`
- Filter bar and search box should match premier's `.filter-bar` / `.filter-btn` / `.search-box` pattern
- The player filter toggle buttons (`#playerFilterBar`) should be restyled to match premier's pill/toggle aesthetic
- Keep `matches.html` JS logic (roster filtering, pagination, RR delta) intact — only visual changes

**Key classes in `premier.css` to replicate or reference:**
- `.section-heading`, `.filter-bar`, `.filter-btn`, `.filter-sep`, `.search-box`
- `.match-list`, `.match-card`, `.scoreboard-table`
- `.page-hero`, `.bg-waves`

---

### 2. Radio Redemption Codes — New Way to Earn FT

Add a "redeem a code" mechanic on `radio.html`: secret codes announced on-air that listeners can type in to claim a one-time FT reward. This incentivizes actually tuning in live.

**Frontend (`radio.html` + `site.js`):**
- Add a small input + button UI below (or overlaid on) the radio player: `[ Enter code... ] [REDEEM]`
- On submit, call `POST /api/economy/redeem-code` with `{ code: "XXXXX" }`
- Show success (e.g., "+150 FT claimed!") or error ("Code invalid / already used") feedback
- Refresh wallet on success via `window.loadUserProfile()`

**Backend (`main.go` or new file):**
- New table: `redeem_codes (code TEXT PK, reward_ft INT, max_uses INT, uses_so_far INT, expires_at TIMESTAMP)`
- New table: `code_redemptions (user_id TEXT, code TEXT, redeemed_at TIMESTAMP, PRIMARY KEY(user_id, code))` — prevents double-dip
- `POST /api/economy/redeem-code` endpoint: validate code exists, not expired, user hasn't redeemed it, uses < max_uses → credit FT, increment uses
- Admin endpoint: `POST /api/admin/create-code` with `{ code, reward_ft, max_uses, expires_at }` to generate codes
- Economy balance: keep codes in 50–300 FT range; daily is 250 FT so codes shouldn't trivially replace it

---

### 3. More Incentive to Get Cards

Multiple angles — pick what fits the economy:

**A. Collection Milestone Rewards (Catalog completion bonuses)**
- When a player unlocks N% of a season's cards (e.g., 25%, 50%, 75%, 100%), they receive FT or a guaranteed-rarity pack
- Frontend: show progress bar in `catalog.html` with milestone markers; animate a reward modal on crossing thresholds
- Backend: track `collection_milestones` table, check on every card unlock/shred

**B. Duplicate Card Shred Bonus**
- If a player already owns a card and pulls a duplicate, auto-shred gives a small bonus (e.g., +10% FT over base shred value)
- Encourages pulling packs even when mostly complete

**C. "Set Completion" Bonus**
- Owning all cards of a specific rarity tier in a season grants a special badge or FT bonus
- Visible in catalog with a "COMPLETE" stamp UI treatment

**D. Showcase / Profile Integration**
- Let users display their rarest card on their leaderboard entry — social flex for collecting
- Adds status incentive without inflating FT economy

**E. Card-Gated Features**
- Small quality-of-life perks tied to card ownership (e.g., owning a player's card shows their stats badge in match history)
- Non-economic incentive that doesn't break token balance

---

### 4. Season 2 Pack Filter — Only Season 2 Cards Drop

When Season 2 launches, packs should only drop Season 2 cards. The backend's `POST /api/economy/buy-pack` logic controls what card is minted.

**Backend (`main.go`):**
- The card selection query in `buy-pack` currently pulls from all available cards
- Add a `current_pack_season` config (env var, DB setting, or admin endpoint) that the pack-opening logic reads
- Filter the card pool: `WHERE season = $current_season` before the rarity-weighted random selection
- Admin endpoint: `POST /api/admin/set-pack-season` with `{ season: "Season 2" }` to flip the active season
- Keep old cards in the catalog/inventory — this only affects what *new* packs can drop

**Frontend (`packs.html`):**
- Display the active season name on the pack UI (e.g., "SEASON 2 PACKS" label under the case)
- The filler cards in the spinner animation are cosmetic-only — no backend change needed there

**Migration note:** Ensure Season 2 cards exist in the DB (`cards` table with `season = 'Season 2'`) before flipping the season, or buy-pack will return errors.

---

## Development Notes

- All pages share `site.js` for auth, wallet, and shared utilities
- `premier.html` is the most visually polished page — use it as the design reference
- Backend is a single large `main.go` file — search for endpoint strings (e.g., `"buy-pack"`) to find handler locations
- No build step for frontend — edit HTML/CSS/JS directly, hard-refresh to see changes
- DB is SQLite at `server/fred-dev.db` — use standard SQLite tooling to inspect
- Version cache-bust: JS files use `?v=` query params (e.g., `site.js?v=mono8`) — increment when deploying changes
