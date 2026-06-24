// --- AUTO ENVIRONMENT DETECTOR ---
function getApiBase() {
    if (window.location.hostname === "localhost" || window.location.hostname === "127.0.0.1") {
        return window.location.origin;
    }
    return "https://api.fredericfan.club";
}

document.addEventListener('DOMContentLoaded', () => {
    // --- DYNAMIC DISCORD LOGIN ---
    const loginBtn = document.getElementById('discordLoginBtn');
    if (loginBtn) {
        // Remove href just in case it was left in the HTML
        loginBtn.removeAttribute('href'); 
        
        loginBtn.addEventListener('click', (e) => {
            e.preventDefault(); // This stops your smooth-scroll script from crashing!
            e.stopPropagation(); // This prevents the click from bubbling up
            
            // Redirect the user to the correct API manually
            window.location.href = `${getApiBase()}/api/auth/discord`;
        });
    }
});

(function () {
    const observerOptions = {
        threshold: 0.1,
        rootMargin: '0px 0px -100px 0px'
    };

    const observer = new IntersectionObserver((entries) => {
        entries.forEach((entry) => {
            if (entry.isIntersecting) {
                entry.target.classList.add('visible');
            }
        });
    }, observerOptions);

    document.querySelectorAll('.fade-in').forEach((el) => observer.observe(el));

    document.querySelectorAll('a[href^="#"]').forEach((anchor) => {
        anchor.addEventListener('click', function (e) {
            const href = this.getAttribute('href');
            if (href.length <= 1) return;
            e.preventDefault();
            const target = document.querySelector(href);
            if (target) {
                target.scrollIntoView({ behavior: 'smooth', block: 'start' });
            }
        });
    });

    const heroLogoWrap = document.getElementById('heroLogoWrap');
    if (heroLogoWrap) {
        window.addEventListener('scroll', () => {
            const scrolled = window.pageYOffset;
            heroLogoWrap.style.transform = `translateY(${scrolled * 0.35}px)`;
        });
    }

    const matchList = document.getElementById('matchList');
    const matchFetchStatus = document.getElementById('matchFetchStatus');
    if (matchList && matchFetchStatus) {
        loadMatchHistory(matchList, matchFetchStatus);
    }

    initPlayerProfilePage();

    // --- AUTHENTICATION & WALLET ---
    window.loadUserProfile = async function(skipMarketReload = false) {
        const authContainer = document.getElementById('authContainer');
        if (!authContainer) return;

        // Make sure we have the API URL
        const meta = document.querySelector('meta[name="fred-api-base"]');
        const apiBase = getApiBase();

        try {
            // Fetch the profile. "credentials: 'include'" is CRITICAL so it sends the cookie!
            const res = await fetch(`${apiBase}/api/user/me`, {
                method: 'GET',
                credentials: 'include' 
            });

            if (res.ok) {
                const user = await res.json();
                
                // Replace the Login button with their profile & wallet
                authContainer.innerHTML = `
                    <div style="display: flex; align-items: center; gap: 12px; background: rgba(0,0,0,0.5); padding: 4px 12px 4px 4px; border-radius: 30px; border: 1px solid rgba(255,255,255,0.1);">
                        <img src="${user.avatar_url}" alt="Profile" style="width: 32px; height: 32px; border-radius: 50%; border: 2px solid #ff4655;">
                        <span style="font-family: 'Orbitron', sans-serif; color: #00ff64; font-weight: 700; font-size: 0.9rem;">
                            ${Math.round(user.fredtokens * 10) / 10} FT
                        </span>
                    </div>
                `;
                if (typeof loadBettingMarket === 'function') {
                    if (!skipMarketReload && typeof loadBettingMarket === 'function') {
                    loadBettingMarket();
                    }
                }
            }
        } catch (error) {
            console.error("Not logged in or API unreachable");
        }
    }

    // Call it when the page loads
    window.loadUserProfile();
})();

// Helper function to safely render text in HTML
function escapeHtml(str) {
    if (str === null || str === undefined) return '';
    return String(str)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;');
}

const PLAYER_PROFILES = {
    themistered: {
        name: 'TheMisterED',
        tag: '#0007',
        role: 'In-Game Leader',
        mainAgent: 'Omen',
        tagline: 'Calm caller with clutch timing.',
        bio: 'Leads team structure, keeps comms clean, and stabilizes late-round decisions.',
        image: 'images/players/profiles/themistered.png'
    },
    heri: {
        name: 'Heri',
        tag: '#BLUB',
        role: 'Controller / Flex',
        mainAgent: 'Brimstone',
        tagline: 'Utility-heavy, disciplined map control.',
        bio: 'Sets pace through smoke timing and post-plant setups to secure rounds.',
        image: 'images/players/profiles/heri.png'
    },
    hhj: {
        name: 'hhj',
        tag: '#8769',
        role: 'Duelist',
        mainAgent: 'Jett',
        tagline: 'Aggressive space creator.',
        bio: 'Looks for first picks and creates pressure to open sites for the team.',
        image: 'images/players/profiles/hhj.png'
    },
    djib: {
        name: 'Djib',
        tag: '#LOVE',
        role: 'Sentinel',
        mainAgent: 'Killjoy',
        tagline: 'Anchor specialist with strong lurk reads.',
        bio: 'Locks down flanks and controls rotations with strong utility discipline.',
        image: 'images/players/profiles/djib.png'
    },
    graussbyt: {
        name: 'Graussbyt',
        tag: '#5629',
        role: 'Initiator',
        mainAgent: 'Sova',
        tagline: 'Information engine of the team.',
        bio: 'Creates opening info and enables executes through timing and recon usage.',
        image: 'images/players/profiles/graussbyt.png'
    },
    lal6s9gne: {
        name: 'Lal6s9gne',
        tag: '#6641',
        role: 'Flex',
        mainAgent: 'Skye',
        tagline: 'Adaptable mid-round impact.',
        bio: 'Fills composition needs and supports both entry and retake structures.',
        image: 'images/players/profiles/lal6s9gne.png'
    },
    xtrix: {
        name: 'Xtrix',
        tag: '#DREAM',
        role: 'Duelist / Flex',
        mainAgent: 'Raze',
        tagline: 'Explosive entries and momentum plays.',
        bio: 'Creates high-tempo openings and converts pressure into site control.',
        image: 'images/players/profiles/xtrix.png'
    },
    vincent: {
        name: 'Vincent',
        tag: '#4397',
        role: 'Sentinel / Anchor',
        mainAgent: 'Cypher',
        tagline: 'Reliable hold and clean retakes.',
        bio: 'Brings consistency and structure with smart setups and post-plant presence.',
        image: 'images/players/profiles/vincent.png'
    }
};

function initPlayerProfilePage() {
    const nameEl = document.getElementById('playerProfileName');
    if (!nameEl) return;

    const params = new URLSearchParams(window.location.search);
    const id = (params.get('player') || '').trim().toLowerCase();
    const profile = PLAYER_PROFILES[id];
    if (!profile) return;

    const roleEl = document.getElementById('playerProfileRole');
    const taglineEl = document.getElementById('playerProfileTagline');
    const tagEl = document.getElementById('playerProfileTag');
    const agentEl = document.getElementById('playerProfileAgent');
    const bioEl = document.getElementById('playerProfileBio');
    const imageEl = document.getElementById('playerProfileImage');
    const fallbackEl = document.getElementById('playerProfileImageFallback');

    nameEl.textContent = profile.name;
    roleEl.textContent = profile.role;
    taglineEl.textContent = profile.tagline;
    tagEl.textContent = profile.tag;
    agentEl.textContent = profile.mainAgent;
    bioEl.textContent = profile.bio;

    imageEl.alt = `${profile.name} portrait`;
    imageEl.src = profile.image;
    imageEl.onerror = function () {
        this.style.display = 'none';
        fallbackEl.textContent = String(profile.name || '?').slice(0, 1).toUpperCase();
        fallbackEl.classList.add('player-profile-image-fallback--show');
    };
}

function formatMatchDate(meta) {
    const patched = meta && meta.game_start_patched;
    const raw = meta && meta.game_start;
    const s = patched || raw || '';
    if (!s) return '—';
    const ms = Date.parse(s);
    if (!Number.isNaN(ms)) {
        return new Date(ms).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
    }
    const str = String(s);
    return str.length > 14 ? str.slice(0, 14) : str;
}

function outcomeForPlayer(match, riotName, riotTag) {
    const me = findPlayerInMatch(match, riotName, riotTag);
    if (!me || !me.team) return null;

    let myTeam = null;
    // Smart switch: v4 array vs v3 object
    if (Array.isArray(match.teams)) {
        myTeam = match.teams.find(t => String(t.team_id).toLowerCase() === String(me.team).toLowerCase());
    } else if (match.teams) {
        myTeam = match.teams[String(me.team).toLowerCase()];
    }

    if (myTeam && typeof myTeam.won === 'boolean') return myTeam.won ? 'win' : 'loss';
    if (myTeam && typeof myTeam.has_won === 'boolean') return myTeam.has_won ? 'win' : 'loss';

    return null;
}

function findPlayerInMatch(match, riotName, riotTag) {
    let players = match && match.players;
    // Smart switch: If it's the old v3 object, extract the array
    if (players && !Array.isArray(players) && players.all_players) players = players.all_players;
    if (!Array.isArray(players)) return null;
    
    return players.find(p => p && String(p.name).toLowerCase() === String(riotName).toLowerCase() && String(p.tag).toLowerCase() === String(riotTag).toLowerCase()) || null;
}

function firstNumericFromKeys(obj, keys) {
    if (!obj || typeof obj !== 'object') return null;
    for (const key of keys) {
        if (!(key in obj)) continue;
        const raw = obj[key];
        if (raw === null || raw === undefined || raw === '') continue;
        const n = Number(raw);
        if (Number.isFinite(n)) return n;
    }
    return null;
}

function ratingDeltaForPlayer(match, riotName, riotTag) {
    const me = findPlayerInMatch(match, riotName, riotTag);
    if (!me) return null;

    // Handle common field naming across different match payload versions.
    const before = firstNumericFromKeys(me, ['mmr_before', 'ranked_rating_before']);
    const after = firstNumericFromKeys(me, ['mmr_after', 'ranked_rating_after']);
    if (before !== null && after !== null) {
        return Math.round(after - before);
    }

    return firstNumericFromKeys(me, ['mmr_change_to_last_game', 'ranked_rating_change']);
}

function playerKeyJs(name, tag) {
    return `${String(name).toLowerCase()}#${String(tag).toLowerCase()}`;
}

function playerImageFilename(name, tag) {
    const key = `${String(name).trim()}-${String(tag).trim()}`.toLowerCase();
    return key.replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
}

