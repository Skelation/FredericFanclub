// Change this if you ever change your primary tracker in the Go backend!

// Remove the ANCHOR_PLAYER constant since we now use a roster array

window.premierMatchesData = [];

// Helper: Aggressively hunts down the team name no matter where the API hides it
function extractTeamName(teamData, fallback) {
    if (!teamData) return fallback;
    if (teamData.roster && teamData.roster.name) return teamData.roster.name;
    if (teamData.customization && teamData.customization.name) return teamData.customization.name;
    if (teamData.name) return teamData.name;
    return fallback;
}

// ── FREDDO SCORE ─────────────────────────────────────────────────────────────
// Season-level composite score (0-100) using aggregated player stats
function calcFreddoScore(p) {
    const fbPerMap = p.first_bloods / Math.max(1, p.matches);
    const fdPerMap = p.first_deaths / Math.max(1, p.matches);
    const clPerMap = p.clutches / Math.max(1, p.matches);

    const acsScore    = Math.min(p.average_acs / 280, 1.2) * 30;
    const kdScore     = Math.min(p.kd / 1.8, 1.2) * 25;
    const kastScore   = (p.kast / 100) * 25;
    const hsScore     = Math.min(p.headshot_percentage / 30, 1) * 5;
    const fbScore     = Math.min(fbPerMap / 2.5, 1) * 8;
    const clutchScore = Math.min(clPerMap / 0.4, 1) * 7;
    const fdPenalty   = Math.min(fdPerMap / 2.5, 1) * 8;

    return Math.max(0, Math.round(Math.min(100,
        acsScore + kdScore + kastScore + hsScore + fbScore + clutchScore - fdPenalty
    )));
}

// Returns per-component contributions for display in the player detail panel
function calcFreddoBreakdown(p) {
    const fbPerMap = p.first_bloods / Math.max(1, p.matches);
    const fdPerMap = p.first_deaths / Math.max(1, p.matches);
    const clPerMap = p.clutches / Math.max(1, p.matches);
    return [
        { label: 'ACS',           pts: Math.min(p.average_acs / 280, 1.2) * 30, max: 36, raw: Math.round(p.average_acs),            rawLbl: 'ACS moyen', sign: 1 },
        { label: 'K/D',           pts: Math.min(p.kd / 1.8, 1.2) * 25,          max: 30, raw: p.kd.toFixed(2),                     rawLbl: 'K/D',       sign: 1 },
        { label: 'KAST%',         pts: (p.kast / 100) * 25,                      max: 25, raw: Math.round(p.kast) + '%',            rawLbl: 'KAST',      sign: 1 },
        { label: 'HS%',           pts: Math.min(p.headshot_percentage / 30, 1) * 5,  max: 5,  raw: Math.round(p.headshot_percentage) + '%', rawLbl: 'HS%', sign: 1 },
        { label: 'First Bloods',  pts: Math.min(fbPerMap / 2.5, 1) * 8,          max: 8,  raw: fbPerMap.toFixed(2),                 rawLbl: 'FB/map',    sign: 1 },
        { label: 'Clutches',      pts: Math.min(clPerMap / 0.4, 1) * 7,          max: 7,  raw: clPerMap.toFixed(2),                 rawLbl: 'Cl/map',    sign: 1 },
        { label: 'First Deaths',  pts: Math.min(fdPerMap / 2.5, 1) * 8,          max: 8,  raw: fdPerMap.toFixed(2),                 rawLbl: 'FD/map',    sign: -1 },
    ];
}

// Per-game score (0-100) from raw match stats (no KAST/FB/clutch available per game)
function calcMatchFreddoScore(kills, deaths, assists, score, headshots, bodyshots, legshots, rounds) {
    const acs = score / Math.max(1, rounds);
    const kd  = kills / Math.max(1, deaths);
    const kda = (kills + assists * 0.5) / Math.max(1, deaths);
    const tot = headshots + bodyshots + legshots;
    const hs  = tot > 0 ? headshots / tot * 100 : 0;

    return Math.max(0, Math.round(Math.min(100,
        Math.min(acs / 280, 1.2) * 40 +
        Math.min(kd  / 1.8, 1.2) * 30 +
        Math.min(hs  / 30, 1)    * 15 +
        Math.min(kda / 2,  1)    * 15
    )));
}

// D → C → B → A → S → S+  tiers
function getFreddoTier(score) {
    if (score >= 85) return { tier: 'S+', color: '#ffea82' };
    if (score >= 75) return { tier: 'S',  color: '#ffd94a' };
    if (score >= 65) return { tier: 'A',  color: '#39ff85' };
    if (score >= 55) return { tier: 'B',  color: '#00d4ff' };
    if (score >= 40) return { tier: 'C',  color: '#c8d8f0' };
    return               { tier: 'D',  color: '#ff4655' };
}

