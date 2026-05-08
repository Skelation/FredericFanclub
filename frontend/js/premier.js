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

        // Render the left-side list
        data.data.forEach((matchObj, index) => {
            const match = matchObj.match || matchObj; 
            const meta = match.metadata;
            
            const date = new Date(meta.game_start * 1000);
            const dateStr = date.toLocaleDateString() + " " + date.toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'});
            
            // --- DYNAMIC TEAM DETECTION ---
            let myRounds = 0, enemyRounds = 0;
            let anchorWon = false;
            let anchorTeamId = "Red";

            // Find which team our Anchor Player was on
            const allPlayers = match.players.all_players || match.players;
            const anchor = allPlayers.find(p => p.name.toLowerCase() === ANCHOR_PLAYER.toLowerCase());
            if (anchor) anchorTeamId = anchor.team || anchor.team_id;

            // Extract the scores based on the Anchor's team
            if (Array.isArray(match.teams)) {
                const myTeam = match.teams.find(t => t.team_id === anchorTeamId);
                const enemyTeam = match.teams.find(t => t.team_id !== anchorTeamId);
                if (myTeam) { myRounds = myTeam.rounds_won; anchorWon = myTeam.won; }
                if (enemyTeam) { enemyRounds = enemyTeam.rounds_won; }
            } else if (match.teams) {
                const myTeam = match.teams[anchorTeamId.toLowerCase()];
                const enemyTeamId = anchorTeamId.toLowerCase() === 'red' ? 'blue' : 'red';
                const enemyTeam = match.teams[enemyTeamId];
                if (myTeam) { myRounds = myTeam.rounds_won; anchorWon = myTeam.has_won; }
                if (enemyTeam) { enemyRounds = enemyTeam.rounds_won; }
            }

            const scoreText = `${myRounds} - ${enemyRounds}`;
            const borderColor = anchorWon ? "#00ff64" : "#ff4655";
            // -------------------------------

            listDiv.innerHTML += `
                <div class="match-card" id="match-card-${meta.matchid}" style="border-left-color: ${borderColor}" onclick="viewMatchDetails('${meta.matchid}')">
                    <div style="font-weight: bold; font-size: 1.1rem; margin-bottom: 5px;">${meta.map}</div>
                    <div style="display: flex; justify-content: space-between; font-size: 0.9rem; color: #aaa;">
                        <span style="color: white; font-weight: bold;">${scoreText}</span>
                        <span>${dateStr}</span>
                    </div>
                </div>
            `;

            if (index === 0) viewMatchDetails(meta.matchid);
        });

    } catch (e) {
        console.error(e);
        listDiv.innerHTML = `<div class="loader" style="color: red;">Error loading matches.</div>`;
    }
}