function makePlayerAvatar(name, tag) {
    const filename = playerImageFilename(name, tag);
    // Add portraits to images/players using "<name>-<tag>.png" slug format.
    const src = `images/players/${filename}.png`;
    const fallback = String(name || '?').slice(0, 1).toUpperCase();
    return `
        <div class="match-player-avatar" title="${escapeHtml(`${name}#${tag}`)}">
            <img src="${escapeHtml(src)}" alt="${escapeHtml(name)} portrait" loading="lazy"
                 onerror="this.style.display='none'; this.parentElement.classList.add('match-player-avatar--fallback'); this.parentElement.textContent='${escapeHtml(fallback)}';">
        </div>
    `;
}

function isCompetitiveMode(mode) {
    if (!mode || typeof mode !== 'string') return false;
    const m = mode.trim().toLowerCase();
    return m === 'competitive' || m === 'premier';
}

function renderMatchRow(match, riotName, riotTag, rrOverride = null, roster = []) {
    const meta = match.metadata || {};
    // Extract map name correctly for both v3 and v4
    const mapName = typeof meta.map === 'object' ? meta.map.name : (meta.map || 'Unknown map');
    
    let red = '—', blue = '—';
    if (Array.isArray(match.teams)) {
        const rt = match.teams.find(t => t.team_id === 'Red');
        const bt = match.teams.find(t => t.team_id === 'Blue');
        red = rt ? rt.rounds_won : '—';
        blue = bt ? bt.rounds_won : '—';
    } else if (match.teams) {
        red = match.teams.red?.rounds_won ?? '—';
        blue = match.teams.blue?.rounds_won ?? '—';
    }

    const outcome = outcomeForPlayer(match, riotName, riotTag);
    console.log(outcome)
    
    const rrDelta = (rrOverride !== null && rrOverride !== undefined && Number.isFinite(Number(rrOverride))) 
        ? Number(rrOverride) 
        : ratingDeltaForPlayer(match, riotName, riotTag);
    
    let rrClass = 'match-rating-change--unknown';
    let rrLabel = 'RR —';
    if (rrDelta !== null) {
        if (rrDelta > 0) {
            rrClass = 'match-rating-change--gain';
            rrLabel = `RR +${rrDelta}`;
        } else if (rrDelta < 0) {
            rrClass = 'match-rating-change--loss';
            rrLabel = `RR ${rrDelta}`;
        } else {
            rrClass = 'match-rating-change--even';
            rrLabel = 'RR +0';
        }
    }
    
    let resultClass = 'match-result--upcoming';
    let resultLabel = 'Draw';
    if (outcome === 'win') {
        resultClass = 'match-result--win';
        resultLabel = 'Win';
    } else if (outcome === 'loss') {
        resultClass = 'match-result--loss';
        resultLabel = 'Loss';
    } else if (outcome === null && (red !== '—' || blue !== '—')) {
        resultLabel = '—';
    }

    let rosterLabelHTML = '';
    if (roster && roster.length > 0) {
        const rosterNames = roster.map(p => escapeHtml(p.name)).join(' + ');
        rosterLabelHTML = `<div class="match-roster-label">👥 ${rosterNames}</div>`;
    }

    const li = document.createElement('li');
    li.className = 'match-card';
    li.innerHTML = `
        ${makePlayerAvatar(riotName, riotTag)}
        <div class="match-main">
            ${rosterLabelHTML}
            <h3>${escapeHtml(mapName)}</h3>
            <p class="match-player">${escapeHtml(riotName)}</p>
            <p class="match-scoreline">Attackers ${escapeHtml(String(red))} – ${escapeHtml(String(blue))} Defenders</p>
            <p class="match-rating-change ${rrClass}">${escapeHtml(rrLabel)}</p>
        </div>
        <span class="match-result ${resultClass}">${escapeHtml(resultLabel)}</span>
    `;
    return li;
}

function renderMatchRow(match, riotName, riotTag, rrOverride = null, roster = []) {
    const meta = match.metadata || {};
    const mapName = meta.map || 'Unknown map';
    const red = (match.teams && match.teams.red && match.teams.red.rounds_won) ?? '—';
    const blue = (match.teams && match.teams.blue && match.teams.blue.rounds_won) ?? '—';
    const outcome = outcomeForPlayer(match, riotName, riotTag);
    
    // Safe check for RR
    const rrDelta = (rrOverride !== null && rrOverride !== undefined && Number.isFinite(Number(rrOverride))) 
        ? Number(rrOverride) 
        : ratingDeltaForPlayer(match, riotName, riotTag);
    
    let rrClass = 'match-rating-change--unknown';
    let rrLabel = 'RR —';
    if (rrDelta !== null) {
        if (rrDelta > 0) {
            rrClass = 'match-rating-change--gain';
            rrLabel = `RR +${rrDelta}`;
        } else if (rrDelta < 0) {
            rrClass = 'match-rating-change--loss';
            rrLabel = `RR ${rrDelta}`;
        } else {
            rrClass = 'match-rating-change--even';
            rrLabel = 'RR +0';
        }
    }
    
    let resultClass = 'match-result--upcoming';
    let resultLabel = 'Draw';
    if (outcome === 'win') {
        resultClass = 'match-result--win';
        resultLabel = 'Win';
    } else if (outcome === 'loss') {
        resultClass = 'match-result--loss';
        resultLabel = 'Loss';
    } else if (outcome === null && (red !== '—' || blue !== '—')) {
        resultLabel = '—';
    }

    // Generate the roster label
    let rosterLabelHTML = '';
    if (roster && roster.length > 0) {
        const rosterNames = roster.map(p => escapeHtml(p.name)).join(' + ');
        rosterLabelHTML = `<div class="match-roster-label">👥 ${rosterNames}</div>`;
    }

    const li = document.createElement('li');
    li.className = 'match-card';
    li.innerHTML = `
        ${makePlayerAvatar(riotName, riotTag)}
        <div class="match-main">
            ${rosterLabelHTML}
            <h3>${escapeHtml(mapName)}</h3>
            <p class="match-player">${escapeHtml(riotName)}</p>
            <p class="match-scoreline">Attackers ${escapeHtml(String(red))} – ${escapeHtml(String(blue))} Defenders</p>
            <p class="match-rating-change ${rrClass}">${escapeHtml(rrLabel)}</p>
        </div>
        <span class="match-result ${resultClass}">${escapeHtml(resultLabel)}</span>
    `;
    return li;
}

function collectPlayersFromEntries(entries) {
    const map = new Map();
    (entries || []).forEach((entry) => {
        (entry.roster || []).forEach((r) => {
            const k = playerKeyJs(r.name, r.tag);
            if (!map.has(k)) map.set(k, { name: r.name, tag: r.tag });
        });
    });
    return Array.from(map.values());
}

function matchIdFromEntry(entry) {
    const m = entry.match && entry.match.metadata;
    return m && m.matchid ? String(m.matchid) : '';
}

function matchStartMeta(entry) {
    const gs = entry.match && entry.match.metadata && entry.match.metadata.game_start;
    return Number(gs) || 0;
}

function mergeRosterDelta(existing, delta) {
    const byId = new Map();
    (existing || []).forEach((e) => {
        const id = matchIdFromEntry(e);
        if (id) byId.set(id, e);
    });
    (delta || []).forEach((e) => {
        const id = matchIdFromEntry(e);
        if (!id) return;
        if (!byId.has(id)) {
            byId.set(id, e);
            return;
        }
        const cur = byId.get(id);
        const rk = new Set((cur.roster || []).map((r) => playerKeyJs(r.name, r.tag)));
        (e.roster || []).forEach((r) => {
            const k = playerKeyJs(r.name, r.tag);
            if (!rk.has(k)) {
                if (!cur.roster) cur.roster = [];
                cur.roster.push(r);
                rk.add(k);
            }
        });
    });
    return Array.from(byId.values()).sort((a, b) => matchStartMeta(b) - matchStartMeta(a));
}

function initMatchFilters(body, matchList, statusEl, apiBase) {
    const filterBar = document.getElementById('playerFilterBar');
    const hintEl = document.getElementById('matchFilterHint');
    const loadMoreBtn = document.getElementById('matchLoadMoreBtn');
    const rows = Array.isArray(body.data) ? body.data : [];
    let configured = Array.isArray(body.players) ? body.players : [];
    if (!configured.length) {
        configured = collectPlayersFromEntries(rows);
    }

    const state = {
        allEntries: rows,
        players: configured,
        selected: new Set(configured.map((p) => playerKeyJs(p.name, p.tag))),
        warnings: body.warnings || [],
        resume: Array.isArray(body.resume) ? body.resume : [],
        hasMore: !!body.hasMore,
        apiBase,
        loadingMore: false
    };

    function competitiveEntries() {
        return state.allEntries.filter((e) => isCompetitiveMode(e.match && e.match.metadata && e.match.metadata.mode));
    }

    function resumeHasMore(s) {
        return (
            s.hasMore &&
            Array.isArray(s.resume) &&
            s.resume.some((r) => r && !r.exhausted)
        );
    }

    function applyFiltersAndRender() {
        const comp = competitiveEntries();
        const filtered = comp.filter((entry) => {
            const roster = entry.roster || [];
            return roster.some((r) => state.selected.has(playerKeyJs(r.name, r.tag)));
        });

        matchList.replaceChildren();
        let msg = '';
        if (state.allEntries.length === 0) {
            msg = 'No matches returned from the API.';
            if (state.warnings.length) msg += ' ' + state.warnings.join(' ');
        } else if (comp.length === 0) {
            msg =
                `Loaded ${state.allEntries.length} game(s), none tagged as Competitive in the API response. (If you play ranked, confirm the upstream returns mode "Competitive".)`;
            if (state.warnings.length) msg += ' ' + state.warnings.join(' ');
        } else if (state.selected.size === 0) {
            msg = `There are ${comp.length} competitive game(s); turn at least one player on to see matches.`;
        } else {
            msg = ``;
            if (state.warnings.length) msg += ' Notes: ' + state.warnings.join(' ');
            if (resumeHasMore(state)) msg += ' Use “Load more” for older competitive games.';
        }
        statusEl.textContent = msg;

        filtered.forEach((entry) => {
            matchList.appendChild(renderRosterMatchRow(entry, state.selected));
        });
    }

    function renderFilterButtons() {
        if (!filterBar) return;
        filterBar.hidden = state.players.length === 0;
        if (hintEl) hintEl.hidden = state.players.length === 0;
        filterBar.replaceChildren();

        const label = document.createElement('span');
        label.className = 'player-filter-label';
        label.textContent = 'Players';
        filterBar.appendChild(label);

        state.players.forEach((p) => {
            const key = playerKeyJs(p.name, p.tag);
            const btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'player-filter-btn';
            btn.textContent = `${p.name}#${p.tag}`;
            btn.dataset.playerKey = key;
            btn.setAttribute('aria-pressed', state.selected.has(key) ? 'true' : 'false');

            function syncVisual() {
                const on = state.selected.has(key);
                btn.classList.toggle('player-filter-btn--on', on);
                btn.classList.toggle('player-filter-btn--off', !on);
                btn.setAttribute('aria-pressed', on ? 'true' : 'false');
            }

            syncVisual();
            btn.addEventListener('click', () => {
                if (state.selected.has(key)) {
                    state.selected.delete(key);
                } else {
                    state.selected.add(key);
                }
                syncVisual();
                applyFiltersAndRender();
            });
            filterBar.appendChild(btn);
        });
    }

    function syncLoadMoreButton() {
        if (!loadMoreBtn) return;
        const show = state.players.length > 0;
        loadMoreBtn.hidden = !show;
        const more = resumeHasMore(state);
        loadMoreBtn.disabled = state.loadingMore || !more;
        loadMoreBtn.textContent = state.loadingMore ? 'Loading…' : more ? 'Load more matches' : 'No more matches';
    }

    async function onLoadMore() {
        if (!resumeHasMore(state) || state.loadingMore) return;
        state.loadingMore = true;
        syncLoadMoreButton();
        const knownMatchIds = state.allEntries.map(matchIdFromEntry).filter(Boolean);
        try {
            const res = await fetch(`${state.apiBase}/api/matches/roster/more`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    resume: state.resume,
                    knownMatchIds,
                    pagesPerRequest: 3
                })
            });
            const text = await res.text();
            let data;
            try {
                data = JSON.parse(text);
            } catch {
                statusEl.classList.add('match-fetch-status--error');
                statusEl.textContent = 'Load more: response was not JSON.';
                state.loadingMore = false;
                syncLoadMoreButton();
                return;
            }
            if (!res.ok) {
                statusEl.classList.add('match-fetch-status--error');
                const msg =
                    (data && (data.errors && data.errors.message)) ||
                    (data && data.message) ||
                    (data && data.error) ||
                    text.slice(0, 200);
                statusEl.textContent = `Load more failed (${res.status}): ${msg}`;
                state.loadingMore = false;
                syncLoadMoreButton();
                return;
            }
            statusEl.classList.remove('match-fetch-status--error');
            state.resume = Array.isArray(data.resume) ? data.resume : [];
            state.hasMore = !!data.hasMore;
            if (Array.isArray(data.warnings) && data.warnings.length) {
                state.warnings = state.warnings.concat(data.warnings);
            }
            state.allEntries = mergeRosterDelta(state.allEntries, data.data || []);
            state.loadingMore = false;
            applyFiltersAndRender();
            syncLoadMoreButton();
        } catch {
            statusEl.classList.add('match-fetch-status--error');
            statusEl.textContent = 'Load more: network error.';
            state.loadingMore = false;
            syncLoadMoreButton();
        }
    }

    renderFilterButtons();
    applyFiltersAndRender();
    if (loadMoreBtn) {
        loadMoreBtn.addEventListener('click', onLoadMore);
    }
    syncLoadMoreButton();
}