async function loadPremierMatches() {
    const listDiv = document.getElementById('matchList');
    try {
        const res = await fetch(`${getApiBase()}/api/matches/premier`);

        if (res.status === 503) {
            listDiv.innerHTML = `<div class="loader">Servers are syncing Riot data. Please refresh in 10 seconds.</div>`;
            return;
        }

        const data = await res.json();

        if (!data.data || data.data.length === 0) {
            listDiv.innerHTML = `<div style="text-align:center; color:#aaa; padding:20px;">No Premier matches found yet.</div>`;
            return;
        }

        window.premierMatchesData = data.data;
        listDiv.innerHTML = "";

        // Pre-compute match outcomes for the team overview banner
        const _rosterCheck = ["heri", "themistered", "graussbyt", "lal6s9gne", "hhj", "riboox"];
        const matchResults = data.data.map(matchObj => {
            const m = matchObj.match || matchObj;
            const allP = m.players.all_players || m.players;
            let bc = 0, rc = 0;
            allP.forEach(p => {
                const n = p.name.toLowerCase();
                if (_rosterCheck.some(r => n.includes(r))) {
                    if ((p.team || p.team_id) === 'Blue') bc++;
                    else if ((p.team || p.team_id) === 'Red') rc++;
                }
            });
            const teamId = rc > bc ? 'Red' : 'Blue';
            let won = false;
            if (Array.isArray(m.teams)) {
                const t = m.teams.find(x => x.team_id === teamId);
                if (t) won = t.won;
            } else if (m.teams) {
                const t = m.teams[teamId.toLowerCase()];
                if (t) won = t.has_won;
            }
            return { won, map: m.metadata.map };
        });
        renderTeamOverviewBanner(matchResults);

        // 1. Fetch upcoming matches
        const nextRes = await fetch(`${getApiBase()}/api/matches/upcoming`);
        const nextData = await nextRes.json();
        
        listDiv.innerHTML = "";

        if (nextData.matches && nextData.matches.length > 0) {
            nextData.matches.forEach(m => {
                const dateObj = new Date(m.time);
                const dateStr = dateObj.toLocaleDateString('fr-FR', { day: 'numeric', month: 'long', year: 'numeric' });
                const timeStr = dateObj.toLocaleTimeString('fr-FR', { hour: '2-digit', minute: '2-digit' }) + " CET";

                listDiv.innerHTML += `
                <div class="match-card upcoming upcoming-card" data-result="upcoming" data-teams="FRED ${m.opponent}">
                    <div class="match-summary">
                        <div class="match-date">${dateStr}<span>${timeStr}</span></div>
                        <div class="team-info">
                            <img src="images/LogoTransparent.png" class="nav-logo-img" width="60" height="60">
                            <div><div class="team-name">FRED</div><div class="team-tag">#FRED</div></div>
                        </div>
                        <div class="score-block">
                            <div class="score-main"><span style="color:rgba(200,216,240,.2)">VS</span></div>
                            <div class="score-meta upcoming">${m.format}</div>
                            <div class="match-map-badge">${m.maps}</div>
                        </div>
                        <div class="team-info right">
                            <div style="text-align:right">
                                <div class="team-name">${m.opponent}</div>
                                <div class="team-tag">${m.tag}</div>
                            </div>
                        </div>
                        <div class="match-format"><div class="format-tag">PREMIER</div></div>
                    </div>
                <div class="countdown">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12,6 12,12 16,14"/></svg>
                    Dans <strong class="match-timer" data-time="${m.time}">Calcul...</strong> 
                </div>
            </div>`;
            });
        }
        startCountdown();

        listDiv.innerHTML += `<div class="season-heading" style="padding:1.5rem 0 .75rem">Résultats récents</div>`;

        // Render the list of played matches
        data.data.forEach((matchObj, index) => {
            const match = matchObj.match || matchObj; 
            const meta = match.metadata;
            
            const date = new Date(meta.game_start * 1000);
            const dateStr = date.toLocaleDateString();
            const timeStr = date.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'});

            // --- DYNAMIC TEAM DETECTION (ROSTER BASED) ---
            // Include all known team players here (must be lowercase)
            const myRoster = ["heri", "themistered", "graussbyt", "lal6s9gne", "djibはコリーヌ お あいして", "hhj", "Riboox"]
           
            let myRounds = 0, enemyRounds = 0;
            let anchorWon = false;
            let anchorTeamId = "Blue"; // Default fallback
            const roundsPlayed = meta.rounds_played;

            const allPlayers = match.players.all_players || match.players;
            
            // Count how many of our roster players are on Blue vs Red
            let blueCount = 0;
            let redCount = 0;
            
            allPlayers.forEach(p => {
                const pName = p.name.toLowerCase();
                if (myRoster.includes(pName)) {
                    const tId = p.team || p.team_id;
                    if (tId === "Blue") blueCount++;
                    if (tId === "Red") redCount++;
                }
            });

            // The team with the most of our players is OUR team
            if (redCount > blueCount) {
                anchorTeamId = "Red";
            }
            const enemyTeamId = anchorTeamId === "Red" ? "Blue" : "Red";

            let myTeamPlayers = allPlayers.filter(p => (p.team || p.team_id) === anchorTeamId);
            let enemyTeamPlayers = allPlayers.filter(p => (p.team || p.team_id) !== anchorTeamId);

            const sortByScore = (a, b) => b.stats.score - a.stats.score;
            myTeamPlayers.sort(sortByScore);
            enemyTeamPlayers.sort(sortByScore);

            let myTeam = null;
            let enemyTeam = null;

            // Extract the scores based on the detected team ID
            if (Array.isArray(match.teams)) {
                myTeam = match.teams.find(t => t.team_id === anchorTeamId);
                enemyTeam = match.teams.find(t => t.team_id !== anchorTeamId);
                if (myTeam) { myRounds = myTeam.rounds_won; anchorWon = myTeam.won; }
                if (enemyTeam) { enemyRounds = enemyTeam.rounds_won; }
            } else if (match.teams) {
                myTeam = match.teams[anchorTeamId.toLowerCase()];
                enemyTeam = match.teams[enemyTeamId.toLowerCase()];
                if (myTeam) { myRounds = myTeam.rounds_won; anchorWon = myTeam.has_won; }
                if (enemyTeam) { enemyRounds = enemyTeam.rounds_won; }
            }

            const result = anchorWon ? "win" : "loss";
            const stringResult = anchorWon ? "Victoire" : "Défaite";
            const map = meta.map;
            let myTeamName = "YOUR TEAM", enemyTeamName = "ENEMY TEAM"; 

            const maxScore = Math.max(...allPlayers.map(p => p.stats.score));
            let myTeamData = null;
            let enemyTeamData = null;

            if (Array.isArray(match.teams)) {
                myTeamData = match.teams.find(t => t.team_id === anchorTeamId);
                enemyTeamData = match.teams.find(t => t.team_id !== anchorTeamId);

                if (myTeamData) {
                    myRounds = myTeamData.rounds_won;
                    myTeamName = extractTeamName(myTeamData, myTeamName);
                }
                if (enemyTeamData) {
                    enemyRounds = enemyTeamData.rounds_won;
                    enemyTeamName = extractTeamName(enemyTeamData, enemyTeamName);
                }
            } else if (match.teams) {
                myTeamData = match.teams[anchorTeamId.toLowerCase()];
                enemyTeamData = match.teams[enemyTeamId.toLowerCase()];

                if (myTeamData) {
                    myRounds = myTeamData.rounds_won || 0;
                    myTeamName = extractTeamName(myTeamData, myTeamName);
                }
                if (enemyTeamData) {
                    enemyRounds = enemyTeamData.rounds_won || 0;
                    enemyTeamName = extractTeamName(enemyTeamData, enemyTeamName);
                }
            }

            // Extra stat availability (mirrors the roster match tab). Rank/agent come
            // from site.js helpers; ADR/FB/FD only show when the data backs them.
            const hasADR = allPlayers.some(p => typeof p.damage_made === 'number' && p.damage_made > 0);
            const fbByPuuid = {}, fdByPuuid = {};
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
            const headCells = ['Joueur', 'ACS'];
            if (hasADR) headCells.push('ADR');
            headCells.push('K', 'D', 'A', 'K/D');
            if (hasFBFD) headCells.push('FB', 'FD');
            headCells.push('HS%', 'Freddo');
            const colCount = headCells.length;
            const headerHTML = headCells.map(h => `<th>${h}</th>`).join('');

            const renderRows = (players, label) => {
                let html = `<tr><td colspan="${colCount}" class="team-divider">— ${label} —</td></tr>`;
                players.forEach(p => {
                    const s = p.stats;
                    const acs = Math.round(s.score / roundsPlayed);
                    const kd = (s.kills / Math.max(1, s.deaths)).toFixed(2);
                    const totalShots = s.headshots + s.bodyshots + s.legshots;
                    const hsPct = totalShots > 0 ? Math.round((s.headshots / totalShots) * 100) : 0;
                    const isMvp = s.score === maxScore;
                    const initials = p.name.substring(0, 2).toUpperCase();
                    const fs = calcMatchFreddoScore(s.kills, s.deaths, s.assists, s.score, s.headshots, s.bodyshots, s.legshots, roundsPlayed);
                    const ft = getFreddoTier(fs);

                    const agent = p.character || '—';
                    const aImg = agentImg(agent);
                    const avatar = aImg
                        ? `<img class="player-avatar agent-avatar" src="${aImg}" alt="${escapeHtml(agent)}" title="${escapeHtml(agent)}" loading="lazy">`
                        : `<div class="player-avatar">${escapeHtml(initials)}</div>`;
                    const adrCell = hasADR
                        ? `<td class="stat-adr">${roundsPlayed > 0 ? Math.round((p.damage_made || 0) / roundsPlayed) : 0}</td>`
                        : '';
                    const fbFdCells = hasFBFD
                        ? `<td class="stat-fb">${fbByPuuid[p.puuid] || 0}</td><td class="stat-fd">${fdByPuuid[p.puuid] || 0}</td>`
                        : '';

                    html += `
                    <tr>
                        <td>
                            <div class="player-name">
                                ${avatar}
                                ${escapeHtml(p.name)} ${isMvp ? '<span class="stat-mvp">★ MVP</span>' : ''}
                            </div>
                        </td>
                        <td class="stat-acs">${acs}</td>
                        ${adrCell}
                        <td>${s.kills}</td>
                        <td>${s.deaths}</td>
                        <td>${s.assists}</td>
                        <td class="${kd >= 1 ? 'stat-kd-pos' : 'stat-kd-neg'}">${kd}</td>
                        ${fbFdCells}
                        <td>${hsPct}%</td>
                        <td class="stat-fred-cell"><span class="fred-tier-badge" style="color:${ft.color}">${ft.tier}</span><span class="fred-score-num">${fs}</span></td>
                    </tr>`;
                });
                return html;
            };

            const scoreboardHtml = renderRows(myTeamPlayers, myTeamName) + renderRows(enemyTeamPlayers, enemyTeamName);

            const matchId = matchObj.match_id || (matchObj.metadata && matchObj.metadata.matchid) || '';
            listDiv.innerHTML += `
            <div class="match-card ${result}" data-result="${result}" data-teams="${myTeamName} ${enemyTeamName}" data-match-id="${matchId}" data-anchor-team="${anchorTeamId}" onclick="toggleCard(this)">
                <div class="match-summary">
                    <div class="match-date">${timeStr}<span>${dateStr}</span></div>
                    <div class="team-info"><img src="images/LogoTransparent.png" alt="" class="nav-logo-img" width="60" height="60" decoding="async"><div><div class="team-name">${myTeamName}</div><div class="team-tag">#FRED</div></div></div>
                    <div class="score-block">
                        <div class="score-main"><span class="s-win">${myTeam.rounds_won}</span><span class="s-dash">—</span><span class="s-loss">${enemyTeam.rounds_won}</span></div>
                        <div class="score-meta win">${stringResult}</div>
                        <div class="match-map-badge">${map}</div>
                    </div>
                    <div class="team-info right"><div style="text-align:right"><div class="team-name">${enemyTeamName}</div><div class="team-tag">#${enemyTeamData.roster.tag}</div></div></div>
                    <div class="match-format"><div class="format-tag">PREMIER</div></div>
                </div>
                <div class="match-details">
                    <div class="match-tabs">
                        <button class="match-tab active" onclick="event.stopPropagation();switchTab(this,'scoreboard')">Scoreboard</button>
                        <button class="match-tab" onclick="event.stopPropagation();switchTab(this,'rounds')">Rounds</button>
                    </div>
                    <div class="stats-panel tab-scoreboard active">
                        <table class="player-table">
                                <thead><tr>${headerHTML}</tr></thead>
                            <tbody>
                                ${scoreboardHtml}
                            </tbody>
                        </table>
                    </div>
                    <div class="stats-panel tab-rounds"></div>
                </div>
            </div>`;
        });

        // Temporary playoffs announcement — fill the bracket + season recap
        renderPlayoffBracket();
        renderSeasonRecap();
    } catch (e) {
        console.error(e);
        listDiv.innerHTML = `<div class="loader" style="color: red;">Error loading matches.</div>`;
    }
}

