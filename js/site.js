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
})();

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
    const me = players.find(
        (p) =>
            p &&
            String(p.name).toLowerCase() === riotName.toLowerCase() &&
            String(p.tag).toLowerCase() === riotTag.toLowerCase()
    );
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

function playerKeyJs(name, tag) {
    return `${String(name).toLowerCase()}#${String(tag).toLowerCase()}`;
}

function isCompetitiveMode(mode) {
    if (!mode || typeof mode !== 'string') return false;
    const m = mode.trim().toLowerCase();
    return m === 'competitive' || m === 'premier';
}

function renderRosterMatchRow(entry, selectedKeys) {
    const match = entry.match;
    const roster = entry.roster || [];
    let primary = roster[0] || { name: '', tag: '' };
    if (selectedKeys && selectedKeys.size > 0) {
        const prefer = roster.find((r) => selectedKeys.has(playerKeyJs(r.name, r.tag)));
        if (prefer) primary = prefer;
    }
    const rosterLine = roster
        .filter((r) => !selectedKeys || selectedKeys.size === 0 || selectedKeys.has(playerKeyJs(r.name, r.tag)))
        .map((r) => `${r.name}#${r.tag}`)
        .join(' · ');
    return renderMatchRow(match, primary.name, primary.tag, rosterLine);
}

function renderMatchRow(match, riotName, riotTag, rosterLine = '') {
    const meta = match.metadata || {};
    const mapName = meta.map || 'Unknown map';
    const mode = meta.mode || 'Unknown mode';
    const red = (match.teams && match.teams.red && match.teams.red.rounds_won) ?? '—';
    const blue = (match.teams && match.teams.blue && match.teams.blue.rounds_won) ?? '—';
    const outcome = outcomeForPlayer(match, riotName, riotTag);
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
    const metaBits = [];
    if (rosterLine) metaBits.push(rosterLine);
    if (mode) metaBits.push(mode);
    if (meta.region) metaBits.push(meta.region);
    const metaText = metaBits.map((s) => escapeHtml(s)).join(' · ');
    li.innerHTML = `
        <div class="match-date">${formatMatchDate(meta)}</div>
        <div class="match-main">
            <h3>${escapeHtml(mapName)}</h3>
            <p class="match-meta">${metaText}</p>
            <p class="match-scoreline">Red ${escapeHtml(String(red))} – ${escapeHtml(String(blue))} Blue</p>
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
