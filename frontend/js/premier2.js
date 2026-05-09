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
                </div>`;
            });
        }

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

document.addEventListener('DOMContentLoaded', loadPremierMatches);