// ══════════════════════════════════════════════════════════════════════════
// TEMPORARY — PLAYOFFS CHAMPION ANNOUNCEMENT (bracket + season recap)
// Remove this block (and the markup/CSS) when the next season starts.
// ══════════════════════════════════════════════════════════════════════════

// Roster-based outcome detection, mirrors the logic used to render match cards.
function ann_analyzeMatch(matchObj) {
    const m = matchObj.match || matchObj;
    const meta = m.metadata || {};
    const roster = ["heri", "themistered", "graussbyt", "lal6s9gne", "hhj", "riboox"];
    const allP = m.players.all_players || m.players || [];

    let blue = 0, red = 0;
    allP.forEach(p => {
        const n = (p.name || '').toLowerCase();
        if (roster.some(r => n.includes(r))) {
            const t = p.team || p.team_id;
            if (t === 'Blue') blue++; else if (t === 'Red') red++;
        }
    });
    const myId = red > blue ? 'Red' : 'Blue';
    const enemyId = myId === 'Red' ? 'Blue' : 'Red';

    let myRounds = 0, enemyRounds = 0, won = false, enemyName = 'Adversaire';
    if (Array.isArray(m.teams)) {
        const mt = m.teams.find(t => t.team_id === myId);
        const et = m.teams.find(t => t.team_id === enemyId);
        if (mt) { myRounds = mt.rounds_won; won = mt.won; }
        if (et) { enemyRounds = et.rounds_won; enemyName = extractTeamName(et, enemyName); }
    } else if (m.teams) {
        const mt = m.teams[myId.toLowerCase()];
        const et = m.teams[enemyId.toLowerCase()];
        if (mt) { myRounds = mt.rounds_won; won = mt.has_won; }
        if (et) { enemyRounds = et.rounds_won; enemyName = extractTeamName(et, enemyName); }
    }
    return { won, myRounds, enemyRounds, enemyName, map: meta.map || '', date: new Date((meta.game_start || 0) * 1000) };
}

function renderPlayoffBracket() {
    const wrap = document.getElementById('playoffBracket');
    if (!wrap) return;
    const all = window.premierMatchesData || [];
    if (!all.length) { wrap.innerHTML = `<div class="po-empty">Parcours indisponible.</div>`; return; }

    // Most recent first in the data → take last 3 games, show oldest → newest (final last).
    const games = all.slice(0, 3).map(ann_analyzeMatch).reverse();
    const allLabels = ['Quarts de finale', 'Demi-finale', 'Finale'];
    const labels = allLabels.slice(allLabels.length - games.length);

    const nodes = games.map((g, i) => {
        const cls = g.won ? 'win' : 'loss';
        const fredWin = g.won ? 'po-winner' : '';
        const enemyWin = g.won ? '' : 'po-winner';
        return `
            <div class="po-node ${cls}">
                <div class="po-round">${labels[i] || 'Match'}</div>
                <div class="po-match">
                    <div class="po-team po-fred ${fredWin}">
                        <span class="po-tname">FRED</span>
                        <span class="po-tscore">${g.myRounds}</span>
                    </div>
                    <div class="po-team ${enemyWin}">
                        <span class="po-tname">${g.enemyName}</span>
                        <span class="po-tscore">${g.enemyRounds}</span>
                    </div>
                </div>
                <div class="po-result ${cls}">${g.won ? 'Victoire' : 'Défaite'}</div>
                ${g.map ? `<div class="po-map">${g.map}</div>` : ''}
            </div>`;
    });

    wrap.innerHTML = nodes.join(`<div class="po-connector">→</div>`) +
        `<div class="po-connector">→</div><div class="po-trophy-end">🏆<span>Titre</span></div>`;
}

