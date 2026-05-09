// Change this if you ever change your primary tracker in the Go backend!
const ANCHOR_PLAYER = "heri"; 

window.premierMatchesData = [];

// NEW HELPER: Aggressively hunts down the team name no matter where the API hides it
function extractTeamName(teamData, fallback) {
    if (!teamData) return fallback;
    if (teamData.roster && teamData.roster.name) return teamData.roster.name;
    if (teamData.customization && teamData.customization.name) return teamData.customization.name;
    if (teamData.name) return teamData.name;
    return fallback;
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
        
        // 1. Fetch upcoming matches from your new JSON file
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
            </div>
`;
            });
        }
        startCountdown();

    listDiv.innerHTML += `
            <div class="season-heading" style="padding:1.5rem 0 .75rem">Résultats récents</div>
        `
        // Render the left-side list
        data.data.forEach((matchObj, index) => {
            const match = matchObj.match || matchObj; 
            const meta = match.metadata;
            
            const date = new Date(meta.game_start * 1000);
            const dateStr = date.toLocaleDateString();
            const timeStr = date.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'});

            // --- DYNAMIC TEAM DETECTION ---
            let myRounds = 0, enemyRounds = 0;
            let anchorWon = false;
            let anchorTeamId = "Red";
            const roundsPlayed = meta.rounds_played;

            // Find which team our Anchor Player was on
            const allPlayers = match.players.all_players || match.players;
            const anchor = allPlayers.find(p => p.name.toLowerCase() === ANCHOR_PLAYER.toLowerCase());
            if (anchor) anchorTeamId = anchor.team || anchor.team_id;
            let myTeamPlayers = allPlayers.filter(p => (p.team || p.team_id) === anchorTeamId);
            let enemyTeamPlayers = allPlayers.filter(p => (p.team || p.team_id) !== anchorTeamId);
            if (enemyTeamPlayers.length > 0) enemyTeamId = enemyTeamPlayers[0].team || enemyTeamPlayers[0].team_id;

            const sortByScore = (a, b) => b.stats.score - a.stats.score;
            myTeamPlayers.sort(sortByScore);
            enemyTeamPlayers.sort(sortByScore);

            let myTeam = null;
            let enemyTeam = null;
            // Extract the scores based on the Anchor's team
            if (Array.isArray(match.teams)) {
                myTeam = match.teams.find(t => t.team_id === anchorTeamId);
                enemyTeam = match.teams.find(t => t.team_id !== anchorTeamId);
                if (myTeam) { myRounds = myTeam.rounds_won; anchorWon = myTeam.won; }
                if (enemyTeam) { enemyRounds = enemyTeam.rounds_won; }
            } else if (match.teams) {
                myTeam = match.teams[anchorTeamId.toLowerCase()];
                const enemyTeamId = anchorTeamId.toLowerCase() === 'red' ? 'blue' : 'red';
                enemyTeam = match.teams[enemyTeamId];
                if (myTeam) { myRounds = myTeam.rounds_won; anchorWon = myTeam.has_won; }
                if (enemyTeam) { enemyRounds = enemyTeam.rounds_won; }
            }

            const result = anchorWon ? "win" : "loss"
            const encounterTeamScore = anchorWon ? "1" : "0"
            const encounterEnemyScore = anchorWon ? "0" : "1"
            const stringResult = anchorWon ? "Victoire" : "Défaite"
            const map = meta.map
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

            const renderRows = (players, label) => {
                let html = `<tr><td colspan="10" class="team-divider">— ${label} —</td></tr>`;
                players.forEach(p => {
                    const s = p.stats;
                    const acs = Math.round(s.score / roundsPlayed);
                    const kd = (s.kills / Math.max(1, s.deaths)).toFixed(2);
                    const totalShots = s.headshots + s.bodyshots + s.legshots;
                    const hsPct = totalShots > 0 ? Math.round((s.headshots / totalShots) * 100) : 0;
                    const isMvp = s.score === maxScore;
                    const initials = p.name.substring(0, 2).toUpperCase();

                    html += `
                    <tr>
                        <td>
                            <div class="player-name">
                                <div class="player-avatar">${initials}</div>
                                ${p.name} ${isMvp ? '<span class="stat-mvp">★ MVP</span>' : ''}
                            </div>
                        </td>
                        <td><span class="agent-badge">${p.character}</span></td>
                        <td class="stat-acs">${acs}</td>
                        <td>${s.kills}</td>
                        <td>${s.deaths}</td>
                        <td>${s.assists}</td>
                        <td class="${kd >= 1 ? 'stat-kd-pos' : 'stat-kd-neg'}">${kd}</td>
                        <td>${hsPct}%</td>
                    </tr>`;
                });
                return html;
            };

            const scoreboardHtml = renderRows(myTeamPlayers, myTeamName) + renderRows(enemyTeamPlayers, enemyTeamName);

                        listDiv.innerHTML += `
            <div class="match-card ${result}" data-result="${result}" data-teams="${myTeamName} ${enemyTeamName}" onclick="toggleCard(this)">
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
                    <div class="stats-panel active" id="m1a">
                        <table class="player-table">
                                <thead><tr><th>Joueur</th><th>Agent</th><th>ACS</th><th>K</th><th>D</th><th>A</th><th>K/D</th><th>HS%</th></tr></thead>
                            <tbody>
                                ${scoreboardHtml}
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>

            `;
        })
    } catch (e) {
        console.error(e);
        listDiv.innerHTML = `<div class="loader" style="color: red;">Error loading matches.</div>`;
    }
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

    // Run immediately and then every minute
    updateTimers();
    setInterval(updateTimers, 60000);
}

// --- DASHBOARD STATS INTEGRATION ---

let dashboardPlayers = [];
let curStat = 'average_acs';
let selPlayer = null;

const statFmt = { 
    average_acs: v => Math.round(v), 
    kd: v => v.toFixed(2), 
    headshot_percentage: v => v + '%', 
    first_bloods: v => v, 
    first_deaths: v => v,
    kast: v => v + '%',
    clutches: v => v 
};
const statLbl = { 
    average_acs: 'ACS', 
    kd: 'K/D', 
    headshot_percentage: 'HS%', 
    first_bloods: 'First Bloods',
    first_deaths: 'First Deaths',
    kast: 'KAST%',
    clutches: 'Clutches' 
};
const statGood = { average_acs: 230, kd: 1.2, headshot_percentage: 30, first_bloods: 15, first_deaths: 5, kast: 70, clutches: 10 };

async function loadDashboardStats() {
    try {
        // Adjust this URL to wherever your Go server serves dashboard_stats.json
        const res = await fetch(`${getApiBase()}/api/matches/stats`);
        if (!res.ok) return;
        const data = await res.json();

        // 1. Load Players
        dashboardPlayers = data.player_stats || [];
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
    } catch (e) {
        console.error("Failed to load dashboard stats:", e);
    }
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
    
    // Sort based on current selected stat
    const sorted = [...dashboardPlayers].sort((a,b) => b[curStat] - a[curStat]);
    
    document.getElementById('playerCards').innerHTML = sorted.map((p, i) => {
        const val = p[curStat];
        const colorClass = val >= statGood[curStat] ? 'good' : 'warn';
        const nameDisplay = p.name.split('#')[0];
        const initials = nameDisplay.substring(0, 2).toUpperCase();

        return `<div class="player-stat-card${selPlayer === p.name ? ' selected' : ''}" onclick="selectPlayer('${p.name}')">
            ${i === 0 ? '<div class="psc-rank">#1</div>' : ''}
            <div class="psc-avatar">${initials}</div>
            <div class="psc-name">${nameDisplay}</div>
            <div class="psc-role">${p.matches} maps</div>
            <div class="psc-main-stat ${colorClass}">${statFmt[curStat](val)}</div>
            <div class="psc-stat-label">${statLbl[curStat]}</div>
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

    document.getElementById('playerDetail').innerHTML = `
        <div class="pdp-header">
            <div class="pdp-avatar-lg">${initials}</div>
            <div><div class="pdp-name">${nameDisplay}</div><div class="pdp-sub">${p.matches} maps jouées</div></div>
        </div>
        <div class="pdp-stats-row">
            <div class="pdp-stat"><div class="pdp-stat-val warn">${Math.round(p.average_acs)}</div><div class="pdp-stat-lbl">ACS moyen</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val ${kdClass}">${p.kd.toFixed(2)}</div><div class="pdp-stat-lbl">K/D ratio</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val">${Math.round(p.headshot_percentage)}%</div><div class="pdp-stat-lbl">Headshot%</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val">${p.kills}</div><div class="pdp-stat-lbl">Kills</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val">${p.assists}</div><div class="pdp-stat-lbl">Assists</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val good">${p.first_bloods}</div><div class="pdp-stat-lbl">First Bloods</div></div>
            <div class="pdp-stat"><div class="pdp-stat-val good">${p.clutches}</div><div class="pdp-stat-lbl">Clutches</div></div>
        </div>
        <div class="pdp-agents">
            <div class="pdp-agents-title">Top 3 Agents</div>
            <div class="agent-rows">${agentRows}</div>
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
