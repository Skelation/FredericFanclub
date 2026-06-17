package premier

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time" // Added time package
)

// --- ALIAS / REASSIGNMENT CONFIG ---

type AliasEntry struct {
	Ingame string `json:"ingame"`
	Player string `json:"player"`
}

type ReassignmentEntry struct {
	MatchID string `json:"match_id"`
	Ingame  string `json:"ingame"`
	Player  string `json:"player"`
}

func loadAliasEntries() []AliasEntry {
	data, err := os.ReadFile("./data/premier/aliases.json")
	if err != nil {
		return nil
	}
	var entries []AliasEntry
	json.Unmarshal(data, &entries)
	return entries
}

func loadReassignmentEntries() []ReassignmentEntry {
	data, err := os.ReadFile("./data/premier/reassignments.json")
	if err != nil {
		return nil
	}
	var entries []ReassignmentEntry
	json.Unmarshal(data, &entries)
	return entries
}

func buildAliasMap(entries []AliasEntry) map[string]string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.Ingame] = e.Player
	}
	return m
}

func buildReassignmentIndex(entries []ReassignmentEntry) map[string]map[string]string {
	idx := make(map[string]map[string]string)
	for _, e := range entries {
		if idx[e.MatchID] == nil {
			idx[e.MatchID] = make(map[string]string)
		}
		idx[e.MatchID][e.Ingame] = e.Player
	}
	return idx
}

// --- JSON INPUT STRUCTURES ---
type MatchData struct {
	Metadata struct {
		Map       string `json:"map"`
		GameStart int64  `json:"game_start"`
	} `json:"metadata"`
	Players struct {
		AllPlayers []struct {
			PUUID          string `json:"puuid"`
			Name           string `json:"name"`
			Tag            string `json:"tag"`
			Team           string `json:"team"`
			Character      string `json:"character"`
			DamageMade     int    `json:"damage_made"`
			DamageReceived int    `json:"damage_received"`
			Stats          struct {
				Kills     int `json:"kills"`
				Deaths    int `json:"deaths"`
				Assists   int `json:"assists"`
				Score     int `json:"score"`
				Headshots int `json:"headshots"`
				Bodyshots int `json:"bodyshots"`
				Legshots  int `json:"legshots"`
			} `json:"stats"`
		} `json:"all_players"`
	} `json:"players"`
	Teams map[string]struct {
		HasWon bool `json:"has_won"`
		Roster struct {
			Name string `json:"name"`
		} `json:"roster"`
	} `json:"teams"`
	Kills []struct {
		KillerPUUID       string `json:"killer_puuid"`
		KillerDisplayName string `json:"killer_display_name"`
		KillerTeam        string `json:"killer_team"`
		VictimPUUID       string `json:"victim_puuid"`
		VictimDisplayName string `json:"victim_display_name"`
		VictimTeam        string `json:"victim_team"`
		WeaponName        string `json:"damage_weapon_name"`
		Assistants        []struct {
			AssistantPUUID string `json:"assistant_puuid"`
		} `json:"assistants"`
		Round           int `json:"round"`
		KillTimeInRound int `json:"kill_time_in_round"`
	} `json:"kills"`
	Rounds []struct {
		WinningTeam string `json:"winning_team"`
		EndType     string `json:"end_type"`
		BombPlanted bool   `json:"bomb_planted"`
	} `json:"rounds"`
}

// --- JSON OUTPUT STRUCTURES (For the Frontend) ---
type AgentStat struct {
	AgentName string `json:"agent_name"`
	Matches   int    `json:"matches"`
}

type WeaponStat struct {
	WeaponName string `json:"weapon"`
	Kills      int    `json:"kills"`
}