function toggleCard(card) { card.classList.toggle('expanded'); }

function getRosterPlayerStats(match, playerName) {
    let players = match && match.players;
    if (players && !Array.isArray(players) && players.all_players) players = players.all_players;
    if (!Array.isArray(players)) return null;
    const p = players.find(pl => pl && pl.name && pl.name.toLowerCase() === String(playerName).toLowerCase());
    if (!p || !p.stats) return null;
    const rounds = (match.metadata && match.metadata.rounds_played > 0) ? match.metadata.rounds_played : 1;
    const kills = p.stats.kills || 0;
    const deaths = p.stats.deaths || 0;
    const assists = p.stats.assists || 0;
    const acs = Math.round((p.stats.score || 0) / rounds);
    const totalShots = (p.stats.headshots || 0) + (p.stats.bodyshots || 0) + (p.stats.legshots || 0);
    const hsPct = totalShots > 0 ? Math.round((p.stats.headshots / totalShots) * 100) : 0;
    const kd = deaths === 0 ? kills.toFixed(2) : (kills / deaths).toFixed(2);
    const agent = p.character || '—';
    return { kills, deaths, assists, acs, hsPct, kd, agent };
}

// --- VALORANT ASSET HELPERS ---
const VAL_IMG_BASE = 'images/valorant';

// Henrik currenttier int → patched label (fallback when currenttier_patched absent)
function rankLabelFromTier(tierId) {
    if (typeof tierId !== 'number' || tierId < 3) return '';
    if (tierId >= 27) return 'Radiant';
    const tiers = ['Iron', 'Bronze', 'Silver', 'Gold', 'Platinum', 'Diamond', 'Ascendant', 'Immortal'];
    const idx = Math.floor((tierId - 3) / 3); // 3-5→0 … 24-26→7
    const div = ((tierId - 3) % 3) + 1;        // 1..3
    return tiers[idx] ? `${tiers[idx]} ${div}` : '';
}

// Reads a player's competitive rank from the Henrik match payload and returns a
// display label, tier color, and rank badge image path. Handles both the flat
// (currenttier / currenttier_patched) and nested (tier.name / tier.id) shapes.
function valorantRankInfo(p) {
    if (!p) return { label: '—', color: 'rgba(200,216,240,.35)', img: '' };

    let tierId = (typeof p.currenttier === 'number' ? p.currenttier : null);
    if (tierId === null && p.tier && typeof p.tier.id === 'number') tierId = p.tier.id;

    let label = p.currenttier_patched
        || (p.tier && (p.tier.name || p.tier.patched))
        || rankLabelFromTier(tierId)
        || '';
    label = String(label || '').trim();

    if (!label || label.toLowerCase() === 'unranked' || label.toLowerCase() === 'unrated') {
        return { label: 'Unranked', color: 'rgba(200,216,240,.35)', img: `${VAL_IMG_BASE}/ranks/Unranked.webp` };
    }

    // Tier color bands (Henrik currenttier int: 3-5 Iron … 27 Radiant)
    const COLORS = {
        iron: '#8a8f99', bronze: '#cd7f32', silver: '#c0cdd6', gold: '#ffd94a',
        platinum: '#34d6c8', diamond: '#b982ff', ascendant: '#2bc97e',
        immortal: '#ff4655', radiant: '#ffea82',
    };
    let key = '';
    if (tierId !== null) {
        if (tierId >= 27) key = 'radiant';
        else if (tierId >= 24) key = 'immortal';
        else if (tierId >= 21) key = 'ascendant';
        else if (tierId >= 18) key = 'diamond';
        else if (tierId >= 15) key = 'platinum';
        else if (tierId >= 12) key = 'gold';
        else if (tierId >= 9) key = 'silver';
        else if (tierId >= 6) key = 'bronze';
        else key = 'iron';
    } else {
        key = label.toLowerCase().split(' ')[0];
    }

    // Filename: "Gold 2" → Gold_2_Rank.webp, "Radiant" → Radiant_Rank.webp
    const img = `${VAL_IMG_BASE}/ranks/${label.replace(/\s+/g, '_')}_Rank.webp`;
    return { label, color: COLORS[key] || '#c8d8f0', img };
}

// Agent splash/icon path. Strips non-alphanumerics so "KAY/O" → KAYO.webp.
function agentImg(character) {
    const c = String(character || '').trim();
    if (!c || c === '—') return '';
    return `${VAL_IMG_BASE}/agents/${c.replace(/[^A-Za-z0-9]/g, '')}.webp`;
}

// Map banner path. "Ascent" → maps/Ascent.webp (only the 12 shipped maps exist).
function mapImg(mapName) {
    const m = String(mapName || '').trim();
    if (!m || m.toLowerCase() === 'unknown map') return '';
    return `${VAL_IMG_BASE}/maps/${m.replace(/[^A-Za-z0-9]/g, '')}.webp`;
}