function renderSeasonRecap() {
    const el = document.getElementById('seasonRecap');
    if (!el) return;
    const all = (window.premierMatchesData || []).map(ann_analyzeMatch);
    if (!all.length) { el.innerHTML = `<div class="sr-loading">Récap indisponible.</div>`; return; }

    const wins = all.filter(g => g.won).length;
    const losses = all.length - wins;
    const wr = Math.round(wins / all.length * 100);

    // Best map by win rate (min 1 game), from this season's games.
    const mapAgg = {};
    all.forEach(g => {
        if (!g.map) return;
        (mapAgg[g.map] = mapAgg[g.map] || { w: 0, t: 0 });
        mapAgg[g.map].t++; if (g.won) mapAgg[g.map].w++;
    });
    let bestMap = null;
    Object.entries(mapAgg).forEach(([name, s]) => {
        const rate = s.w / s.t;
        if (!bestMap || rate > bestMap.rate || (rate === bestMap.rate && s.t > bestMap.t)) {
            bestMap = { name, rate, w: s.w, t: s.t };
        }
    });

    // Longest win streak across the season (match order is consistent).
    let longestStreak = 0, run = 0;
    all.forEach(g => { run = g.won ? run + 1 : 0; if (run > longestStreak) longestStreak = run; });

    // Total rounds won across the season.
    const roundsWon = all.reduce((sum, g) => sum + (g.myRounds || 0), 0);

    const cards = [
        `<div class="sr-stat"><div class="sr-stat-val gold">CHAMPIONS</div><div class="sr-stat-lbl">Playoffs</div><div class="sr-stat-sub">Saison terminée</div></div>`,
        `<div class="sr-stat"><div class="sr-stat-val green">${wins}<span style="color:rgba(200,216,240,.25)">–</span><span style="color:var(--red)">${losses}</span></div><div class="sr-stat-lbl">Bilan</div><div class="sr-stat-sub">${all.length} matchs joués</div></div>`,
        `<div class="sr-stat"><div class="sr-stat-val cyan">${wr}%</div><div class="sr-stat-lbl">Taux de victoire</div><div class="sr-stat-sub">sur la saison</div></div>`,
    ];
    if (bestMap) {
        cards.push(`<div class="sr-stat"><div class="sr-stat-val">${bestMap.name}</div><div class="sr-stat-lbl">Meilleure map</div><div class="sr-stat-sub">${bestMap.w}V — ${bestMap.t - bestMap.w}D</div></div>`);
    }
    cards.push(`<div class="sr-stat"><div class="sr-stat-val gold">${longestStreak}</div><div class="sr-stat-lbl">Plus longue série</div><div class="sr-stat-sub">victoires d'affilée</div></div>`);
    cards.push(`<div class="sr-stat"><div class="sr-stat-val cyan">${roundsWon}</div><div class="sr-stat-lbl">Rounds gagnés</div><div class="sr-stat-sub">sur la saison</div></div>`);

    el.innerHTML = `
        <div class="sr-grid">${cards.join('')}</div>
        <div class="sr-note">Une saison qui se conclut par une victoire du bracket en Advanced 3&nbsp;: ${wins} victoires, un parcours en playoffs maîtrisé et un trophée au bout. Merci à toute la team FRED et ses supporters — rendez-vous la saison prochaine&nbsp;!</div>
    `;
}

function startCountdown() {
    const updateTimers = () => {
        const timers = document.querySelectorAll('.match-timer');
        
        timers.forEach(timer => {
            const targetDate = new Date(timer.getAttribute('data-time'));
            const now = new Date();
            const diff = targetDate - now;

            if (diff <= 0) {
                timer.innerHTML = "En cours ou terminé";
                return;
            }

            const days = Math.floor(diff / (1000 * 60 * 60 * 24));
            const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
            const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));

            let countdownText = "";
            if (days > 0) countdownText += `${days}j `;
            if (hours > 0 || days > 0) countdownText += `${hours}h `;
            countdownText += `${minutes}m`;

            timer.innerHTML = countdownText;
        });
    };

    updateTimers();
    setInterval(updateTimers, 60000);
}

// --- DASHBOARD STATS INTEGRATION ---

let dashboardPlayers = [];
let curStat = 'average_acs';
let selPlayer = null;
let tableSortStat = 'freddo_score';
let tableSortDir = -1;

const statFmt = {
    average_acs: v => Math.round(v),
    adr: v => Math.round(v),
    kd: v => v.toFixed(2),
    headshot_percentage: v => v + '%',
    first_bloods: v => v,
    first_deaths: v => v,
    kast: v => v + '%',
    clutches: v => v,
    freddo_score: v => v
};
const statLbl = {
    average_acs: 'ACS',
    adr: 'ADR',
    kd: 'K/D',
    headshot_percentage: 'HS%',
    first_bloods: 'First Bloods',
    first_deaths: 'First Deaths',
    kast: 'KAST%',
    clutches: 'Clutches',
    freddo_score: 'Freddo Score'
};
const statGood = { average_acs: 230, adr: 150, kd: 1.2, headshot_percentage: 30, first_bloods: 15, first_deaths: 5, kast: 70, clutches: 10, freddo_score: 65 };

