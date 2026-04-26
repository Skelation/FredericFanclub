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
    window.loadUserProfile = async function() {
        const authContainer = document.getElementById('authContainer');
        if (!authContainer) return;

        // Make sure we have the API URL
        const meta = document.querySelector('meta[name="fred-api-base"]');
        const apiBase = ((meta && meta.getAttribute('content')) || 'https://api.fredericfan.club').replace(/\/$/, '');

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
                    loadBettingMarket();
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

async function loadMatchHistory(matchList, statusEl) {
    const meta = document.querySelector('meta[name="fred-api-base"]');
    const apiBase = ((meta && meta.getAttribute('content')) || '').replace(/\/$/, '');

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

    const apiBase = document.querySelector('meta[name="fred-api-base"]').getAttribute('content').replace(/\/$/, '');
    
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
            document.getElementById('propTypeName').textContent = "Total " + data.prop_type;
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

    const apiBase = document.querySelector('meta[name="fred-api-base"]').getAttribute('content').replace(/\/$/, '');
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
            window.loadUserProfile(); 
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
    const apiBase = document.querySelector('meta[name="fred-api-base"]').getAttribute('content').replace(/\/$/, '');

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
    const apiBase = document.querySelector('meta[name="fred-api-base"]').getAttribute('content').replace(/\/$/, '');

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
    const apiBase = document.querySelector('meta[name="fred-api-base"]').getAttribute('content').replace(/\/$/, '');

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
    const apiBase = document.querySelector('meta[name="fred-api-base"]').getAttribute('content').replace(/\/$/, '');

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

    const apiBase = document.querySelector('meta[name="fred-api-base"]').getAttribute('content').replace(/\/$/, '');
    
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
    const apiBase = document.querySelector('meta[name="fred-api-base"]').getAttribute('content').replace(/\/$/, '');

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
    const apiBase = document.querySelector('meta[name="fred-api-base"]').getAttribute('content').replace(/\/$/, '');

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