// --- MATCH UI RENDERER (FINAL RR FIX) ---
function renderRosterMatchRow(entry, selectedSet) {
    const matchData = entry.match || {};
    const meta = matchData.metadata || {};
    const roster = entry.roster || [];

    const mapName = typeof meta.map === 'object' ? meta.map.name : (meta.map || 'Unknown Map');
    const mode = meta.mode || 'Unrated';

    // 1. EXACT PLAYER TARGETING
    let targetFred = roster[0]; 
    if (selectedSet && roster.length > 0) {
        const filteredRoster = roster.filter(r => selectedSet.has(playerKeyJs(r.name, r.tag)));
        if (filteredRoster.length > 0) {
            targetFred = filteredRoster[0];
        }
    }

    let matchClass = "draw";
    let rrDelta = null;

    if (targetFred) {
        let outcome = outcomeForPlayer(matchData, targetFred.name, targetFred.tag);

        // 2. THE PLOT TWIST RR EXTRACTION!
        if (entry.rrByPlayer) {
            const searchKey = `${targetFred.name}#${targetFred.tag}`.toLowerCase();
            if (entry.rrByPlayer[searchKey] !== undefined) {
                rrDelta = entry.rrByPlayer[searchKey];
            }
        }
        
        if (rrDelta === null) {
            rrDelta = ratingDeltaForPlayer(matchData, targetFred.name, targetFred.tag);
        }

        // 3. THE "REDACTED NAME" BYPASS! 
        if (outcome === null && rrDelta !== null) {
            if (rrDelta > 0) outcome = 'win';
            else if (rrDelta < 0) outcome = 'loss';
        }

        if (outcome === 'win') matchClass = 'win';
        else if (outcome === 'loss') matchClass = 'loss';
    }

    // 4. SCORE SORTING
    let redRounds = 0, blueRounds = 0;

    if (Array.isArray(matchData.teams)) {
        const redTeam = matchData.teams.find(t => t.team_id === 'Red');
        const blueTeam = matchData.teams.find(t => t.team_id === 'Blue');
        if (redTeam && blueTeam) { redRounds = redTeam.rounds_won; blueRounds = blueTeam.rounds_won; }
    } else if (matchData.teams && matchData.teams.red && matchData.teams.blue) {
        redRounds = matchData.teams.red.rounds_won; blueRounds = matchData.teams.blue.rounds_won;
    }

    const hasScore = redRounds !== 0 || blueRounds !== 0;
    const higher = Math.max(redRounds, blueRounds);
    const lower = Math.min(redRounds, blueRounds);

    let scoreHTML;
    if (!hasScore) {
        scoreHTML = `<div class="score-main"><span class="s-dash">—</span></div>`;
    } else if (matchClass === 'win') {
        scoreHTML = `<div class="score-main"><span class="s-win">${higher}</span><span class="s-dash">—</span><span class="s-loss">${lower}</span></div>`;
    } else if (matchClass === 'loss') {
        scoreHTML = `<div class="score-main"><span class="s-loss">${lower}</span><span class="s-dash">—</span><span class="s-win">${higher}</span></div>`;
    } else {
        scoreHTML = `<div class="score-main"><span>${redRounds}</span><span class="s-dash">—</span><span>${blueRounds}</span></div>`;
    }

    const scoreMetaLabel = matchClass === 'win' ? 'Victoire' : matchClass === 'loss' ? 'Défaite' : '—';
    const scoreMetaClass = matchClass === 'win' ? 'win' : matchClass === 'loss' ? 'loss' : '';

    // 4.5 Get stats
    const myStats = getPlayerStats(matchData, targetFred.puuid);

    let statsColHTML = '';
    if (myStats) {
        statsColHTML = `<div class="team-name">${escapeHtml(String(myStats.kda))}</div><div class="team-tag">${escapeHtml(String(myStats.acs))} ACS</div>`;
    }

    // 5. RR display
    let rrLabel = '—';
    if (rrDelta !== null && isCompetitiveMode(mode)) {
        const sign = rrDelta > 0 ? '+' : '';
        rrLabel = `${sign}${rrDelta} RR`;
    } else if (isCompetitiveMode(mode)) {
        rrLabel = 'RR —';
    }

    // 6. Player / roster display
    const playerInitials = targetFred ? String(targetFred.name || '?').slice(0, 2).toUpperCase() : '??';
    const playerName = targetFred ? escapeHtml(targetFred.name) : '—';
    const teammates = roster
        .filter(r => !targetFred || r.name !== targetFred.name)
        .map(r => escapeHtml(r.name)).join(', ');
    const teammatesHTML = teammates ? `<div class="team-tag">👥 ${teammates}</div>` : '';

    // 7. Scoreboard rows for all players in match, grouped by team
    let allPlayers = matchData.players;
    if (allPlayers && !Array.isArray(allPlayers) && allPlayers.all_players) allPlayers = allPlayers.all_players;
    if (!Array.isArray(allPlayers)) allPlayers = [];

    const rosterNames = new Set(roster.map(r => r.name.toLowerCase()));
    const rounds = (meta.rounds_played > 0 ? meta.rounds_played : null) || allPlayers.reduce((max, p) => {
        const s = p.stats || {};
        return Math.max(max, (s.kills || 0) + (s.deaths || 0) > 0 ? (matchData.metadata?.rounds_played || 1) : 1);
    }, 1);
    const roundCount = (matchData.metadata?.rounds_played > 0 ? matchData.metadata.rounds_played : 1);

    // First-blood / first-death per player (puuid → count). The initial roster feed
    // strips the kill array but ships server-computed first_bloods / first_deaths on
    // each player; the load-more feed ships raw matches with the kill array instead.
    // Prefer the precomputed fields, otherwise derive from the kill feed.
    const fbByPuuid = {};
    const fdByPuuid = {};
    let hasFBFD = false;
    for (const p of allPlayers) {
        if (typeof p.first_bloods === 'number' || typeof p.first_deaths === 'number') {
            hasFBFD = true;
            if (p.puuid) {
                fbByPuuid[p.puuid] = p.first_bloods || 0;
                fdByPuuid[p.puuid] = p.first_deaths || 0;
            }
        }
    }
    if (!hasFBFD && Array.isArray(matchData.kills) && matchData.kills.length) {
        const roundFirst = {};
        for (const k of matchData.kills) {
            const prev = roundFirst[k.round];
            if (!prev || (k.kill_time_in_round || 0) < (prev.kill_time_in_round || 0)) {
                roundFirst[k.round] = k;
            }
        }
        for (const k of Object.values(roundFirst)) {
            if (k.killer_puuid) fbByPuuid[k.killer_puuid] = (fbByPuuid[k.killer_puuid] || 0) + 1;
            if (k.victim_puuid) fdByPuuid[k.victim_puuid] = (fdByPuuid[k.victim_puuid] || 0) + 1;
        }
        hasFBFD = true;
    }
    // Only show the ADR column when the data actually backs it.
    const hasADR = allPlayers.some(p => typeof p.damage_made === 'number' && p.damage_made > 0);

    const headCells = ['Joueur', 'Rang', 'ACS'];
    if (hasADR) headCells.push('ADR');
    headCells.push('K', 'D', 'A', 'K/D');
    if (hasFBFD) headCells.push('FB', 'FD');
    headCells.push('HS%');
    const colCount = headCells.length;
    const headerHTML = headCells.map(h => `<th>${h}</th>`).join('');

    function buildPlayerRow(p) {
        const s = p.stats || {};
        const kills = s.kills || 0;
        const deaths = s.deaths || 0;
        const assists = s.assists || 0;
        const acs = Math.round((s.score || 0) / roundCount);
        const totalShots = (s.headshots || 0) + (s.bodyshots || 0) + (s.legshots || 0);
        const hsPct = totalShots > 0 ? Math.round((s.headshots / totalShots) * 100) : 0;
        const kd = deaths === 0 ? kills.toFixed(2) : (kills / deaths).toFixed(2);
        const kdClass = parseFloat(kd) >= 1 ? 'stat-kd-pos' : 'stat-kd-neg';
        const agent = p.character || '—';
        const name = p.name || p.gameName || '—';
        const initials = String(name).slice(0, 2).toUpperCase();
        const isRoster = rosterNames.has(String(name).toLowerCase());
        const rank = valorantRankInfo(p);
        const aImg = agentImg(agent);
        const avatar = aImg
            ? `<img class="player-avatar agent-avatar" src="${aImg}" alt="${escapeHtml(agent)}" title="${escapeHtml(agent)}" loading="lazy">`
            : `<div class="player-avatar">${escapeHtml(initials)}</div>`;
        const rankCell = rank.img
            ? `<img class="rank-icon" src="${rank.img}" alt="${escapeHtml(rank.label)}" title="${escapeHtml(rank.label)}" loading="lazy">`
            : `<span class="rank-badge" style="color:${rank.color}" title="${escapeHtml(rank.label)}">—</span>`;
        const adrCell = hasADR
            ? `<td class="stat-adr">${roundCount > 0 ? Math.round((p.damage_made || 0) / roundCount) : 0}</td>`
            : '';
        const fbFdCells = hasFBFD
            ? `<td class="stat-fb">${fbByPuuid[p.puuid] || 0}</td><td class="stat-fd">${fdByPuuid[p.puuid] || 0}</td>`
            : '';
        return `<tr${isRoster ? ' class="roster-highlight"' : ''}>
            <td><div class="player-name">${avatar}${escapeHtml(name)}</div></td>
            <td>${rankCell}</td>
            <td class="stat-acs">${acs}</td>
            ${adrCell}
            <td>${kills}</td>
            <td>${deaths}</td>
            <td>${assists}</td>
            <td class="${kdClass}">${kd}</td>
            ${fbFdCells}
            <td>${hsPct}%</td>
        </tr>`;
    }

    // Determine FRED's team from the targetFred player entry
    let fredTeam = null;
    if (targetFred) {
        const fredEntry = allPlayers.find(p => (p.name || p.gameName || '').toLowerCase() === targetFred.name.toLowerCase());
        if (fredEntry) fredTeam = fredEntry.team;
    }

    const fredSide = allPlayers.filter(p => p.team === fredTeam).sort((a, b) => Math.round((b.stats?.score || 0) / roundCount) - Math.round((a.stats?.score || 0) / roundCount));
    const enemySide = allPlayers.filter(p => p.team !== fredTeam).sort((a, b) => Math.round((b.stats?.score || 0) / roundCount) - Math.round((a.stats?.score || 0) / roundCount));

    const fredLabel = `<tr><td colspan="${colCount}" style="padding:.4rem .8rem;font-family:var(--font-hd);font-size:.6rem;letter-spacing:.1em;text-transform:uppercase;color:var(--cyan);background:rgba(0,212,255,.06)">Notre équipe</td></tr>`;
    const enemyLabel = `<tr><td colspan="${colCount}" style="padding:.4rem .8rem;font-family:var(--font-hd);font-size:.6rem;letter-spacing:.1em;text-transform:uppercase;color:rgba(200,216,240,.4);background:rgba(255,70,85,.04)">Adversaires</td></tr>`;

    const scoreboardRows = allPlayers.length > 0
        ? fredLabel + fredSide.map(buildPlayerRow).join('') + enemyLabel + enemySide.map(buildPlayerRow).join('')
        : '';

    // 8. ASSEMBLE using premier.css classes
    const li = document.createElement('div');
    li.className = `match-card${matchClass !== 'draw' ? ' ' + matchClass : ''}`;
    li.setAttribute('onclick', 'toggleCard(this)');
    const mImg = mapImg(mapName);
    const summaryStyle = mImg
        ? ` style="background-image:linear-gradient(90deg, rgba(13,20,40,.97) 0%, rgba(13,20,40,.82) 45%, rgba(13,20,40,.92) 100%), url('${mImg}')"`
        : '';
    li.innerHTML = `
        <div class="match-summary${mImg ? ' has-map-banner' : ''}"${summaryStyle}>
            <div class="match-date">
                ${escapeHtml(mapName)}
                <span>${escapeHtml(mode)}</span>
            </div>
            <div class="team-info">
                <div class="team-logo">${escapeHtml(playerInitials)}</div>
                <div>
                    <div class="team-name">${playerName}</div>
                    ${teammatesHTML}
                </div>
            </div>
            <div class="score-block">
                ${scoreHTML}
                <div class="score-meta ${scoreMetaClass}">${scoreMetaLabel}</div>
            </div>
            <div class="team-info">
                <div>${statsColHTML}</div>
            </div>
            <div class="match-format">
                <span class="format-tag${rrDelta !== null && rrDelta > 0 ? ' rr-gain' : rrDelta !== null && rrDelta < 0 ? ' rr-loss' : ''}">${escapeHtml(rrLabel)}</span>
            </div>
        </div>
        <div class="match-details">
            <div class="stats-panel active">
                <table class="player-table">
                    <thead><tr>${headerHTML}</tr></thead>
                    <tbody>${scoreboardRows || `<tr><td colspan="${colCount}" style="text-align:center;color:rgba(200,216,240,.3);padding:1rem">Aucune donnée disponible</td></tr>`}</tbody>
                </table>
            </div>
        </div>
    `;

    return li;
}