async function loadDashboardStats() {
    try {
        // Adjust this URL to wherever your Go server serves dashboard_stats.json
        const res = await fetch(`${getApiBase()}/api/matches/stats`);
        if (!res.ok) return;
        const data = await res.json();

        // 1. Load Players — enrich with per-map rates + Freddo score
        dashboardPlayers = (data.player_stats || []).map(p => {
            const m = Math.max(1, p.matches);
            return {
                ...p,
                kills_per_map:        p.kills / m,
                assists_per_map:      p.assists / m,
                first_bloods_per_map: p.first_bloods / m,
                first_deaths_per_map: p.first_deaths / m,
                clutches_per_map:     p.clutches / m,
                freddo_score:         calcFreddoScore(p)
                // adr and win_rate come directly from the backend
            };
        });
        document.getElementById('playerStatsHeading').innerHTML = `Performances joueurs <span>Saison Actuelle</span>`;
        renderCards();

        // 2. Load Maps
        const mapsGrid = document.getElementById('mapsGrid');
        mapsGrid.innerHTML = '';
        if (data.team_stats && data.team_stats.map_win_rates) {
            for (const [mapName, stats] of Object.entries(data.team_stats.map_win_rates)) {
                const wrClass = stats.win_rate >= 60 ? 'high' : stats.win_rate < 40 ? 'low' : 'mid';
                const abbr = mapName.substring(0, 2).toUpperCase();
                
                mapsGrid.innerHTML += `
                <div class="map-card">
                    <div class="map-card-bg"><div class="map-geo">${abbr}</div><div class="map-name-overlay">${mapName}</div></div>
                    <div class="map-card-body">
                        <div class="map-wr-row"><div class="map-wr-pct ${wrClass}">${Math.round(stats.win_rate)}%</div><div class="map-record">${stats.wins}V — ${stats.losses}D</div></div>
                        <div class="map-bar-track"><div class="map-bar-fill ${wrClass}" style="width:${Math.max(4, stats.win_rate)}%"></div></div>
                    </div>
                </div>`;
            }
        }

        // 3. Load Performances
        const perfGrid = document.getElementById('perfGrid');
        perfGrid.innerHTML = '';
        if (data.performances) {
            const perfs = [
                { key: 'best_performance', title: '⭐ Meilleure performance', color: 'gold', valColor: 'var(--gold)' },
                { key: 'most_kills', title: '🔫 Plus de kills', color: 'green', valColor: 'var(--green)' },
                { key: 'worst_performance', title: '💀 Jour Sans', color: 'purple', valColor: 'var(--purple)' },
                { key: 'clutch_king', title: '⚡ Clutch king', color: 'cyan', valColor: 'var(--cyan)' }
            ];

            perfs.forEach(pData => {
                const perf = data.performances[pData.key];
                if (!perf || !perf.player_name) return;

                const initials = perf.player_name.substring(0, 2).toUpperCase();
                const kd = perf.deaths > 0 ? (perf.kills / perf.deaths).toFixed(2) : perf.kills.toFixed(2);
                const kdColor = kd >= 1.2 ? 'var(--green)' : kd < 1 ? 'var(--red)' : 'white';
                const outcomeClass = perf.match_outcome === 'Win' ? 'win' : 'loss';
                const outcomeText = perf.match_outcome === 'Win' ? '✓ VICTOIRE' : '✗ DÉFAITE';

                perfGrid.innerHTML += `
                <div class="perf-card">
                    <div class="perf-card-glow ${pData.color}"></div>
                    <div class="perf-badge ${pData.color}">${pData.title}</div>
                    <div class="perf-player">
                        <div class="perf-avatar ${pData.color}">${initials}</div>
                        <div><div class="perf-player-name">${perf.player_name.split('#')[0]}</div><div class="perf-player-meta">${perf.map}</div></div>
                    </div>
                    <div class="perf-stats">
                        <div class="perf-stat"><div class="perf-stat-val" style="color:${pData.valColor}">${Math.round(perf.acs)}</div><div class="perf-stat-lbl">ACS</div></div>
                        <div class="perf-stat"><div class="perf-stat-val">${perf.kills}/${perf.deaths}/${perf.assists}</div><div class="perf-stat-lbl">K/D/A</div></div>
                        <div class="perf-stat"><div class="perf-stat-val" style="color:${kdColor}">${kd}</div><div class="perf-stat-lbl">K/D</div></div>
                        <div class="perf-stat"><div class="perf-stat-val">${Math.round(perf.headshot_pct)}%</div><div class="perf-stat-lbl">HS%</div></div>
                    </div>
                    <div class="perf-context"><div class="perf-vs">vs <strong>${perf.opponent}</strong></div><div class="perf-result ${outcomeClass}">${outcomeText}</div></div>
                </div>`;
            });
        }
        renderDistinctions(dashboardPlayers);
        renderStatsTable(dashboardPlayers);
    } catch (e) {
        console.error("Failed to load dashboard stats:", e);
    }
}

// ══ NEW RENDER FUNCTIONS ══

function renderTeamOverviewBanner(results) {
    const el = document.getElementById('teamOverviewBanner');
    if (!el || !results.length) return;

    const wins = results.filter(r => r.won).length;
    const losses = results.filter(r => !r.won).length;
    const total = wins + losses;
    const winRate = total > 0 ? Math.round((wins / total) * 100) : 0;

    // results is sorted newest-first, so the current streak starts at index 0.
    let streakCount = 1;
    const streakWon = results[0].won;
    for (let i = 1; i < results.length; i++) {
        if (results[i].won === streakWon) streakCount++;
        else break;
    }
    const streakStr = (streakWon ? 'W' : 'L') + streakCount;
    const streakClass = streakWon ? 'cyan' : 'red';
    const winRateClass = winRate >= 60 ? 'green' : winRate < 40 ? 'red' : 'gold';

    // Take the 7 most recent (start of the list), kept newest → oldest left-to-right.
    const recent = results.slice(0, 7);
    const formDots = recent.map(r =>
        `<span class="form-dot ${r.won ? 'win' : 'loss'}" title="${r.map}">${r.won ? 'W' : 'L'}</span>`
    ).join('');

    el.innerHTML = `
        <div class="overview-pill">
            <div class="overview-pill-val green">${wins}</div>
            <div class="overview-pill-lbl">Victoires</div>
        </div>
        <div class="overview-pill">
            <div class="overview-pill-val red">${losses}</div>
            <div class="overview-pill-lbl">Défaites</div>
        </div>
        <div class="overview-pill">
            <div class="overview-pill-val ${winRateClass}">${winRate}%</div>
            <div class="overview-pill-lbl">Win Rate</div>
        </div>
        <div class="overview-pill">
            <div class="overview-pill-val">${total}</div>
            <div class="overview-pill-lbl">Matchs joués</div>
        </div>
        <div class="overview-pill overview-pill--form">
            <div class="overview-pill-lbl">Forme récente</div>
            <div class="form-strip">${formDots}</div>
        </div>`;
}


