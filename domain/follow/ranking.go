package follow

import (
	"slices"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Ranking records a win or a loss for the group when a match's score
// screen reports the result. It only reads events and never sends anything
// to the game.
//
// Each GameLobbySetup replaces the snapshot of the lobby: who sat on which
// team. Lobby updates stop once a match loads, so the last one is the
// match. UpdateScoreInfo with the multiplayer shape (scoreInfo.
// localPlayerWon) records the match against that snapshot and closes it,
// so a repeated score event or screen counts nothing. Leaving the game, a
// menu screen, a disconnect or a new lobby ends the match without a
// result. The score screen itself doesn't end it: the score event may
// come after the screen is set.
//
// The player's team gets the player's result. A group member on another
// team gets the opposite when that is certain: the player won, or the
// match had exactly two teams. A per-team won flag in the score, when it
// agrees with the player's own result, decides instead.
type Ranking struct {
	now     func() time.Time
	entries []dto.RankingEntry
	lobby   *rankLobby
	changed bool
}

// rankLobby is the last lobby: the occupied seats of players who take
// part, and whether the teams are all known.
type rankLobby struct {
	seats []rankSeat
	// teams counts the distinct teams of everyone taking part, computers
	// included; unknown is set when a seat has no team number.
	teams   int
	unknown bool
	// selfObserver is set when the local player only watched.
	selfObserver bool
}

type rankSeat struct {
	name  string
	team  int64
	self  bool
	human bool
}

// noTeam marks a seat whose team the lobby didn't say.
const noTeam = -1

// NewRanking returns a recorder holding the saved entries.
func NewRanking(now func() time.Time, saved []dto.RankingEntry) *Ranking {
	r := &Ranking{now: now}
	for _, e := range saved {
		if strings.TrimSpace(e.Tag) == "" {
			continue
		}
		if i := r.index(e.Tag); i >= 0 {
			// A file listing a player twice adds the records up.
			prev := r.entries[i]
			prev.Wins += e.Wins
			prev.Losses += e.Losses
			if e.LastPlayed.After(prev.LastPlayed) {
				prev.LastPlayed, prev.Tag = e.LastPlayed, e.Tag
			}
			r.entries[i] = prev
			continue
		}
		r.entries = append(r.entries, e)
	}
	r.order()
	return r
}

// Receive applies one game event. group is the BattleTags the player
// follows now: it is read when the match ends.
func (r *Ranking) Receive(event dto.GameEvent, group []string) {
	p := event.Payload
	switch event.MessageType {
	case "GameLobbySetup":
		r.lobby = readRankLobby(p)
	case "MultiplayerGameLeave", "ExitedGame", "LoggedOut":
		r.End()
	case "SetGlueScreen":
		if screen, _ := p["screen"].(string); slices.Contains(menuScreens, screen) {
			r.End()
		}
	case "UpdateScoreInfo":
		info := object(p["scoreInfo"])
		won, ok := info["localPlayerWon"].(bool)
		if !ok || r.lobby == nil {
			return
		}
		lobby := r.lobby
		r.lobby = nil
		r.record(lobby, won, teamResults(info), group)
	}
}

// End forgets the lobby: the match is over without a result, or the game
// is gone.
func (r *Ranking) End() { r.lobby = nil }

// Clear forgets every entry. A match in progress is still recorded.
func (r *Ranking) Clear() {
	r.entries = nil
	r.changed = false
}

// Entries returns the records, most recently played first. Never nil.
func (r *Ranking) Entries() []dto.RankingEntry {
	out := slices.Clone(r.entries)
	if out == nil {
		out = []dto.RankingEntry{}
	}
	return out
}

// Unsaved reports whether a match was recorded since MarkSaved.
func (r *Ranking) Unsaved() bool { return r.changed }

// MarkSaved records that the entries were saved.
func (r *Ranking) MarkSaved() { r.changed = false }

// record counts one match for the player and the group members who took
// part.
func (r *Ranking) record(l *rankLobby, localWon bool, byTeam map[int64]bool, group []string) {
	if l.selfObserver {
		return
	}
	self := slices.IndexFunc(l.seats, func(s rankSeat) bool { return s.self })
	if self < 0 {
		return
	}
	myTeam := l.seats[self].team
	if myTeam == noTeam {
		return
	}
	if known, ok := byTeam[myTeam]; !ok || known != localWon {
		// The flags don't line up with the player's own result, so their
		// team numbers can't be trusted.
		byTeam = nil
	}
	now := r.now()
	counted := map[string]bool{}
	for _, s := range l.seats {
		key := tagKey(s.name)
		if !s.human || key == "" || counted[key] || s.team == noTeam {
			continue
		}
		if !s.self && !slices.ContainsFunc(group, func(g string) bool { return tagKey(g) == key }) {
			continue
		}
		var win bool
		switch {
		case s.team == myTeam:
			win = localWon
		case byTeam != nil && teamKnown(byTeam, s.team):
			win = byTeam[s.team]
		case localWon:
			win = false
		case l.teams == 2 && !l.unknown:
			win = true
		default:
			continue
		}
		counted[key] = true
		r.add(s.name, win, now)
	}
}

func teamKnown(m map[int64]bool, team int64) bool {
	_, ok := m[team]
	return ok
}

func (r *Ranking) add(tag string, win bool, now time.Time) {
	i := r.index(tag)
	if i < 0 {
		r.entries = append(r.entries, dto.RankingEntry{Tag: tag})
		i = len(r.entries) - 1
	}
	e := r.entries[i]
	e.Tag, e.LastPlayed = tag, now
	if win {
		e.Wins++
	} else {
		e.Losses++
	}
	r.entries[i] = e
	r.changed = true
	r.order()
}

func (r *Ranking) index(tag string) int {
	key := tagKey(tag)
	return slices.IndexFunc(r.entries, func(e dto.RankingEntry) bool { return tagKey(e.Tag) == key })
}

// order sorts the entries most recently played first, then by BattleTag,
// and keeps dto.MaxRankingEntries.
func (r *Ranking) order() {
	slices.SortStableFunc(r.entries, func(a, b dto.RankingEntry) int {
		if c := b.LastPlayed.Compare(a.LastPlayed); c != 0 {
			return c
		}
		return strings.Compare(tagKey(a.Tag), tagKey(b.Tag))
	})
	if len(r.entries) > dto.MaxRankingEntries {
		r.entries = r.entries[:dto.MaxRankingEntries]
	}
}

// readRankLobby reads a GameLobbySetup: occupied seats, observers left
// out, humans and computers alike so that teams are counted right.
func readRankLobby(p map[string]any) *rankLobby {
	players, _ := list(p["players"])
	l := &rankLobby{}
	teams := map[int64]bool{}
	for _, raw := range players {
		x := object(raw)
		if !numberIs(x["slotStatus"], slotOccupied) {
			continue
		}
		name, _ := x["name"].(string)
		name = strings.TrimSpace(name)
		self := isTrue(x["isSelf"])
		if isTrue(x["isObserver"]) {
			if self {
				l.selfObserver = true
			}
			continue
		}
		team, ok := intValue(x["team"])
		if !ok {
			team, l.unknown = noTeam, true
		} else {
			teams[team] = true
		}
		l.seats = append(l.seats, rankSeat{name: name, team: team, self: self, human: numberIs(x["slotType"], slotHuman)})
	}
	l.teams = len(teams)
	return l
}

// teamResults reads the per-team won flags of a score, if it has any:
// entries of scoreInfo.players with teamIndex and a boolean won. A team
// whose entries disagree is left out.
func teamResults(info map[string]any) map[int64]bool {
	players, _ := list(info["players"])
	out := map[int64]bool{}
	conflict := map[int64]bool{}
	for _, raw := range players {
		x := object(raw)
		team, ok := intValue(x["teamIndex"])
		won, wonOK := x["won"].(bool)
		if !ok || !wonOK {
			continue
		}
		if prev, seen := out[team]; seen && prev != won {
			conflict[team] = true
		}
		out[team] = won
	}
	for team := range conflict {
		delete(out, team)
	}
	return out
}