async function loadMatchHistory(matchList, statusEl) {
    const meta = document.querySelector('meta[name="fred-api-base"]');
    const apiBase = getApiBase();

    statusEl.classList.remove('match-fetch-status--error');

    if (!apiBase) {
        statusEl.classList.add('match-fetch-status--error');
        statusEl.textContent =
            'Add <meta name="fred-api-base" content="http://fredericfan.club:8080"> to matches.html (your Go server URL).';
        return;
    }

    // NEW: Append the current exact millisecond to force a fresh pull
    const url = `${apiBase}/api/matches/roster?_t=${Date.now()}`;
    statusEl.textContent = 'Loading roster matches…';

    let res;
    try {
        // NEW: Tell the browser explicitly to bypass its local memory
        res = await fetch(url, { cache: 'no-store' });
    } catch (e) {
        statusEl.classList.add('match-fetch-status--error');
        statusEl.textContent =
            'Could not reach the API. Start the Go server (see server/) and keep fred-api-base pointed at it.';
        return;
    }

    const text = await res.text();
    let body;
    try {
        body = JSON.parse(text);
    } catch {
        statusEl.classList.add('match-fetch-status--error');
        statusEl.textContent = res.ok ? 'Unexpected response (not JSON).' : `Error ${res.status}: ${text.slice(0, 200)}`;
        return;
    }

    if (!res.ok) {
        statusEl.classList.add('match-fetch-status--error');
        const msg =
            (body && (body.errors && body.errors.message)) ||
            (body && body.message) ||
            (body && body.error) ||
            text.slice(0, 200);
        statusEl.textContent = `Request failed (${res.status}): ${msg}`;
        return;
    }

    if (!Array.isArray(body.data)) {
        statusEl.classList.add('match-fetch-status--error');
        statusEl.textContent = 'Invalid response: missing data array.';
        matchList.replaceChildren();
        return;
    }

    initMatchFilters(body, matchList, statusEl, apiBase);
}

// --- PREDICTION MARKET LOGIC ---

async function loadBettingMarket() {
    const widget = document.getElementById('bettingWidget');
    if (!widget) return; 

    const authCheck = document.getElementById('authContainer');
    if (!authCheck || authCheck.innerHTML.includes('Login with Discord')) {
        widget.style.display = 'none';
        return;
    }
    
    widget.style.display = 'block';

    const apiBase = getApiBase();

    // === 1. PERSISTENT WEBSOCKET CONNECTION ===
    if (!window.bettingSocket || window.bettingSocket.readyState === WebSocket.CLOSED) {
        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const cleanHost = apiBase.replace(/^https?:\/\//, ''); 
        const wsUrl = `${wsProtocol}//${cleanHost}/api/ws/betting`;

        window.bettingSocket = new WebSocket(wsUrl);

        window.bettingSocket.onclose = function() {
            setTimeout(() => { if (document.getElementById('bettingWidget')) loadBettingMarket(); }, 3000);
        };

        window.bettingSocket.onmessage = function(event) {
            const msg = JSON.parse(event.data);

            if (msg.type === "new_bet") {
                const bet = msg.payload;
                const betsList = document.getElementById('liveBetsList');
                if (!betsList) return;

                if (betsList.innerHTML.includes("No bets placed yet")) betsList.innerHTML = "";

                const isOver = bet.choice === 'over';
                const choiceColor = isOver ? '#00ff64' : '#ff4655';
                const amountFormatted = Math.round(bet.amount * 10) / 10;

                let displayChoice = bet.choice;
                const typeName = document.getElementById('propTypeName')?.textContent || '';
                if (typeName === 'MATCH RESULT') displayChoice = isOver ? 'WIN' : 'LOSS';

                const newBetDiv = document.createElement('div');
                newBetDiv.style.cssText = `display: flex; align-items: center; justify-content: space-between; background: rgba(0,0,0,0.4); padding: 12px 16px; border-radius: 8px; border: 1px solid ${choiceColor}; margin-bottom: 0px; height: 0; opacity: 0; overflow: hidden; transition: all 0.4s cubic-bezier(0.175, 0.885, 0.32, 1.275);`;
                
                newBetDiv.innerHTML = `
                    <div style="display: flex; align-items: center; gap: 12px;">
                        <img src="${escapeHtml(bet.avatar)}" style="width: 32px; height: 32px; border-radius: 50%; border: 1px solid ${choiceColor};">
                        <span style="font-weight: 700; color: white; font-family: 'Rajdhani', sans-serif; font-size: 1.1rem;">${escapeHtml(bet.username)}</span>
                    </div>
                    <div style="font-family: 'Rajdhani', sans-serif; font-size: 1.1rem;">
                        <span style="color: rgba(255,255,255,0.3); font-size: 0.85rem; margin-right: 8px;">#${bet.id}</span>
                        <span style="color: ${choiceColor}; font-weight: bold; text-transform: uppercase; letter-spacing: 1px; margin-right: 12px;">${escapeHtml(displayChoice)}</span>
                        <span style="color: #00d4ff; font-weight: bold; font-family: 'Orbitron', sans-serif; font-size: 1rem;">${amountFormatted} FT</span>
                    </div>
                `;

                betsList.prepend(newBetDiv);
                newBetDiv.offsetHeight; // Force browser reflow to trigger animation

                requestAnimationFrame(() => {
                    newBetDiv.style.height = "58px"; 
                    newBetDiv.style.opacity = "1";
                    newBetDiv.style.marginBottom = "10px";
                    newBetDiv.style.boxShadow = `0 0 20px ${choiceColor}40`; 
                    setTimeout(() => {
                        newBetDiv.style.boxShadow = "none";
                        newBetDiv.style.borderColor = "rgba(255,255,255,0.05)";
                    }, 1000);
                });
            }
            else if (msg.type === "market_locked") {
                const badge = document.getElementById('marketStatusBadge');
                if (badge) {
                    badge.textContent = "MARKET LOCKED";
                    badge.className = "market-badge status-closed";
                    badge.style.color = "#ffaa00"; 
                    badge.style.borderColor = "rgba(255, 170, 0, 0.3)";
                    badge.style.background = "rgba(255, 170, 0, 0.1)";
                }
                const btnOver = document.getElementById('btnOver');
                const btnUnder = document.getElementById('btnUnder');
                const msgEl = document.getElementById('betMessage');
                if (btnOver) { btnOver.disabled = true; btnOver.style.opacity = "0.5"; }
                if (btnUnder) { btnUnder.disabled = true; btnUnder.style.opacity = "0.5"; }
                if (msgEl) { msgEl.textContent = "Bets are locked! Good luck!"; msgEl.style.color = "#ffaa00"; }
            }
            else if (["market_resolved", "market_cancelled", "market_published"].includes(msg.type)) {
                loadBettingMarket();
                if (typeof window.loadUserProfile === 'function') window.loadUserProfile(); 
            }
        };
    }
    // ==============================================
    
    try {
        // NEW: Fetching the single event market!
        const res = await fetch(`${apiBase}/api/betting/market`, { credentials: 'include', cache: 'no-store' });
        if (res.ok) {
            const data = await res.json();
            
            const statusBadge = document.getElementById('marketStatusBadge');
            const publicArea = document.getElementById('publicMarketArea');
            const closedArea = document.getElementById('marketClosedArea');
            const btnOver = document.getElementById('btnOver');
            const btnUnder = document.getElementById('btnUnder');
            const msgEl = document.getElementById('betMessage');
            
            // 1. If NO market exists
            if (data.exists === false) {
                publicArea.style.display = "none";
                closedArea.style.display = "block";
                statusBadge.textContent = "MARKET CLOSED";
                statusBadge.className = "market-badge status-closed";
                statusBadge.style.cssText = ""; // Reset custom styles
                return;
            }

            // 2. A market exists (Open or Locked)
            publicArea.style.display = "block";
            closedArea.style.display = "none";

            document.getElementById('propPlayerName').textContent = data.player;
            document.getElementById('propTypeName').textContent = (data.prop_type === 'adr') ? "ADR (per round)" : "Total " + data.prop_type;
            document.getElementById('propLineValue').textContent = Number(data.line.toFixed(2));
            document.getElementById('oddsOver').textContent = data.over_multiplier.toFixed(2) + "x";
            document.getElementById('oddsUnder').textContent = data.under_multiplier.toFixed(2) + "x";

            // --- RENDER LIVE BETS FEED ---
            const betsList = document.getElementById('liveBetsList');
            if (betsList) {
                const activeBets = data.active_bets || []; // <--- FIX: Fallback to empty array!
                
                if (activeBets.length === 0) {
                    betsList.innerHTML = `<p style="text-align: center; color: rgba(255,255,255,0.4); font-size: 0.9rem;">No bets placed yet. Be the first!</p>`;
                } else {
                    betsList.innerHTML = activeBets.map(bet => {
                        const isOver = bet.choice === 'over';
                        const choiceColor = isOver ? '#00ff64' : '#ff4655';
                        const amountFormatted = Math.round(bet.amount * 10) / 10;

                        let displayChoice = bet.choice;
                        if (data.prop_type === 'match_result') {
                            displayChoice = isOver ? 'WIN' : 'LOSS';
                        }
                        
                        return `
                        <div style="display: flex; align-items: center; justify-content: space-between; background: rgba(0,0,0,0.4); padding: 12px 16px; border-radius: 8px; border: 1px solid rgba(255,255,255,0.05); transition: transform 0.2s;">
                            <div style="display: flex; align-items: center; gap: 12px;">
                                <img src="${escapeHtml(bet.avatar)}" style="width: 32px; height: 32px; border-radius: 50%; border: 1px solid ${choiceColor};">
                                <span style="font-weight: 700; color: white; font-family: 'Rajdhani', sans-serif; font-size: 1.1rem;">${escapeHtml(bet.username)}</span>
                            </div>
                            <div style="font-family: 'Rajdhani', sans-serif; font-size: 1.1rem;">
                                <span style="color: rgba(255,255,255,0.3); font-size: 0.85rem; margin-right: 8px;">#${bet.id}</span>
                                <span style="color: ${choiceColor}; font-weight: bold; text-transform: uppercase; letter-spacing: 1px; margin-right: 12px;">${escapeHtml(bet.choice)}</span>
                                <span style="color: #00d4ff; font-weight: bold; font-family: 'Orbitron', sans-serif; font-size: 1rem;">${amountFormatted} FT</span>
                            </div>
                        </div>
                        `;
                    }).join('');
                }
            }

            // If it is OPEN
             // If it is OPEN
            if (data.is_open) {
                statusBadge.textContent = "MARKET OPEN";
                statusBadge.className = "market-badge status-open";
                statusBadge.style.cssText = ""; 
                
                // NEW: Smart Labels!
                if (data.prop_type === 'match_result') {
                    document.getElementById('propTypeName').textContent = "MATCH RESULT";
                    document.getElementById('propLineValue').parentElement.style.display = 'none'; // Hide the line
                    btnOver.innerHTML = `FRED WIN (<span id="oddsOver">${data.over_multiplier.toFixed(2)}x</span>)`;
                    btnUnder.innerHTML = `FRED LOSS (<span id="oddsUnder">${data.under_multiplier.toFixed(2)}x</span>)`;
                } else if (data.prop_type === 'kd_ratio') {
                    document.getElementById('propTypeName').textContent = "K/D RATIO"; // Prettify KD!
                    document.getElementById('propLineValue').parentElement.style.display = 'block';
                    btnOver.innerHTML = `OVER (<span id="oddsOver">${data.over_multiplier.toFixed(2)}x</span>)`;
                    btnUnder.innerHTML = `UNDER (<span id="oddsUnder">${data.under_multiplier.toFixed(2)}x</span>)`;
                } else if (data.prop_type === 'adr') {
                    document.getElementById('propTypeName').textContent = "ADR (per round)"; // Prettify ADR!
                    document.getElementById('propLineValue').parentElement.style.display = 'block';
                    btnOver.innerHTML = `OVER (<span id="oddsOver">${data.over_multiplier.toFixed(2)}x</span>)`;
                    btnUnder.innerHTML = `UNDER (<span id="oddsUnder">${data.under_multiplier.toFixed(2)}x</span>)`;
                } else {
                    document.getElementById('propTypeName').textContent = "Total " + data.prop_type;
                    document.getElementById('propLineValue').parentElement.style.display = 'block'; 
                    btnOver.innerHTML = `OVER (<span id="oddsOver">${data.over_multiplier.toFixed(2)}x</span>)`;
                    btnUnder.innerHTML = `UNDER (<span id="oddsUnder">${data.under_multiplier.toFixed(2)}x</span>)`;
                }
                
                btnOver.disabled = false;
                btnUnder.disabled = false;
                btnOver.style.opacity = "1";
                btnUnder.style.opacity = "1";
                msgEl.textContent = "";
            }
            // If it is LOCKED
            else {
                statusBadge.textContent = "MARKET LOCKED";
                statusBadge.className = "market-badge status-closed";
                // Add a cool orange "Locked" style
                statusBadge.style.color = "#ffaa00"; 
                statusBadge.style.borderColor = "rgba(255, 170, 0, 0.3)";
                statusBadge.style.background = "rgba(255, 170, 0, 0.1)";

                btnOver.disabled = true;
                btnUnder.disabled = true;
                btnOver.style.opacity = "0.5";
                btnUnder.style.opacity = "0.5";
                msgEl.textContent = "Bets are locked! Good luck!";
                msgEl.style.color = "#ffaa00";
            }
        }
    } catch (err) {
        console.error("Failed to load betting market", err);
    }

    // Connect to the WebSocket
    if (!window.bettingSocket || window.bettingSocket.readyState === WebSocket.CLOSED) {
        const apiBase = getApiBase(); // e.g., "http://localhost:8080"
        const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const cleanHost = apiBase.replace(/^https?:\/\//, ''); 
        const wsUrl = `${wsProtocol}//${cleanHost}/api/ws/betting`;

        window.bettingSocket = new WebSocket(wsUrl);

        window.bettingSocket.onmessage = function(event) {
            const msg = JSON.parse(event.data);

            // EVENT 1: A new bet drops in!
            if (msg.type === "new_bet") {
                const bet = msg.payload;
                const betsList = document.getElementById('liveBetsList');
                if (!betsList) return;

                // Create the new bet element
                const newBetDiv = document.createElement('div');
                const choiceColor = bet.choice === 'over' ? '#00ff64' : '#ff4655';
                
                // Start it with 0 height and 0 opacity for a slick slide-down animation
                newBetDiv.style.cssText = `display: flex; align-items: center; justify-content: space-between; background: rgba(0,0,0,0.4); padding: 12px 16px; border-radius: 8px; border: 1px solid ${choiceColor}; margin-bottom: 0px; height: 0; opacity: 0; overflow: hidden; transition: all 0.4s ease;`;
                
                newBetDiv.innerHTML = `
                    <div style="display: flex; align-items: center; gap: 12px;">
                        <img src="${bet.avatar}" style="width: 32px; height: 32px; border-radius: 50%;">
                        <span style="color: white; font-weight: bold;">${bet.username}</span>
                    </div>
                    <div>
                        <span style="color: ${choiceColor}; font-weight: bold; margin-right: 12px;">${bet.choice.toUpperCase()}</span>
                        <span style="color: #00d4ff; font-weight: bold;">${bet.amount} FT</span>
                    </div>
                `;

                // Add it to the top of the list
                betsList.prepend(newBetDiv);

                // Force the browser to render the 0-height state, then animate it open
                newBetDiv.offsetHeight; 
                requestAnimationFrame(() => {
                    newBetDiv.style.height = "58px"; 
                    newBetDiv.style.opacity = "1";
                    newBetDiv.style.marginBottom = "10px";
                });
            }
            
            // EVENT 2: The market was resolved (Win/Loss)!
            else if (msg.type === "market_resolved") {
                // The cleanest way to handle a massive state change is to 
                // command the browser to re-fetch the market and their user wallet!
                loadBettingMarket();
                if (typeof window.loadUserProfile === 'function') {
                    window.loadUserProfile(); 
                }
            }
        };

        // Auto-reconnect if the server restarts
        window.bettingSocket.onclose = function() {
            setTimeout(() => {
                if (document.getElementById('bettingWidget')) loadBettingMarket(); 
            }, 3000);
        };
    }
}

window.placePropBet = async function(choice) {
    const msgEl = document.getElementById('betMessage');
    const amountInput = document.getElementById('betAmountInput');
    const amount = parseInt(amountInput.value);

    if (isNaN(amount) || amount <= 0) {
        msgEl.style.color = "#ff4655";
        msgEl.textContent = "Please enter a valid amount.";
        return;
    }

    const apiBase = getApiBase();
    msgEl.style.color = "white";
    msgEl.textContent = "Placing bet...";

    try {
        const res = await fetch(`${apiBase}/api/betting/place`, {
            method: 'POST',
            credentials: 'include',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ choice: choice, amount: amount }) // "over" or "under"
        });

        const data = await res.json();

        if (res.ok && data.success) {
            msgEl.style.color = "#00ff64";
            msgEl.textContent = `Success! Bet locked in. New Balance: ${data.new_balance} FT`;
            amountInput.value = '';
            window.loadUserProfile(true); // <--- PASS TRUE HERE!
        } else {
            msgEl.style.color = "#ff4655";
            msgEl.textContent = data.error || "Failed to place bet.";
        }
    } catch (err) {
        msgEl.style.color = "#ff4655";
        msgEl.textContent = "Network error. Try again.";
    }
};