type PlayerOverview struct {
	Name               string      `json:"name"`
	Matches            int         `json:"matches"`
	Wins               int         `json:"wins"`
	WinRate            float64     `json:"win_rate"`
	AverageACS         float64     `json:"average_acs"`
	ADR                float64     `json:"adr"`
	KD                 float64     `json:"kd"`
	HeadshotPercentage float64     `json:"headshot_percentage"`
	Kills              int         `json:"kills"`
	Assists            int         `json:"assists"`
	FirstBloods        int         `json:"first_bloods"`
	FirstDeaths        int         `json:"first_deaths"`
	Clutches           int         `json:"clutches"`
	KAST               float64     `json:"kast"`
	TopAgents          []AgentStat  `json:"top_agents"`
	TopWeapons         []WeaponStat `json:"top_weapons"`
}

type MapStats struct {
	Wins    int     `json:"wins"`
	Losses  int     `json:"losses"`
	WinRate float64 `json:"win_rate"`
}

type TeamStats struct {
	MapWinRates map[string]MapStats `json:"map_win_rates"`
}

type PlayerPerformance struct {
	PlayerName   string  `json:"player_name"`
	ACS          float64 `json:"acs"`
	Kills        int     `json:"kills"`
	Deaths       int     `json:"deaths"`
	Assists      int     `json:"assists"`
	HeadshotPct  float64 `json:"headshot_pct"`
	Opponent     string  `json:"opponent"`
	MatchOutcome string  `json:"match_outcome"`
	Map          string  `json:"map"`
	MetricValue  float64 `json:"-"`
}

type Performances struct {
	BestPerformance  PlayerPerformance `json:"best_performance"`
	MostKills        PlayerPerformance `json:"most_kills"`
	ClutchKing       PlayerPerformance `json:"clutch_king"`
	WorstPerformance PlayerPerformance `json:"worst_performance"`
}

type DashboardData struct {
	TeamStats    TeamStats        `json:"team_stats"`
	PlayerStats  []PlayerOverview `json:"player_stats"`
	Performances Performances     `json:"performances"`
}

type aggPlayerStats struct {
	Matches        int
	Wins           int
	RoundsPlayed   int
	TotalScore     int
	TotalDamage    int
	Kills          int
	Deaths         int
	Assists        int
	TotalHeadshots int
	TotalBodyshots int
	TotalLegshots  int
	FirstBloods    int
	FirstDeaths    int
	Clutches       int
	KASTEvents     int
	AgentsPlayed   map[string]int
	WeaponKills    map[string]int
}

// Helper methods
func round2(val float64) float64 {
	return math.Round(val*100) / 100
}

func isTarget(name string, targets []string) bool {
	for _, t := range targets {
		if t == name {
			return true
		}
	}
	return false
}