function renderDistinctions(players) {
    const el = document.getElementById('distinctionsGrid');
    if (!el || !players.length) return;
    const q = players.filter(p => p.matches >= 2);
    if (!q.length) return;

    // Each category has a scoring function + direction.
    // Players are assigned greedily in priority order — each player appears exactly once.
    const categories = [
        {
            score: p => p.average_acs, dir: 1, title: '⭐ Le Fragger', color: 'gold',
            sub:     p => `${Math.round(p.average_acs)} ACS moyen`,
            mainVal: p => Math.round(p.average_acs),  mainLbl: 'ACS moyen',
            stats:   p => [{v:p.kills,l:'Kills'},{v:p.kd.toFixed(2),l:'K/D'},{v:Math.round(p.headshot_percentage)+'%',l:'HS%'}],
            roast:   () => 'Les stats ne mentent pas. L\'équipe tourne autour de lui.'
        },
        {
            score: p => p.assists_per_map, dir: 1, title: '🤝 Le Passeur', color: 'cyan',
            sub:     p => `${p.assists_per_map.toFixed(2)} assists/map`,
            mainVal: p => p.assists_per_map.toFixed(2),  mainLbl: 'Assists / map',
            stats:   p => [{v:p.assists,l:'Total'},{v:p.kd.toFixed(2),l:'K/D'},{v:Math.round(p.kast)+'%',l:'KAST'}],
            roast:   () => 'Le vrai MVP silencieux. Chaque kill de l\'équipe lui doit quelque chose.'
        },
        {
            score: p => p.kast, dir: 1, title: '🛡️ Le Survivant', color: 'green',
            sub:     p => `${Math.round(p.kast)}% KAST`,
            mainVal: p => Math.round(p.kast)+'%',  mainLbl: 'KAST%',
            stats:   p => [{v:p.kills,l:'Kills'},{v:p.kd.toFixed(2),l:'K/D'},{v:p.first_deaths_per_map.toFixed(2),l:'FD/map'}],
            roast:   () => 'Présent dans chaque round. Il se bat jusqu\'au bout — souvent encore debout quand les autres sont au sol.'
        },
        {
            score: p => p.first_bloods_per_map, dir: 1, title: '💥 L\'Ouvreur', color: 'gold',
            sub:     p => `${p.first_bloods_per_map.toFixed(2)} FB/map`,
            mainVal: p => p.first_bloods_per_map.toFixed(2),  mainLbl: 'First Bloods / map',
            stats:   p => [{v:p.kd.toFixed(2),l:'K/D'},{v:Math.round(p.headshot_percentage)+'%',l:'HS%'},{v:Math.round(p.kast)+'%',l:'KAST'}],
            roast:   () => 'Il frappe en premier. L\'équipe n\'a qu\'à suivre. En général.'
        },
        {
            score: p => p.clutches_per_map, dir: 1, title: '⚡ Le Clutcheur', color: 'cyan',
            sub:     p => `${p.clutches_per_map.toFixed(2)} clutches/map`,
            mainVal: p => p.clutches_per_map.toFixed(2),  mainLbl: 'Clutches / map',
            stats:   p => [{v:p.clutches,l:'Total'},{v:p.kd.toFixed(2),l:'K/D'},{v:Math.round(p.kast)+'%',l:'KAST'}],
            roast:   () => 'Au fond du gouffre, avec une balle. Et il sort vainqueur.'
        },
        {
            score: p => p.first_bloods / Math.max(1, p.first_deaths), dir: 1, title: '👻 Le Fantôme', color: 'purple',
            sub:     p => `Ratio ${(p.first_bloods / Math.max(1, p.first_deaths)).toFixed(2)} FB/FD`,
            mainVal: p => (p.first_bloods / Math.max(1, p.first_deaths)).toFixed(2),  mainLbl: 'Ratio FB/FD',
            stats:   p => [{v:p.first_bloods,l:'First Bloods'},{v:p.first_deaths,l:'First Deaths'},{v:p.kd.toFixed(2),l:'K/D'}],
            roast:   () => 'Il ouvre les rounds ET il survit. En même temps. Insolent.'
        },
        {
            score: p => p.first_deaths_per_map, dir: 1, title: '🪦 La Première Victime', color: 'red',
            sub:     p => `${p.first_deaths_per_map.toFixed(2)} FD/map`,
            mainVal: p => p.first_deaths_per_map.toFixed(2),  mainLbl: 'First Deaths / map',
            stats:   p => [{v:p.kills,l:'Kills quand même'},{v:Math.round(p.headshot_percentage)+'%',l:'HS%'},{v:Math.round(p.kast)+'%',l:'KAST'}],
            roast:   p => '"Je repère les positions ennemies avec mon corps." — ' + p.name.split('#')[0]
        },
        {
            score: p => p.headshot_percentage, dir: -1, title: '🦵 L\'Ami des Tibias', color: 'purple',
            sub:     p => `${Math.round(p.headshot_percentage)}% HS`,
            mainVal: p => Math.round(p.headshot_percentage)+'%',  mainLbl: 'Headshot %',
            stats:   p => [{v:p.kills,l:'Kills'},{v:p.kd.toFixed(2),l:'K/D'},{v:p.first_bloods,l:'First Bloods'}],
            roast:   () => 'La tête c\'est surfait. Les tibias, les genoux, les épaules — tout est valable.'
        }
    ];

    const assigned = new Set();
    const assignments = [];
    for (const cat of categories) {
        const rem = q.filter(p => !assigned.has(p.name));
        if (!rem.length) break;
        const best = rem.reduce((b, p) => cat.dir === 1 ? (cat.score(p) > cat.score(b) ? p : b) : (cat.score(p) < cat.score(b) ? p : b));
        assigned.add(best.name);
        assignments.push({ cat, player: best });
    }

    el.innerHTML = assignments.map(({ cat, player }) => {
        const name = player.name.split('#')[0];
        const initials = name.substring(0, 2).toUpperCase();
        const statsHtml = cat.stats(player).map(s =>
            `<div class="dist-stat"><div class="dist-stat-val">${s.v}</div><div class="dist-stat-lbl">${s.l}</div></div>`
        ).join('');
        return `<div class="dist-card">
            <div class="dist-card-glow ${cat.color}"></div>
            <div class="dist-badge ${cat.color}">${cat.title}</div>
            <div class="dist-player">
                <div class="dist-avatar ${cat.color}">${initials}</div>
                <div><div class="dist-player-name">${name}</div><div class="dist-player-sub">${cat.sub(player)}</div></div>
            </div>
            <div class="dist-main-stat">
                <div class="dist-main-val">${cat.mainVal(player)}</div>
                <div class="dist-main-lbl">${cat.mainLbl}</div>
            </div>
            <div class="dist-stats">${statsHtml}</div>
            <div class="dist-roast">${cat.roast(player)}</div>
        </div>`;
    }).join('');
}

function renderStatsTable(players) {
    const el = document.getElementById('statsTableWrap');
    if (!el || !players.length) return;
    el.innerHTML = `<table class="stats-summary-table">
        <thead><tr>
            <th class="sth sth-name">Joueur</th>
            <th class="sth sth-sortable sth-active" onclick="setTableSort(this,'freddo_score')">Freddo <span class="sort-arrow">▼</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'average_acs')">ACS <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'adr')">ADR <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'kd')">K/D <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'headshot_percentage')">HS% <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'kills_per_map')">K/map <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'assists_per_map')">A/map <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'first_bloods_per_map')">FB/map <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'first_deaths_per_map')">FD/map <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'clutches_per_map')">Cl/map <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'kast')">KAST% <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth sth-sortable" onclick="setTableSort(this,'win_rate')">W% <span class="sort-arrow sort-idle">⇕</span></th>
            <th class="sth">Maps</th>
        </tr></thead>
        <tbody id="statsTableBody"></tbody>
    </table>`;
    renderTableBody(players);
}

function setTableSort(th, stat) {
    if (tableSortStat === stat) { tableSortDir *= -1; }
    else { tableSortStat = stat; tableSortDir = -1; }
    const table = th.closest('table');
    table.querySelectorAll('.sth-active').forEach(e => e.classList.remove('sth-active'));
    table.querySelectorAll('.sort-arrow').forEach(e => { e.textContent = '⇕'; e.classList.add('sort-idle'); });
    th.classList.add('sth-active');
    const arrow = th.querySelector('.sort-arrow');
    if (arrow) { arrow.textContent = tableSortDir === -1 ? '▼' : '▲'; arrow.classList.remove('sort-idle'); }
    renderTableBody(dashboardPlayers);
}

function renderTableBody(players) {
    const tbody = document.getElementById('statsTableBody');
    if (!tbody) return;
    const sorted = [...players].sort((a, b) =>
        tableSortDir === -1 ? b[tableSortStat] - a[tableSortStat] : a[tableSortStat] - b[tableSortStat]
    );
    tbody.innerHTML = sorted.map(p => {
        const name = p.name.split('#')[0];
        const initials = name.substring(0, 2).toUpperCase();
        const ft    = getFreddoTier(p.freddo_score);
        const adrC  = (p.adr || 0) >= statGood.adr ? 'sst-gold' : '';
        const acsC  = p.average_acs >= statGood.average_acs ? 'sst-gold' : '';
        const kdC   = p.kd >= 1.2 ? 'sst-green' : p.kd < 1 ? 'sst-red' : '';
        const hsC   = p.headshot_percentage >= statGood.headshot_percentage ? 'sst-green' : '';
        const fbC   = p.first_bloods_per_map >= 1 ? 'sst-green' : '';
        const fdC   = p.first_deaths_per_map >= 1.5 ? 'sst-red' : '';
        const clC   = p.clutches_per_map >= 0.3 ? 'sst-gold' : '';
        const kastC = p.kast >= statGood.kast ? 'sst-green' : '';
        const wrC   = (p.win_rate || 0) >= 60 ? 'sst-green' : (p.win_rate || 0) < 40 ? 'sst-red' : '';
        return `<tr class="sst-row">
            <td><div class="sst-player"><div class="sst-avatar">${initials}</div><span class="sst-name">${name}</span></div></td>
            <td class="sst-val sst-fred-cell"><span class="fred-tier-badge sst-fred-badge" style="color:${ft.color}">${ft.tier}</span><span class="fred-score-num">${p.freddo_score}</span></td>
            <td class="sst-val ${acsC}">${Math.round(p.average_acs)}</td>
            <td class="sst-val ${adrC}">${Math.round(p.adr || 0)}</td>
            <td class="sst-val ${kdC}">${p.kd.toFixed(2)}</td>
            <td class="sst-val ${hsC}">${Math.round(p.headshot_percentage)}%</td>
            <td class="sst-val">${p.kills_per_map.toFixed(1)}</td>
            <td class="sst-val sst-cyan">${p.assists_per_map.toFixed(1)}</td>
            <td class="sst-val ${fbC}">${p.first_bloods_per_map.toFixed(2)}</td>
            <td class="sst-val ${fdC}">${p.first_deaths_per_map.toFixed(2)}</td>
            <td class="sst-val ${clC}">${p.clutches_per_map.toFixed(2)}</td>
            <td class="sst-val ${kastC}">${Math.round(p.kast)}%</td>
            <td class="sst-val ${wrC}">${Math.round(p.win_rate || 0)}%</td>
            <td class="sst-val sst-dim">${p.matches}</td>
        </tr>`;
    }).join('');
}

