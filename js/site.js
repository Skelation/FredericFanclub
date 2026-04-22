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
})();

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
    const players = match && match.players && match.players.all_players;
    if (!Array.isArray(players)) return null;
    const me = findPlayerInMatch(match, riotName, riotTag);
    if (!me || !me.team) return null;
    const key = String(me.team).toLowerCase();
    const myTeam = match.teams && match.teams[key];
    if (myTeam && typeof myTeam.has_won === 'boolean') {
        return myTeam.has_won ? 'win' : 'loss';
    }
    const red = (match.teams && match.teams.red && match.teams.red.rounds_won) || 0;
    const blue = (match.teams && match.teams.blue && match.teams.blue.rounds_won) || 0;
    if (me.team === 'Red') {
        if (red > blue) return 'win';
        if (red < blue) return 'loss';
    }
    if (me.team === 'Blue') {
        if (blue > red) return 'win';
        if (blue < red) return 'loss';
    }
    return null;
}

function findPlayerInMatch(match, riotName, riotTag) {
    const players = match && match.players && match.players.all_players;
    if (!Array.isArray(players)) return null;
    return (
        players.find(
            (p) =>
                p &&
                String(p.name).toLowerCase() === String(riotName).toLowerCase() &&
                String(p.tag).toLowerCase() === String(riotTag).toLowerCase()
        ) || null
    );
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

function renderRosterMatchRow(entry, selectedKeys) {
    const match = entry.match;
    const roster = entry.roster || [];
    const rrByPlayer = (entry && entry.rrByPlayer) || {};
    let primary = roster[0] || { name: '', tag: '' };
    if (selectedKeys && selectedKeys.size > 0) {
        const prefer = roster.find((r) => selectedKeys.has(playerKeyJs(r.name, r.tag)));
        if (prefer) primary = prefer;
    }
    const rrOverride = rrByPlayer[playerKeyJs(primary.name, primary.tag)];
    return renderMatchRow(match, primary.name, primary.tag, rrOverride);
}

function renderMatchRow(match, riotName, riotTag, rrOverride = null) {
    const meta = match.metadata || {};
    const mapName = meta.map || 'Unknown map';
    const red = (match.teams && match.teams.red && match.teams.red.rounds_won) ?? '—';
    const blue = (match.teams && match.teams.blue && match.teams.blue.rounds_won) ?? '—';
    const outcome = outcomeForPlayer(match, riotName, riotTag);
    const rrDelta = Number.isFinite(Number(rrOverride)) ? Number(rrOverride) : ratingDeltaForPlayer(match, riotName, riotTag);
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

    const li = document.createElement('li');
    li.className = 'match-card';
    li.innerHTML = `
        ${makePlayerAvatar(riotName, riotTag)}
        <div class="match-main">
            <h3>${escapeHtml(mapName)}</h3>
            <p class="match-player">${escapeHtml(riotName)}</p>
            <p class="match-scoreline">Attackers ${escapeHtml(String(red))} – ${escapeHtml(String(blue))} Defenders</p>
            <p class="match-rating-change ${rrClass}">${escapeHtml(rrLabel)}</p>
        </div>
        <span class="match-result ${resultClass}">${escapeHtml(resultLabel)}</span>
    `;
    return li;
}

function escapeHtml(s) {
    return String(s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
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
            msg = `Showing ${filtered.length} competitive game(s) (${comp.length} competitive loaded, ${state.allEntries.length} total from API).`;
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
            'Add <meta name="fred-api-base" content="http://127.0.0.1:8080"> to matches.html (your Go server URL).';
        return;
    }

    const url = `${apiBase}/api/matches/roster`;
    statusEl.textContent = 'Loading roster matches…';

    let res;
    try {
        res = await fetch(url);
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