func GenerateTeamStats(targetPlayers []string) {
	archiveDir := "./data/premier/archive"
	statsFile := "./data/premier/dashboard_stats.json"

	// --- DATE FILTER LOGIC ---
	// Set the cutoff date for the current split (DD/MM/YYYY)
	cutoffDate, _ := time.Parse("02/01/2006", "06/05/2026")
	cutoffUnix := cutoffDate.Unix()
	// -------------------------

	globalAliases := buildAliasMap(loadAliasEntries())
	matchReassignments := buildReassignmentIndex(loadReassignmentEntries())

	playerAggregates := make(map[string]aggPlayerStats)
	teamMapRates := make(map[string]MapStats)

	bestPerf := PlayerPerformance{}
	mostKillsPerf := PlayerPerformance{}
	clutchKingPerf := PlayerPerformance{}
	worstPerf := PlayerPerformance{MetricValue: 9999.0}

	files, err := os.ReadDir(archiveDir)
	if err != nil {
		log.Println("Stats Generator: Error reading archive dir:", err)
		return
	}

	type fileInfo struct {
		Path      string
		MatchID   string
		GameStart int64
	}
	var validFiles []fileInfo

	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		matchID := strings.TrimSuffix(f.Name(), ".json")
		path := filepath.Join(archiveDir, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var peek MatchData
		if err := json.Unmarshal(data, &peek); err == nil {
			validFiles = append(validFiles, fileInfo{
				Path:      path,
				MatchID:   matchID,
				GameStart: peek.Metadata.GameStart,
			})
		}
	}

	// Sort chronologically (newest first)
	sort.Slice(validFiles, func(i, j int) bool {
		return validFiles[i].GameStart > validFiles[j].GameStart
	})

	// Filter out any files BEFORE the cutoff date
	var recentFiles []fileInfo
	for _, f := range validFiles {
		if f.GameStart >= cutoffUnix {
			recentFiles = append(recentFiles, f)
		}
	}
	validFiles = recentFiles // Overwrite with only the recent ones

	// 2. Process only the recent valid files
	for _, fInfo := range validFiles {
		data, err := os.ReadFile(fInfo.Path)
		if err != nil {
			continue
		}

		var match MatchData
		if err := json.Unmarshal(data, &match); err != nil {
			continue
		}

		// Build effective alias map: global aliases + match-specific overrides
		effectiveAliases := make(map[string]string, len(globalAliases))
		for k, v := range globalAliases {
			effectiveAliases[k] = v
		}
		for k, v := range matchReassignments[fInfo.MatchID] {
			effectiveAliases[k] = v
		}

		teamCounts := make(map[string]int)
		playerTeams := make(map[string]string)
		puuidToName := make(map[string]string)
		for _, p := range match.Players.AllPlayers {
			fullName := fmt.Sprintf("%s#%s", p.Name, p.Tag)

			if alias, exists := effectiveAliases[fullName]; exists {
				fullName = alias
			}

			playerTeams[p.PUUID] = p.Team
			puuidToName[p.PUUID] = fullName
			if isTarget(fullName, targetPlayers) {
				teamCounts[p.Team]++
			}
		}

		// Weapon kill tracking
		for _, k := range match.Kills {
			if k.WeaponName == "" {
				continue
			}
			killerName := puuidToName[k.KillerPUUID]
			if !isTarget(killerName, targetPlayers) {
				continue
			}
			agg := playerAggregates[killerName]
			if agg.WeaponKills == nil {
				agg.WeaponKills = make(map[string]int)
			}
			agg.WeaponKills[k.WeaponName]++
			playerAggregates[killerName] = agg
		}

		ourTeamStr := "Blue"
		if teamCounts["Red"] > teamCounts["Blue"] {
			ourTeamStr = "Red"
		}
		opponentTeamStr := "Blue"
		if ourTeamStr == "Blue" {
			opponentTeamStr = "Red"
		}

		ourTeamWon := match.Teams[strings.ToLower(ourTeamStr)].HasWon
		opponentName := match.Teams[strings.ToLower(opponentTeamStr)].Roster.Name
		if opponentName == "" {
			opponentName = fmt.Sprintf("Team %s", opponentTeamStr)
		}

		mapName := match.Metadata.Map
		ms := teamMapRates[mapName]
		if ourTeamWon {
			ms.Wins++
		} else {
			ms.Losses++
		}
		ms.WinRate = round2(float64(ms.Wins) / float64(ms.Wins+ms.Losses) * 100.0)
		teamMapRates[mapName] = ms

		type KillEvent struct {
			KillerPUUID     string
			VictimPUUID     string
			VictimTeam      string
			Assistants      []string
			KillTimeInRound int
		}
		roundKills := make(map[int][]KillEvent)
		for _, k := range match.Kills {
			var asts []string
			for _, a := range k.Assistants {
				asts = append(asts, a.AssistantPUUID)
			}
			roundKills[k.Round] = append(roundKills[k.Round], KillEvent{
				KillerPUUID:     k.KillerPUUID,
				VictimPUUID:     k.VictimPUUID,
				VictimTeam:      k.VictimTeam,
				Assistants:      asts,
				KillTimeInRound: k.KillTimeInRound,
			})
		}

		clutches := make(map[string]int)
		fbTracker := make(map[string]int)
		fdTracker := make(map[string]int)
		kastTracker := make(map[string]int)

		for roundNum, r := range match.Rounds {
			winner := r.WinningTeam
			if winner == "" {
				continue
			}

			killsThisRound := roundKills[roundNum]

			// Evaluate KAST (Kill, Assist, Survive, Trade) for all players
			for puuid := range playerTeams {
				k, a, s, t := false, false, true, false

				for _, kill := range killsThisRound {
					if kill.KillerPUUID == puuid {
						k = true
					}
					for _, ast := range kill.Assistants {
						if ast == puuid {
							a = true
						}
					}
					if kill.VictimPUUID == puuid {
						s = false
						// Check if Traded (killer dies within 5000ms)
						for _, subKill := range killsThisRound {
							if subKill.VictimPUUID == kill.KillerPUUID {
								timeDiff := subKill.KillTimeInRound - kill.KillTimeInRound
								if timeDiff > 0 && timeDiff <= 5000 {
									t = true
									break
								}
							}
						}
					}
				}
				if k || a || s || t {
					kastTracker[puuid]++
				}
			}

			// First Blood and First Death detection
			if len(killsThisRound) > 0 {
				sort.Slice(killsThisRound, func(i, j int) bool {
					return killsThisRound[i].KillTimeInRound < killsThisRound[j].KillTimeInRound
				})
				fbTracker[killsThisRound[0].KillerPUUID]++
				fdTracker[killsThisRound[0].VictimPUUID]++
			}

			// Clutch detection
			victims := make(map[string]bool)
			for _, k := range killsThisRound {
				if k.VictimTeam == winner {
					victims[k.VictimPUUID] = true
				}
			}

			var aliveOnWinner []string
			for puuid, team := range playerTeams {
				if team == winner && !victims[puuid] {
					aliveOnWinner = append(aliveOnWinner, puuid)
				}
			}

			if len(aliveOnWinner) == 1 {
				clutches[aliveOnWinner[0]]++
			}
		}

		for _, p := range match.Players.AllPlayers {
			fullName := fmt.Sprintf("%s#%s", p.Name, p.Tag)

			if alias, exists := effectiveAliases[fullName]; exists {
				fullName = alias
			}

			if !isTarget(fullName, targetPlayers) {
				continue
			}

			roundsPlayed := len(match.Rounds)
			if roundsPlayed == 0 {
				continue
			}

			matchACS := float64(p.Stats.Score) / float64(roundsPlayed)
			matchHS := 0.0
			totalShots := p.Stats.Headshots + p.Stats.Bodyshots + p.Stats.Legshots
			if totalShots > 0 {
				matchHS = float64(p.Stats.Headshots) / float64(totalShots) * 100.0
			}

			matchOutcomeStr := "Loss"
			if match.Teams[strings.ToLower(p.Team)].HasWon {
				matchOutcomeStr = "Win"
			}

			agg := playerAggregates[fullName]
			agg.Matches++
			if match.Teams[strings.ToLower(p.Team)].HasWon {
				agg.Wins++
			}
			agg.RoundsPlayed += roundsPlayed
			agg.TotalScore += p.Stats.Score
			agg.TotalDamage += p.DamageMade
			agg.Kills += p.Stats.Kills
			agg.Deaths += p.Stats.Deaths
			agg.Assists += p.Stats.Assists
			agg.TotalHeadshots += p.Stats.Headshots
			agg.TotalBodyshots += p.Stats.Bodyshots
			agg.TotalLegshots += p.Stats.Legshots
			agg.FirstBloods += fbTracker[p.PUUID]
			agg.FirstDeaths += fdTracker[p.PUUID]
			agg.Clutches += clutches[p.PUUID]
			agg.KASTEvents += kastTracker[p.PUUID]
			if agg.AgentsPlayed == nil {
				agg.AgentsPlayed = make(map[string]int)
			}
			agg.AgentsPlayed[p.Character]++
			playerAggregates[fullName] = agg

			perf := PlayerPerformance{
				PlayerName:   fullName,
				ACS:          round2(matchACS),
				Kills:        p.Stats.Kills,
				Deaths:       p.Stats.Deaths,
				Assists:      p.Stats.Assists,
				HeadshotPct:  round2(matchHS),
				Opponent:     opponentName,
				MatchOutcome: matchOutcomeStr,
				Map:          mapName,
			}

			if matchACS > bestPerf.MetricValue {
				bestPerf = perf
				bestPerf.MetricValue = matchACS
			}
			if p.Stats.Kills > int(mostKillsPerf.MetricValue) {
				mostKillsPerf = perf
				mostKillsPerf.MetricValue = float64(p.Stats.Kills)
			}
			if float64(clutches[p.PUUID]) > clutchKingPerf.MetricValue {
				clutchKingPerf = perf
				clutchKingPerf.MetricValue = float64(clutches[p.PUUID])
			}
			if roundsPlayed >= 13 && matchACS < worstPerf.MetricValue {
				worstPerf = perf
				worstPerf.MetricValue = matchACS
			}
		}
	}

	var finalPlayers []PlayerOverview
	for name, agg := range playerAggregates {
		if agg.Matches == 0 {
			continue
		}

		avgACS := float64(agg.TotalScore) / float64(agg.RoundsPlayed)
		adr := float64(agg.TotalDamage) / float64(agg.RoundsPlayed)
		winRate := float64(agg.Wins) / float64(agg.Matches) * 100.0
		kd := float64(agg.Kills)
		if agg.Deaths > 0 {
			kd = float64(agg.Kills) / float64(agg.Deaths)
		}

		hsPct := 0.0
		totalShots := agg.TotalHeadshots + agg.TotalBodyshots + agg.TotalLegshots
		if totalShots > 0 {
			hsPct = float64(agg.TotalHeadshots) / float64(totalShots) * 100.0
		}

		kastPct := 0.0
		if agg.RoundsPlayed > 0 {
			kastPct = float64(agg.KASTEvents) / float64(agg.RoundsPlayed) * 100.0
		}

		var agents []AgentStat
		for agName, count := range agg.AgentsPlayed {
			agents = append(agents, AgentStat{AgentName: agName, Matches: count})
		}
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].Matches > agents[j].Matches
		})
		if len(agents) > 3 {
			agents = agents[:3]
		}

		var weapons []WeaponStat
		for wName, wKills := range agg.WeaponKills {
			weapons = append(weapons, WeaponStat{WeaponName: wName, Kills: wKills})
		}
		sort.Slice(weapons, func(i, j int) bool {
			return weapons[i].Kills > weapons[j].Kills
		})
		if len(weapons) > 5 {
			weapons = weapons[:5]
		}

		finalPlayers = append(finalPlayers, PlayerOverview{
			Name:               name,
			Matches:            agg.Matches,
			Wins:               agg.Wins,
			WinRate:            round2(winRate),
			AverageACS:         round2(avgACS),
			ADR:                round2(adr),
			KD:                 round2(kd),
			HeadshotPercentage: round2(hsPct),
			Kills:              agg.Kills,
			Assists:            agg.Assists,
			FirstBloods:        agg.FirstBloods,
			FirstDeaths:        agg.FirstDeaths,
			Clutches:           agg.Clutches,
			KAST:               round2(kastPct),
			TopAgents:          agents,
			TopWeapons:         weapons,
		})
	}

	sort.Slice(finalPlayers, func(i, j int) bool {
		return finalPlayers[i].Name < finalPlayers[j].Name
	})

	output := DashboardData{
		TeamStats:   TeamStats{MapWinRates: teamMapRates},
		PlayerStats: finalPlayers,
		Performances: Performances{
			BestPerformance:  bestPerf,
			MostKills:        mostKillsPerf,
			ClutchKing:       clutchKingPerf,
			WorstPerformance: worstPerf,
		},
	}

	outBytes, err := json.MarshalIndent(output, "", "  ")
	if err == nil {
		os.MkdirAll("./data/premier", 0755)
		os.WriteFile(statsFile, outBytes, 0644)
		log.Println("Stats Generator: Dashboard JSON refreshed in", statsFile)
	}
}