// --- ADMIN CONTROLS ---

document.addEventListener('keydown', function(e) {
    if (e.shiftKey && e.key === 'A') {
        const panel = document.getElementById('adminPanel');
        if (panel) panel.style.display = panel.style.display === 'none' ? 'block' : 'none';
    }
});

let pendingPreview = null;

window.previewPropBet = async function() {
    const token = document.getElementById('adminTokenInput').value;
    const player = document.getElementById('adminPlayerSelect').value;
    const propType = document.getElementById('adminPropSelect').value; // Grab the prop type!
    const msgEl = document.getElementById('adminMessage');
    const apiBase = getApiBase();

    msgEl.style.color = "white";
    msgEl.textContent = "Crunching historical stats...";

    try {
        const res = await fetch(`${apiBase}/api/admin/preview-prop`, {
            method: 'POST',
            headers: { 'X-Admin-Token': token, 'Content-Type': 'application/json' },
            body: JSON.stringify({ player: player, prop_type: propType }) // Send it!
        });
        
        if (res.ok) {
            pendingPreview = await res.json();
            
            let displayType = pendingPreview.prop_type.toUpperCase().replace('_', '/');
            if (pendingPreview.prop_type === 'match_result') displayType = "MATCH RESULT";

            document.getElementById('previewPlayer').textContent = pendingPreview.player.toUpperCase();
            document.getElementById('previewType').textContent = displayType; // Formatted nicely!
            document.getElementById('previewLine').textContent = Number(pendingPreview.line.toFixed(2));
            document.getElementById('previewOver').textContent = pendingPreview.over_multiplier.toFixed(2) + 'x';
            document.getElementById('previewUnder').textContent = pendingPreview.under_multiplier.toFixed(2) + 'x';
            
            document.getElementById('adminPreviewBox').style.display = 'block';
            msgEl.textContent = "";
        } else {
            const err = await res.json();
            msgEl.style.color = "#ff4655";
            msgEl.textContent = err.error || "Failed to generate preview.";
        }
    } catch (e) {
        msgEl.style.color = "#ff4655";
        msgEl.textContent = "Network error.";
    }
};

window.publishPropBet = async function() {
    if (!pendingPreview) return;
    const token = document.getElementById('adminTokenInput').value;
    const apiBase = getApiBase();

    try {
        const res = await fetch(`${apiBase}/api/admin/publish-prop`, {
            method: 'POST',
            headers: { 'X-Admin-Token': token, 'Content-Type': 'application/json' },
            body: JSON.stringify(pendingPreview)
        });
        
        if (res.ok) {
            document.getElementById('adminMessage').style.color = "#00ff64";
            document.getElementById('adminMessage').textContent = "MARKET PUBLISHED! The fans can now bet.";
            loadBettingMarket(); // Instantly show the new market on the screen!
        }
    } catch (e) {
        document.getElementById('adminMessage').style.color = "#ff4655";
        document.getElementById('adminMessage').textContent = "Failed to publish.";
    }
};

window.lockPropMarket = async function() {
    const token = document.getElementById('adminTokenInput').value;
    const msgEl = document.getElementById('adminMessage');
    const apiBase = getApiBase();

    try {
        const res = await fetch(`${apiBase}/api/admin/lock-prop`, {
            method: 'POST',
            headers: { 'X-Admin-Token': token }
        });
        const data = await res.json();
        if (res.ok) {
            msgEl.style.color = "#ffaa00";
            msgEl.textContent = data.message;
            loadBettingMarket(); // Refreshes the public UI to show "MARKET CLOSED" badge
        } else {
            msgEl.style.color = "#ff4655";
            msgEl.textContent = data.error || "Failed to lock.";
        }
    } catch (e) {
        msgEl.textContent = "Network error.";
    }
};

window.resolvePropMarket = async function(outcome) {
    if (!confirm(`Resolve market as ${outcome.toUpperCase()}? This pays out the tokens and permanently closes the prop.`)) return;

    const token = document.getElementById('adminTokenInput').value;
    const msgEl = document.getElementById('adminMessage');
    const apiBase = getApiBase();

    try {
        const res = await fetch(`${apiBase}/api/admin/resolve-prop`, {
            method: 'POST',
            headers: { 'X-Admin-Token': token, 'Content-Type': 'application/json' },
            body: JSON.stringify({ outcome: outcome })
        });
        const data = await res.json();
        if (res.ok) {
            msgEl.style.color = "#00ff64";
            msgEl.textContent = data.message;
            loadBettingMarket(); // Reverts UI to the grey "NO ACTIVE PROPOSITION"
            loadUserProfile();   // Updates your own wallet instantly if you won!
        } else {
            msgEl.style.color = "#ff4655";
            msgEl.textContent = data.error || "Failed to resolve.";
        }
    } catch (e) {
        msgEl.textContent = "Network error.";
    }
};

