package replay

import (
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Leave reasons and results (see w3g_format.txt, "LeaveGame").
const (
	reasonRemote = 0x01
	reasonLocal  = 0x0C
	resultLeft7  = 0x07
	resultLost   = 0x08
	resultWon    = 0x09
	resultDraw   = 0x0A
)

// decideWinner reads the leave records with the rules the replay format's
// authors found fail-safe across thousands of replays, then w3gjs's
// fallback for games with two teams.
func decideWinner(players []dto.ReplayPlayer, leaves []dto.ReplayLeave) dto.ReplayWinner {
	unknown := dto.ReplayWinner{Team: -1, Confidence: "unknown", Basis: "no leave record decides it"}
	team := map[int]int{}
	teams := map[int]bool{}
	for _, p := range players {
		if !p.Observer {
			team[p.ID] = p.Team
			teams[p.Team] = true
		}
	}
	if len(leaves) == 0 || len(teams) < 2 {
		return unknown
	}
	saverLeave := leaves[len(leaves)-1]
	saver := saverLeave.PlayerID
	saverTeam, saverPlays := team[saver]
	other := func(t int) (int, bool) {
		if len(teams) != 2 {
			return -1, false
		}
		for k := range teams {
			if k != t {
				return k, true
			}
		}
		return -1, false
	}

	for _, l := range leaves {
		if l.RawResult == resultDraw {
			return dto.ReplayWinner{Team: -1, Draw: true, Confidence: "certain", Basis: "a leave record says draw"}
		}
	}
	for _, l := range leaves {
		if t, ok := team[l.PlayerID]; ok && l.RawReason == reasonRemote && l.RawResult == resultWon {
			return dto.ReplayWinner{Team: t, Confidence: "certain", Basis: "a player left with a won result"}
		}
	}
	if saverPlays {
		for _, l := range leaves {
			if l.RawReason != reasonLocal {
				continue
			}
			switch l.RawResult {
			case resultWon:
				return dto.ReplayWinner{Team: saverTeam, Confidence: "certain", Basis: "the saver's game recorded a win"}
			case resultLost:
				if t, ok := other(saverTeam); ok {
					return dto.ReplayWinner{Team: t, Confidence: "certain", Basis: "the saver's game recorded a loss"}
				}
			}
		}
	}

	// A team all of whose players were erased lost; one team left standing won.
	lostPlayers := map[int]bool{}
	for _, l := range leaves {
		if l.RawReason == reasonRemote && l.RawResult == resultLost {
			lostPlayers[l.PlayerID] = true
		}
	}
	standing := map[int]bool{}
	for _, p := range players {
		if !p.Observer && !p.Computer && !lostPlayers[p.ID] {
			standing[p.Team] = true
		}
	}
	if len(lostPlayers) > 0 && len(standing) == 1 {
		for t := range standing {
			return dto.ReplayWinner{Team: t, Confidence: "certain", Basis: "every other team was erased"}
		}
	}

	if saverPlays && saverLeave.RawReason == reasonLocal && saverLeave.RawResult == resultLeft7 {
		inc := len(leaves) > 1
		for _, l := range leaves[:len(leaves)-1] {
			if saverLeave.Counter <= l.Counter {
				inc = false
			}
		}
		if inc {
			return dto.ReplayWinner{Team: saverTeam, Confidence: "likely", Basis: "the saver's last leave record was counted up"}
		}
		if t, ok := other(saverTeam); ok {
			return dto.ReplayWinner{Team: t, Confidence: "likely", Basis: "the saver's last leave record was not counted up"}
		}
	}

	for _, l := range leaves {
		if t, ok := team[l.PlayerID]; ok {
			if w, ok := other(t); ok {
				return dto.ReplayWinner{Team: w, Confidence: "guess", Basis: "the first team to leave lost"}
			}
			break
		}
	}
	return unknown
}