function viewMatchDetails(matchId) {
    document.querySelectorAll('.match-card').forEach(c => c.classList.remove('active'));
    document.getElementById(`match-card-${matchId}`).classList.add('active');

    const matchObj = window.premierMatchesData.find(m => (m.match ? m.match.metadata.matchid : m.metadata.matchid) === matchId);
    if (!matchObj) return;
    
    const match = matchObj.match || matchObj;
    const meta = match.metadata;
    const panel = document.getElementById('scoreboardPanel');
    panel.style.display = "block";

    // --- SEPARATE PLAYERS BY TEAM ---
    let anchorTeamId = "Red";
    let enemyTeamId = "Blue";
    
    const allPlayers = match.players.all_players || match.players;
    const anchor = allPlayers.find(p => p.name.toLowerCase() === ANCHOR_PLAYER.toLowerCase());
    if (anchor) anchorTeamId = anchor.team || anchor.team_id;

    let myTeamPlayers = allPlayers.filter(p => (p.team || p.team_id) === anchorTeamId);
    let enemyTeamPlayers = allPlayers.filter(p => (p.team || p.team_id) !== anchorTeamId);

    if (enemyTeamPlayers.length > 0) enemyTeamId = enemyTeamPlayers[0].team || enemyTeamPlayers[0].team_id;

    const sortByScore = (a, b) => b.stats.score - a.stats.score;
    myTeamPlayers.sort(sortByScore);
    enemyTeamPlayers.sort(sortByScore);

    // --- EXTRACT ACTUAL PREMIER TEAM NAMES & SCORES ---
    let myRounds = 0, enemyRounds = 0;
    let myTeamName = "YOUR TEAM", enemyTeamName = "ENEMY TEAM"; 

    if (Array.isArray(match.teams)) {
        const myTeamData = match.teams.find(t => t.team_id === anchorTeamId);
        const enemyTeamData = match.teams.find(t => t.team_id !== anchorTeamId);
        
        if (myTeamData) {
            myRounds = myTeamData.rounds_won;
            myTeamName = extractTeamName(myTeamData, myTeamName);
        }
        if (enemyTeamData) {
            enemyRounds = enemyTeamData.rounds_won;
            enemyTeamName = extractTeamName(enemyTeamData, enemyTeamName);
        }
    } else if (match.teams) {
        const myTeamData = match.teams[anchorTeamId.toLowerCase()];
        const enemyTeamData = match.teams[enemyTeamId.toLowerCase()];
        
        if (myTeamData) {
            myRounds = myTeamData.rounds_won || 0;
            myTeamName = extractTeamName(myTeamData, myTeamName);
        }
        if (enemyTeamData) {
            enemyRounds = enemyTeamData.rounds_won || 0;
            enemyTeamName = extractTeamName(enemyTeamData, enemyTeamName);
        }
    }

    const totalRounds = myRounds + enemyRounds || 1; 

    // --- DETERMINE WINNERS AND LOSERS ---
    let myTeamClass = myRounds >= enemyRounds ? "team-win" : "team-loss";
    let enemyTeamClass = enemyRounds > myRounds ? "team-win" : "team-loss";

    let myScoreColor = myRounds >= enemyRounds ? "#00ff64" : "#ff4655";
    let enemyScoreColor = enemyRounds > myRounds ? "#00ff64" : "#ff4655";

    // --- BUILD THE SCOREBOARD UI ---
    panel.innerHTML = `
        <div class="sb-header">
            <div>
                <h2 style="margin: 0; color: white; text-transform: uppercase;">${meta.map}</h2>
                <div style="color: #aaa;">Premier • ${meta.game_length ? Math.floor(meta.game_length/60) : "??"} Mins • ${meta.cluster}</div>
            </div>
            <div class="sb-score">
                <span style="color: ${myScoreColor};">${myRounds}</span> 
                <span style="color: #444;">-</span> 
                <span style="color: ${enemyScoreColor};">${enemyRounds}</span>
            </div>
        </div>

        ${buildTeamTable(myTeamName.toUpperCase(), myTeamPlayers, myTeamClass, totalRounds)}
        ${buildTeamTable(enemyTeamName.toUpperCase(), enemyTeamPlayers, enemyTeamClass, totalRounds)}
    `;
}

function buildTeamTable(teamName, players, cssClass, totalRounds) {
    if (!players || players.length === 0) return "";

    let rows = "";
    players.forEach(p => {
        const stats = p.stats;
        const acs = Math.round(stats.score / totalRounds);
        const kd = stats.deaths === 0 ? stats.kills : (stats.kills / stats.deaths).toFixed(2);
        const kdColor = kd >= 1 ? "#00ff64" : (kd < 0.8 ? "#ff4655" : "white");

        rows += `
            <tr>
                <td>
                    <img src="${p.assets.agent.small}" class="agent-icon" alt="${p.character}">
                    <strong style="color: white; font-size: 1.1rem;">${p.name}</strong><span style="color:#666;">#${p.tag}</span>
                </td>
                <td style="font-weight: bold;">${acs}</td>
                <td><span style="color: white; font-weight: bold;">${stats.kills}</span></td>
                <td>${stats.deaths}</td>
                <td>${stats.assists}</td>
                <td style="color: ${kdColor}; font-weight: bold;">${kd}</td>
            </tr>
        `;
    });

    return `
        <table class="team-table ${cssClass}">
            <thead>
                <tr>
                    <th>${teamName}</th>
                    <th width="10%">ACS</th>
                    <th width="10%">K</th>
                    <th width="10%">D</th>
                    <th width="10%">A</th>
                    <th width="10%">K/D</th>
                </tr>
            </thead>
            <tbody>
                ${rows}
            </tbody>
        </table>
    `;
}

document.addEventListener('DOMContentLoaded', loadPremierMatches);