let usersLoaded = false;
window.fetchAdminUsers = async function() {
    if (usersLoaded) return; // Only fetch once
    const token = document.getElementById('adminTokenInput').value;
    if (!token) return;

    const apiBase = getApiBase();
    
    try {
        const res = await fetch(`${apiBase}/api/admin/users`, {
            headers: { 'X-Admin-Token': token }
        });
        if (res.ok) {
            const users = await res.json();
            const select = document.getElementById('adminUserSelect');
            select.innerHTML = ''; // Clear loading text
            
            users.forEach(u => {
                const opt = document.createElement('option');
                opt.value = u.DiscordID;
                opt.textContent = `${u.Username} (Linked: ${u.Linked})`;
                select.appendChild(opt);
            });
            usersLoaded = true;
        }
    } catch (e) {
        console.error("Failed to load users");
    }
}

window.linkUserToPlayer = async function() {
    const token = document.getElementById('adminTokenInput').value;
    const discordId = document.getElementById('adminUserSelect').value;
    const player = document.getElementById('adminLinkPlayerSelect').value;
    const msgEl = document.getElementById('adminMessage');
    const apiBase = getApiBase();

    try {
        const res = await fetch(`${apiBase}/api/admin/link-user`, {
            method: 'POST',
            headers: { 'X-Admin-Token': token, 'Content-Type': 'application/json' },
            body: JSON.stringify({ discord_id: discordId, player: player })
        });
        const data = await res.json();
        if (res.ok) {
            msgEl.style.color = "#00ff64";
            msgEl.textContent = data.message;
            usersLoaded = false; // Force refresh the dropdown next time they click it
            fetchAdminUsers();
        } else {
            msgEl.style.color = "#ff4655";
            msgEl.textContent = data.error;
        }
    } catch (e) {
        msgEl.textContent = "Network error.";
    }
}

window.cancelEntireMarket = async function() {
    if (!confirm("🚨 ABORT MARKET? This will instantly cancel the event and mass-refund everyone's Fredtokens. Are you sure?")) return;

    const token = document.getElementById('adminTokenInput').value;
    const msgEl = document.getElementById('adminMessage');
    const apiBase = getApiBase();

    msgEl.style.color = "white";
    msgEl.textContent = "Refunding all users...";

    try {
        const res = await fetch(`${apiBase}/api/admin/cancel-market`, {
            method: 'POST',
            headers: { 'X-Admin-Token': token }
        });
        const data = await res.json();
        
        if (res.ok) {
            msgEl.style.color = "#00ff64";
            msgEl.textContent = data.message;
            loadBettingMarket(); // UI returns to the grey "No Active Proposition" screen
            loadUserProfile();   // Instantly updates your own wallet if you had a bet placed!
        } else {
            msgEl.style.color = "#ff4655";
            msgEl.textContent = data.error || "Failed to cancel market.";
        }
    } catch (e) {
        msgEl.textContent = "Network error.";
    }
};

document.addEventListener('DOMContentLoaded', () => {
    const btnOpenCase = document.getElementById('btnOpenCase');
    if (!btnOpenCase) return; // Only run if we are on the packs page
    
    const track = document.getElementById('spinnerTrack');
    const winnerReveal = document.getElementById('winnerReveal');
    const winnerCardDisplay = document.getElementById('winnerCardDisplay');

    // --- NEW: PRE-FILL THE SPINNER ON LOAD ---
    function initSpinnerPreview() {
        if (!track) return;
        track.innerHTML = ""; 
        const fillerRarities = [
            "iron", "iron", "iron", "iron", "bronze", "bronze", 
            "diamond", "ascendant", "immortal", "radiant"
        ]; 

        // Generate 10 static cards to fill the window
        for (let i = 0; i < 10; i++) {
            const cardEl = document.createElement('div');
            const cardRarity = fillerRarities[Math.floor(Math.random() * fillerRarities.length)];
            
            cardEl.className = `tcg-card rarity-${cardRarity}`;
            cardEl.style.backgroundImage = `linear-gradient(135deg, #1a1a1a 0%, #0a0a0a 100%)`;
            cardEl.style.justifyContent = "center"; 
            cardEl.innerHTML = `
                <span style="font-size: 4rem; opacity: 0.1; font-family: sans-serif;">?</span>
                <span style="position: absolute; bottom: 10px; font-size: 0.8rem; color: rgba(255,255,255,0.5); text-transform: uppercase;">${cardRarity}</span>
            `;
            track.appendChild(cardEl);
        }
        
        // Offset it slightly so it doesn't look perfectly rigid
        track.style.transform = `translateX(-20px)`;
    }

    // Call it immediately when the page loads!
    initSpinnerPreview();
    
    // -----------------------------------------

        btnOpenCase.addEventListener('click', async () => {
        btnOpenCase.disabled = true;
        btnOpenCase.innerText = "Opening...";
        winnerReveal.style.display = "none";
        track.style.transition = "none"; 
        track.style.transform = "translateX(0px)"; 
        track.innerHTML = ""; 

        try {
            // 1. Ask the server to buy the pack and give us a card
            const response = await fetch(`${getApiBase()}/api/economy/buy-pack`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                credentials: 'include'
                // NOTE: Make sure your fetch includes credentials if you are using cookies!
                // credentials: 'include' 
            });

            const data = await response.json();

            if (!response.ok) {
                alert("Error: " + (data.error || "Could not open pack"));
                btnOpenCase.disabled = false;
                btnOpenCase.innerText = "Open Case (250 FT)";
                return;
            }

            // 2. We have the real winning card from the DB!
            const winningCardData = data.card;
            const winningIndex = 45; 

            // Create a weighted array of our new Valorant Ranks for the filler cards
            const fillerRarities = [
                "iron", "iron", "iron", "iron", "iron", 
                "bronze", "bronze", "bronze", 
                "diamond", "diamond", 
                "ascendant", 
                "immortal", 
                "radiant"
            ]; 

            // 3. Generate the 60 Mystery Cards
            for (let i = 0; i < 60; i++) {
                const cardEl = document.createElement('div');
                let cardRarity = "";
                
                if (i === winningIndex) {
                    // The winner gets its true rarity, but its identity is HIDDEN!
                    cardRarity = winningCardData.rarity;
                    cardEl.id = "spinningWinningCard"; // We need this ID to find it when it stops
                } else {
                    // Random filler rarity
                    cardRarity = fillerRarities[Math.floor(Math.random() * fillerRarities.length)];
                }
                
                // Style it as a Mystery Box
                cardEl.className = `tcg-card rarity-${cardRarity}`;
                cardEl.style.backgroundImage = `linear-gradient(135deg, #1a1a1a 0%, #0a0a0a 100%)`;
                cardEl.style.justifyContent = "center"; // Center the question mark
                cardEl.innerHTML = `
                    <span style="font-size: 4rem; opacity: 0.1; font-family: sans-serif;">?</span>
                    <span style="position: absolute; bottom: 10px; font-size: 0.8rem; color: rgba(255,255,255,0.5); text-transform: uppercase;">${cardRarity}</span>
                `;
                
                track.appendChild(cardEl);
            }

            track.getBoundingClientRect(); // Force render

            // 4. Calculate where to stop
            const totalCardWidth = 160 + 20; 
            const offsetToWinner = (winningIndex * totalCardWidth);
            const centerAdjustment = (track.parentElement.offsetWidth / 2) - (totalCardWidth / 2);
            const randomSuspense = Math.floor(Math.random() * 80) - 40; 
            const finalTransform = offsetToWinner - centerAdjustment + randomSuspense;

            // 5. Spin it!
            setTimeout(() => {
                track.style.transition = "transform 6s cubic-bezier(0.15, 0.9, 0.15, 1)";
                track.style.transform = `translateX(-${finalTransform}px)`;
            }, 50);

            // 6. THE DRAMATIC REVEAL (Runs when the spinner stops)
            setTimeout(() => {
                let imgPath = winningCardData.image_url;
                if (imgPath && !imgPath.startsWith('http') && !imgPath.startsWith('/')) {
                    imgPath = '/' + imgPath; 
                }

                // A. Reveal the card *inside* the spinner track
                const spinningWinner = document.getElementById('spinningWinningCard');
                if (spinningWinner) {
                    spinningWinner.style.justifyContent = "flex-end"; // Move text back to bottom
                    spinningWinner.style.backgroundImage = `linear-gradient(to top, rgba(0,0,0,0.9) 0%, rgba(0,0,0,0) 40%), url('${imgPath}')`;
                    spinningWinner.innerHTML = ``;
                    
                    // Add a little flash effect to make it pop!
                    spinningWinner.style.boxShadow = "0 0 40px rgba(255,255,255,0.8)";
                    setTimeout(() => { spinningWinner.style.boxShadow = ""; }, 500);
                }

                // B. Show the big winner display below
                winnerCardDisplay.innerHTML = `
                    <div class="tcg-card rarity-${winningCardData.rarity}" 
                         style="margin: 0 auto; width: 240px; height: 336px; font-size: 1.5rem; 
                                background-image: linear-gradient(to top, rgba(0,0,0,0.9) 0%, rgba(0,0,0,0) 40%), url('${imgPath}');">
                    </div>`;
                
                winnerReveal.style.display = "block";
                
                // C. Reset the button
                btnOpenCase.disabled = false;
                btnOpenCase.innerText = "Open Case (250 FT)";
                
                if (typeof window.loadUserProfile === 'function') {
                    window.loadUserProfile();
                }
                
            }, 6000); 

        } catch (error) {
            console.error("Failed to open case:", error);
            alert("Something went wrong with the server.");
            btnOpenCase.disabled = false;
            btnOpenCase.innerText = "Open Case (250 FT)";
        }
    });
});

window.claimDailyReward = async function() {
    const btn = document.getElementById('btnDailyReward');
    const msg = document.getElementById('dailyMessage');
    if (!btn) return;

    btn.disabled = true;
    btn.innerText = "CLAIMING...";
    msg.innerText = "";

    try {
        const res = await fetch(`${getApiBase()}/api/economy/daily`, {
            method: 'POST',
            credentials: 'include'
        });

        const data = await res.json();

        if (res.ok) {
            msg.style.color = "#00ff64";
            msg.innerText = data.message;
            btn.innerText = "CLAIMED!";
            btn.style.background = "#333";
            btn.style.boxShadow = "none";
            btn.style.color = "#666";
            
            // Instantly update the user's wallet in the top right corner!
            if (typeof window.loadUserProfile === 'function') {
                window.loadUserProfile();
            }
        } else {
            msg.style.color = "#ffaa00"; // Orange color for the cooldown timer
            msg.innerText = data.error;
            btn.disabled = false;
            btn.innerText = "CLAIM 250 FT";
        }
    } catch (err) {
        msg.style.color = "#ff4655";
        msg.innerText = "Network Error.";
        btn.disabled = false;
        btn.innerText = "CLAIM 250 FT";
    }
};