// UI functions for Player Stats
function setStat(btn, stat) {
    curStat = stat;
    document.querySelectorAll('.stat-pill').forEach(p => p.classList.remove('active'));
    btn.classList.add('active');
    renderCards();
}

function renderCards() {
    if (!dashboardPlayers.length) return;
    const sorted = [...dashboardPlayers].sort((a, b) => b[curStat] - a[curStat]);

    // Stat podium — top 3 respond to the toggle
    const podiumEl = document.getElementById('statPodium');
    if (podiumEl) {
        const top3 = sorted.slice(0, 3);
        const makeSlot = (p, rank) => {
            if (!p) return '';
            const name = p.name.split('#')[0];
            const initials = name.substring(0, 2).toUpperCase();
            const colorKey = ['gold', 'silver', 'bronze'][rank - 1];
            const rankKey  = ['first', 'second', 'third'][rank - 1];
            const crown = rank === 1 ? '<div class="podium-crown">♛</div>' : '';
            return `<div class="podium-slot podium-slot--${rankKey}">
                ${crown}
                <div class="podium-avatar podium-avatar--${colorKey}">${initials}</div>
                <div class="podium-name">${name}</div>
                <div class="podium-kills">${statFmt[curStat](p[curStat])}</div>
                <div class="podium-kills-lbl">${statLbl[curStat]}</div>
                <div class="podium-rank-bar podium-rank-bar--${rankKey}"><span class="podium-rank-num">${rank}</span></div>
            </div>`;
        };
        podiumEl.innerHTML = makeSlot(top3[1], 2) + makeSlot(top3[0], 1) + makeSlot(top3[2], 3);
    }

    document.getElementById('playerCards').innerHTML = sorted.map((p, i) => {
        const val = p[curStat];
        const nameDisplay = p.name.split('#')[0];
        const initials = nameDisplay.substring(0, 2).toUpperCase();
        const isFred = curStat === 'freddo_score';
        const ft = isFred ? getFreddoTier(val) : null;
        const colorClass = isFred ? '' : (val >= statGood[curStat] ? 'good' : 'warn');
        const mainStatHtml = isFred
            ? `<div class="psc-main-stat psc-fred-tier" style="color:${ft.color}">${ft.tier}</div>
               <div class="psc-stat-label">${val} pts · Freddo</div>`
            : `<div class="psc-main-stat ${colorClass}">${statFmt[curStat](val)}</div>
               <div class="psc-stat-label">${statLbl[curStat]}</div>`;
        return `<div class="player-stat-card${selPlayer === p.name ? ' selected' : ''}" onclick="selectPlayer('${p.name}')">
            ${i === 0 ? '<div class="psc-rank">#1</div>' : ''}
            <div class="psc-avatar">${initials}</div>
            <div class="psc-name">${nameDisplay}</div>
            <div class="psc-role">${p.matches} maps</div>
            ${mainStatHtml}
        </div>`;
    }).join('');
}