// ── RECAP TYPES ──────────────────────────────────────────────────────────────

type recapKillOut struct {
	Killer     string `json:"k"`
	KillerTeam string `json:"kt"`
	Victim     string `json:"v"`
	Weapon     string `json:"w"`
}

type recapRoundOut struct {
	Num     int            `json:"n"`
	Winner  string         `json:"winner"`
	EndType string         `json:"end"`
	Planted bool           `json:"planted"`
	Kills   []recapKillOut `json:"kills"`
}

type matchRecapOut struct {
	Rounds []recapRoundOut `json:"rounds"`
}

// GenerateRecaps reads archive files and writes condensed per-match round
// recap files to data/premier/recaps/. Already-generated recaps are skipped.
func GenerateRecaps() {
	archiveDir := "./data/premier/archive"
	recapDir := "./data/premier/recaps"
	os.MkdirAll(recapDir, 0755)

	files, err := os.ReadDir(archiveDir)
	if err != nil {
		log.Println("GenerateRecaps: cannot read archive dir:", err)
		return
	}

	generated := 0
	for _, f := range files {
		if !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		matchID := strings.TrimSuffix(f.Name(), ".json")
		recapPath := filepath.Join(recapDir, matchID+".json")

		// Skip if recap already exists
		if _, err := os.Stat(recapPath); err == nil {
			continue
		}

		data, err := os.ReadFile(filepath.Join(archiveDir, f.Name()))
		if err != nil {
			continue
		}

		var m MatchData
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}

		// Build PUUID → short display name map
		puuidShortName := make(map[string]string)
		for _, p := range m.Players.AllPlayers {
			puuidShortName[p.PUUID] = p.Name
		}

		// Index kills by round, sorted by kill time
		type tempKill struct {
			killer     string
			killerTeam string
			victim     string
			weapon     string
			killTime   int
		}
		roundKillIdx := make(map[int][]tempKill)
		for _, k := range m.Kills {
			weapon := k.WeaponName
			if weapon == "" {
				weapon = "Ability"
			}
			killer := puuidShortName[k.KillerPUUID]
			if killer == "" {
				killer = k.KillerDisplayName
			}
			victim := puuidShortName[k.VictimPUUID]
			if victim == "" {
				victim = k.VictimDisplayName
			}
			roundKillIdx[k.Round] = append(roundKillIdx[k.Round], tempKill{
				killer:     killer,
				killerTeam: k.KillerTeam,
				victim:     victim,
				weapon:     weapon,
				killTime:   k.KillTimeInRound,
			})
		}
		for rNum := range roundKillIdx {
			sort.Slice(roundKillIdx[rNum], func(i, j int) bool {
				return roundKillIdx[rNum][i].killTime < roundKillIdx[rNum][j].killTime
			})
		}

		// Build output rounds
		rounds := make([]recapRoundOut, len(m.Rounds))
		for i, r := range m.Rounds {
			kills := make([]recapKillOut, 0, len(roundKillIdx[i]))
			for _, k := range roundKillIdx[i] {
				kills = append(kills, recapKillOut{
					Killer:     k.killer,
					KillerTeam: k.killerTeam,
					Victim:     k.victim,
					Weapon:     k.weapon,
				})
			}
			rounds[i] = recapRoundOut{
				Num:     i + 1,
				Winner:  r.WinningTeam,
				EndType: r.EndType,
				Planted: r.BombPlanted,
				Kills:   kills,
			}
		}

		out := matchRecapOut{Rounds: rounds}
		outBytes, err := json.Marshal(out)
		if err != nil {
			continue
		}
		os.WriteFile(recapPath, outBytes, 0644)
		generated++
	}

	if generated > 0 {
		log.Printf("GenerateRecaps: wrote %d new recap files", generated)
	}
}