window.checkDailyRewardStatus = async function() {
    const btn = document.getElementById('btnDailyReward');
    const msg = document.getElementById('dailyMessage');
    if (!btn) return; // Only run if we are on the packs page

    try {
        const res = await fetch(`${getApiBase()}/api/economy/daily`, {
            method: 'GET',
            credentials: 'include'
        });

        if (res.ok) {
            const data = await res.json();
            
            if (!data.available) {
                // Instantly lock and grey out the button!
                btn.disabled = true;
                btn.innerText = "ON COOLDOWN";
                btn.style.background = "#333";
                btn.style.color = "#666";
                btn.style.boxShadow = "none";
                msg.style.color = "#ffaa00";
                msg.innerText = `Come back in ${data.hours}h ${data.minutes}m!`;
            }
        }
    } catch (err) {
        console.error("Failed to check daily status.");
    }
};

// Listen for the page to load, then run the check!
document.addEventListener('DOMContentLoaded', () => {
    if (document.getElementById('btnDailyReward')) {
        checkDailyRewardStatus();
    }
});

// --- DAILY MISSIONS INJECTOR ---
async function loadDailyMissions() {
    const list = document.getElementById('missionsList');
    if (!list) return; // Only run if we are on a page that has the missions box!

    try {
        const res = await fetch(`${getApiBase()}/api/quests`, { credentials: 'include' });
        if (!res.ok) throw new Error();
        const data = await res.json();
        
        list.innerHTML = ""; // Clear loading text

        data.quests.forEach(qData => {
            const q = qData.quest;
            const claimed = qData.claimed;
            
            // Color coding based on difficulty
            let accentColor = "#2bc97e"; // Easy (Green)
            if (q.difficulty === "medium") accentColor = "#ffaa00"; // Yellow
            if (q.difficulty === "hard") accentColor = "#ff4655"; // Red

            const btnHtml = claimed 
                ? `<button disabled style="background: #333; color: #666; border: none; padding: 8px 15px; border-radius: 4px; font-family: 'Orbitron'; font-weight: bold; cursor: not-allowed;">COMPLETED</button>`
                : `<button onclick="verifyQuest('${q.difficulty}')" style="background: ${accentColor}; color: #000; border: none; padding: 8px 15px; border-radius: 4px; font-family: 'Orbitron'; font-weight: bold; cursor: pointer; transition: transform 0.2s;" onmouseover="this.style.transform='scale(1.05)'" onmouseout="this.style.transform='scale(1)'">VERIFY MATCHES</button>`;

            list.innerHTML += `
                <div style="display: flex; justify-content: space-between; align-items: center; background: rgba(255,255,255,0.03); padding: 15px; border-radius: 6px; border-left: 4px solid ${accentColor};">
                    <div>
                        <div style="font-family: 'Orbitron'; color: white; font-size: 1.2rem; font-weight: bold;">${q.title}</div>
                        <div style="font-family: 'Rajdhani'; color: #aaa; font-size: 1.1rem; margin-top: 4px;">${q.description}</div>
                    </div>
                    <div style="text-align: right;">
                        <div style="font-family: 'Orbitron'; color: #00d4ff; font-weight: bold; font-size: 1.1rem; margin-bottom: 8px;">+${q.reward} FT</div>
                        ${btnHtml}
                    </div>
                </div>
            `;
        });

    } catch (err) {
        list.innerHTML = `<p style="text-align: center; color: #ff4655; font-family: 'Rajdhani', sans-serif; font-size: 1.2rem;">Please log in to view and track your daily missions.</p>`;
    }

    // Start the Reset Timer
    setInterval(() => {
        const timerText = document.getElementById('resetTimer');
        if (!timerText) return;

        const now = new Date();
        let nextReset = new Date();
        nextReset.setUTCHours(2, 0, 0, 0); // 2:00 AM UTC
        if (now.getTime() > nextReset.getTime()) {
            nextReset.setUTCDate(nextReset.getUTCDate() + 1);
        }
        
        const diff = nextReset - now;
        const h = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
        const m = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
        
        timerText.innerText = `Resets in: ${h}h ${m}m`;
    }, 1000);
}

// The placeholder verification function
window.verifyQuest = async function(difficulty) {
    const apiBase = getApiBase();
    
    // Find the specific button that was clicked so we can animate it
    const list = document.getElementById('missionsList');
    const buttons = list.querySelectorAll('button');
    let targetBtn = null;
    
    // Simple way to find the button we just clicked based on difficulty color/text
    buttons.forEach(btn => {
        if (btn.getAttribute('onclick') && btn.getAttribute('onclick').includes(difficulty)) {
            targetBtn = btn;
        }
    });

    if (targetBtn) {
        targetBtn.disabled = true;
        targetBtn.innerText = "SCANNING RIOT API...";
        targetBtn.style.background = "#fff";
        targetBtn.style.color = "#000";
    }

    try {
        const res = await fetch(`${apiBase}/api/quests/verify`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            credentials: 'include',
            body: JSON.stringify({ difficulty: difficulty })
        });

        const data = await res.json();

        if (res.ok) {
            // SUCCESS! 
            alert(data.message);
            
            // Reload the missions UI to show it as "COMPLETED"
            loadDailyMissions();
            
            // Instantly update the user's wallet in the top right
            if (typeof window.loadUserProfile === 'function') {
                window.loadUserProfile();
            }
        } else {
            // FAILED (or not done yet)
            alert("Status: " + data.error);
            if (targetBtn) {
                targetBtn.disabled = false;
                targetBtn.innerText = "VERIFY MATCHES";
                
                // Reset colors based on difficulty
                if (difficulty === 'easy') targetBtn.style.background = "#2bc97e";
                if (difficulty === 'medium') targetBtn.style.background = "#ffaa00";
                if (difficulty === 'hard') targetBtn.style.background = "#ff4655";
            }
        }
    } catch (err) {
        alert("Network error while scanning matches.");
        if (targetBtn) {
            targetBtn.disabled = false;
            targetBtn.innerText = "VERIFY MATCHES";
        }
    }
};

// Run it when the page loads!
document.addEventListener('DOMContentLoaded', loadDailyMissions);

function getPlayerStats(match, playerPuuid) {
    // 1. Find the player using their uncensored PUUID instead of their name
    let players = match && match.players;
    if (players && !Array.isArray(players) && players.all_players) {
        players = players.all_players;
    }
    if (!Array.isArray(players)) return null;
    
    const me = players.find(p => p && p.puuid === playerPuuid);
    if (!me || !me.stats) return null;

    // 2. Extract Kills, Deaths, and Assists
    const kills = me.stats.kills;
    const deaths = me.stats.deaths;
    const assists = me.stats.assists;

    // 3. Calculate Average Combat Score (ACS)
    // The API gives us total score, so we divide by total rounds played
    const totalScore = me.stats.score;
    const roundsPlayed = match.metadata.rounds_played;
    let acs = 0;
    
    if (roundsPlayed > 0) {
        acs = Math.round(totalScore / roundsPlayed);
    }

    // Return it as a neat object to use in your HTML
    return {
        kda: `${kills}/${deaths}/${assists}`,
        acs: acs,
        kdRatio: deaths === 0 ? kills : (kills / deaths).toFixed(2)
    };
}

window.publishPropBet = async function() {
    if (!pendingPreview) return;
    const token = document.getElementById('adminTokenInput').value;
    const apiBase = getApiBase();

    // NEW: Grab all checked vetoes from the grid!
    const vetoes = Array.from(document.querySelectorAll('#generatedVetoGrid input:checked')).map(cb => cb.value);
    pendingPreview.vetoes = vetoes;

    try {
        const res = await fetch(`${apiBase}/api/admin/publish-prop`, {
            method: 'POST',
            headers: { 'X-Admin-Token': token, 'Content-Type': 'application/json' },
            body: JSON.stringify(pendingPreview)
        });
        
        if (res.ok) {
            document.getElementById('adminMessage').style.color = "#00ff64";
            document.getElementById('adminMessage').textContent = "MARKET PUBLISHED! The fans can now bet.";
            
            // Clear the checkboxes for next time
            document.querySelectorAll('#generatedVetoGrid input:checked').forEach(cb => cb.checked = false);
            loadBettingMarket();
        }
    } catch (e) {
        document.getElementById('adminMessage').style.color = "#ff4655";
        document.getElementById('adminMessage').textContent = "Failed to publish.";
    }
};

window.publishCustomMarket = async function() {
    const token = document.getElementById('adminTokenInput').value;
    const msgEl = document.getElementById('adminMessage');
    const apiBase = getApiBase();

    const targetName = document.getElementById('customTargetName').value;
    const propName = document.getElementById('customPropName').value;
    const line = parseFloat(document.getElementById('customLine').value) || 0.5;
    const overMult = parseFloat(document.getElementById('customOver').value) || 1.5;
    const underMult = parseFloat(document.getElementById('customUnder').value) || 1.5;

    // NEW: Grab all checked vetoes from the custom grid!
    const vetoes = Array.from(document.querySelectorAll('#customVetoGrid input:checked')).map(cb => cb.value);

    if (!targetName || !propName) {
        msgEl.style.color = "#ff4655";
        msgEl.textContent = "Please enter a Target Player and Bet Name.";
        return;
    }

    const customMarket = {
        player: targetName,
        prop_type: propName,
        line: line,
        over_multiplier: overMult,
        under_multiplier: underMult,
        is_open: true,
        vetoes: vetoes // Ship the array to Go!
    };

    try {
        const res = await fetch(`${apiBase}/api/admin/publish-prop`, {
            method: 'POST',
            headers: { 'X-Admin-Token': token, 'Content-Type': 'application/json' },
            body: JSON.stringify(customMarket)
        });
        
        if (res.ok) {
            msgEl.style.color = "#ffaa00";
            msgEl.textContent = "CUSTOM MARKET PUBLISHED! The fans can now bet.";
            
            // Clear all inputs
            document.getElementById('customTargetName').value = '';
            document.getElementById('customPropName').value = '';
            document.getElementById('customOver').value = '';
            document.getElementById('customUnder').value = '';
            document.querySelectorAll('#customVetoGrid input:checked').forEach(cb => cb.checked = false);
            
            loadBettingMarket();
        } else {
            msgEl.style.color = "#ff4655";
            msgEl.textContent = "Failed to publish custom market.";
        }
    } catch (e) {
        msgEl.style.color = "#ff4655";
        msgEl.textContent = "Network error.";
    }
};