function selectPlayer(name) {
    if (selPlayer === name) { 
        selPlayer = null; 
        document.getElementById('playerDetail').classList.remove('visible'); 
        renderCards(); 
        return; 
    }
    selPlayer = name;
    renderCards();
    
    const p = dashboardPlayers.find(x => x.name === name);
    if (!p) return;

    const kdClass = p.kd >= 1.2 ? 'good' : p.kd < 1 ? 'bad' : 'warn';
    const nameDisplay = p.name.split('#')[0];
    const initials = nameDisplay.substring(0, 2).toUpperCase();
    
    let agentRows = "";
    if (p.top_agents && p.top_agents.length > 0) {
        agentRows = p.top_agents.map(a => {
            const pct = Math.round((a.matches / p.matches) * 100);
            return `<div class="agent-row">
                <div class="agent-name">${a.agent_name}</div>
                <div class="agent-games">${a.matches} picks</div>
                <div class="agent-bar-track"><div class="agent-bar-fill" style="width:${pct}%; background:var(--blue)"></div></div>
                <div class="agent-wr">${pct}% pickrate</div>
            </div>`;
        }).join('');
    } else {
        agentRows = `<div style="color:#aaa; font-size:0.9rem; margin-top:1rem;">Pas de données d'agent.</div>`;
    }

    let weaponRows = "";
    if (p.top_weapons && p.top_weapons.length > 0) {
        const totalWeaponKills = p.top_weapons.reduce((s, w) => s + w.kills, 0);
        const denominator = Math.max(1, p.kills);
        weaponRows = p.top_weapons.map(w => {
            const pct = Math.round((w.kills / denominator) * 100);
            const barPct = Math.round((w.kills / Math.max(1, p.top_weapons[0].kills)) * 100);
            return `<div class="agent-row">
                <div class="agent-name">${w.weapon}</div>
                <div class="agent-games">${w.kills} kills</div>
                <div class="agent-bar-track"><div class="agent-bar-fill" style="width:${barPct}%; background:var(--gold)"></div></div>
                <div class="agent-wr">${pct}%</div>
            </div>`;
        }).join('');
    } else {
        weaponRows = `<div style="color:#aaa; font-size:0.9rem; margin-top:1rem;">Pas de données d'arme.</div>`;
    }

    const fs = p.freddo_score || 0;
    const ft = getFreddoTier(fs);
    const breakdown = calcFreddoBreakdown(p);
    const breakdownHtml = breakdown.map(c => {
        const pct = Math.min((c.pts / c.max) * 100, 100);
        const sign = c.sign === -1 ? '−' : '+';
        const colorVar = c.sign === -1 ? 'var(--red)' : (pct >= 75 ? 'var(--green)' : pct >= 40 ? 'var(--gold)' : 'var(--cyan)');
        return `<div class="fbd-row">
            <div class="fbd-label">${c.label}</div>
            <div class="fbd-raw">${c.raw}</div>
            <div class="fbd-bar-track"><div class="fbd-bar-fill" style="width:${pct}%;background:${colorVar}"></div></div>
            <div class="fbd-pts" style="color:${colorVar}">${sign}${c.pts.toFixed(1)}<span class="fbd-max">/${c.max}</span></div>
        </div>`;
    }).join('');

    document.getElementById('playerDetail').innerHTML = `
        <div class="pdp-header">
            <div class="pdp-avatar-lg">${initials}</div>
            <div>
                <div class="pdp-name">${nameDisplay}</div>
                <div class="pdp-sub">${p.matches} maps jouées</div>
            </div>
            <div class="pdp-fred-score">
                <div class="pdp-fred-tier" style="color:${ft.color}">${ft.tier}</div>
                <div class="pdp-fred-pts">${fs} pts</div>
                <div class="pdp-fred-lbl">Freddo Score</div>
            </div>
        </div>
        <div class="pdp-stats-row">
            <div class="pdp-stat"><div class="pdp-stat-val warn">${Math.round(p.average_acs)}</div><div class="pdp-stat-lbl">ACS moyen</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val" style="color:var(--gold)">${Math.round(p.adr || 0)}</div><div class="pdp-stat-lbl">ADR</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val ${kdClass}">${p.kd.toFixed(2)}</div><div class="pdp-stat-lbl">K/D ratio</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val">${Math.round(p.headshot_percentage)}%</div><div class="pdp-stat-lbl">HS%</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val">${(p.kills_per_map||0).toFixed(1)}</div><div class="pdp-stat-lbl">K/map</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val sst-cyan">${(p.assists_per_map||0).toFixed(1)}</div><div class="pdp-stat-lbl">A/map</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val good">${(p.first_bloods_per_map||0).toFixed(2)}</div><div class="pdp-stat-lbl">FB/map</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val good">${(p.clutches_per_map||0).toFixed(2)}</div><div class="pdp-stat-lbl">Cl/map</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val">${Math.round(p.kast)}%</div><div class="pdp-stat-lbl">KAST%</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val" style="color:${(p.win_rate||0)>=60?'var(--green)':(p.win_rate||0)<40?'var(--red)':'var(--gold)'}">${Math.round(p.win_rate||0)}%</div><div class="pdp-stat-lbl">Win Rate</div></div>
        </div>
        <div class="pdp-freddo-breakdown">
            <div class="pdp-freddo-breakdown-title">Calcul du Freddo Score</div>
            <div class="fbd-rows">${breakdownHtml}</div>
            <div class="fbd-total">
                <span>Total</span>
                <span class="fbd-total-val" style="color:${ft.color}">${fs} <span style="opacity:.45;font-size:.8em">/ 100</span></span>
            </div>
        </div>
        <div class="pdp-agents">
            <div class="pdp-agents-title">Top 3 Agents</div>
            <div class="agent-rows">${agentRows}</div>
        </div>
        <div class="pdp-agents">
            <div class="pdp-agents-title">Top Armes</div>
            <div class="agent-rows">${weaponRows}</div>
        </div>`;
        
    document.getElementById('playerDetail').classList.add('visible');
    document.getElementById('playerDetail').scrollIntoView({behavior:'smooth',block:'nearest'});
}

// Hook into existing initialization
const originalLoadMatches = window.onload || document.addEventListener;
document.addEventListener('DOMContentLoaded', () => {
    // loadPremierMatches() is already hooked in premier2.js
    loadDashboardStats();
});

function toggleCard(card) { card.classList.toggle('expanded'); }

function switchTab(btn, tabName) {
    const card = btn.closest('.match-card');
    card.querySelectorAll('.match-tab').forEach(t => t.classList.remove('active'));
    btn.classList.add('active');
    card.querySelectorAll('.stats-panel').forEach(p => p.classList.remove('active'));
    card.querySelector('.tab-' + tabName).classList.add('active');

    if (tabName === 'rounds') {
        const panel = card.querySelector('.tab-rounds');
        if (!panel.dataset.loaded) {
            panel.dataset.loaded = '1';
            loadRecap(panel, card.dataset.matchId, card.dataset.anchorTeam);
        }
    }
}

const endTypeLabel = {
    'Eliminated':    'Élim',
    'Bomb detonated':'Bombe',
    'Bomb defused':  'Désamorcé',
    'Round_Timeout': 'Temps',
};

async function loadRecap(panel, matchId, anchorTeam) {
    if (!matchId) { panel.innerHTML = `<div class="recap-error">ID de match manquant.</div>`; return; }
    panel.innerHTML = `<div class="recap-loading">Chargement des rounds…</div>`;
    try {
        const res = await fetch(`${getApiBase()}/api/matches/premier/${matchId}/rounds`);
        if (!res.ok) { panel.innerHTML = `<div class="recap-error">Recap non disponible.</div>`; return; }
        const data = await res.json();
        renderRoundRecap(panel, data.rounds || [], anchorTeam);
    } catch(e) {
        panel.innerHTML = `<div class="recap-error">Erreur de chargement.</div>`;
    }
}

function renderRoundRecap(panel, rounds, anchorTeam) {
    if (!rounds.length) { panel.innerHTML = `<div class="recap-error">Aucun round disponible.</div>`; return; }

    const halfSize = Math.ceil(rounds.length / 2);
    let html = '<div class="recap-wrap">';

    rounds.forEach((r, i) => {
        if (i === 0) html += `<div class="recap-half-label">1ère mi-temps</div>`;
        if (i === halfSize) html += `<div class="recap-half-label">2ème mi-temps</div>`;

        const isWin = r.winner === anchorTeam;
        const outcomeClass = isWin ? 'win' : 'loss';
        const outcomeLabel = isWin ? 'V' : 'D';
        const endLbl = endTypeLabel[r.end] || r.end || '?';
        const plantedIcon = r.planted ? ' <span class="recap-planted">💣</span>' : '';

        const killsHtml = (r.kills || []).map(k => {
            const isOurKill = k.kt === anchorTeam;
            const killClass = isOurKill ? 'our-kill' : 'enemy-kill';
            const weapon = k.w !== 'Ability' ? `<span class="recap-weapon">${k.w}</span>` : '<span class="recap-weapon ability">Ability</span>';
            return `<span class="recap-kill ${killClass}">${k.k} ${weapon}→ ${k.v}</span>`;
        }).join('');

        html += `<div class="recap-round ${outcomeClass}">
            <div class="recap-round-hd">
                <span class="recap-rnum">R${String(r.n).padStart(2,'0')}</span>
                <span class="recap-outcome ${outcomeClass}">${outcomeLabel}</span>
                <span class="recap-endtype">${endLbl}${plantedIcon}</span>
            </div>
            <div class="recap-kills">${killsHtml || '<span class="recap-no-kills">—</span>'}</div>
        </div>`;
    });

    html += '</div>';
    panel.innerHTML = html;
}

function setFilter(btn, f) {
    curFilter = f;
    document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
    btn.classList.add('active'); applyFilters();
}
function filterSearch() { applyFilters(); }
function applyFilters() {
    const q = document.getElementById('searchInput').value.toLowerCase();
    document.querySelectorAll('.match-card').forEach(c => {
        const ok = (curFilter==='all'||c.dataset.result===curFilter) && (!q||(c.dataset.teams||'').toLowerCase().includes(q));
        c.style.display = ok ? '' : 'none';
    });
}

document.addEventListener('DOMContentLoaded', loadPremierMatches);
