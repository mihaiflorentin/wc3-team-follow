package follow

import (
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Ranking records a win or a loss for every human the player played a match
// with, and the player, when a match's score event reports the result. It only reads events and never sends anything
// to the game.
//
// Each GameLobbySetup replaces the snapshot of the lobby: who sat on which
// team. SetGlueScreen LOADING_SCREEN arms a match. The first UpdateScoreInfo
// while armed records it and disarms, so the duplicate the game sends is
// ignored. Leaving the game (ExitedGame, MultiplayerGameLeave) and the score
// screen don't disarm: the game reports both before the score. A menu
// screen, a new lobby, a log out, a disconnect or Clear disarm without a
// result.
//
// The result comes from scoreInfo.players, which names every human by
// BattleTag with a won flag. When no player won the match wasn't finished
// (the player left, or it was a draw), and only what is certain is recorded.
// Flags that contradict themselves (winners on two teams, or the player's
// own flag against localPlayerWon) aren't used: a custom map once reported
// everyone in a 4v4 as winners while localPlayerWon said the player lost.
// When localPlayerWon disagrees with the player's own flag, it decides by
// team; otherwise nothing is recorded. A score without those fields falls
// back to the lobby snapshot.
type Ranking struct {
	now     func() time.Time
	logger  *slog.Logger
	entries []dto.RankingEntry
	lobby   *rankLobby
	armed   bool
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
// logger gets one line per recorded match (counts and the source) and per
// ignored score, never BattleTags or payloads.
func NewRanking(now func() time.Time, saved []dto.RankingEntry, logger *slog.Logger) *Ranking {
	r := &Ranking{now: now, logger: logger}
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
// follows now: it is read when the score arrives.
func (r *Ranking) Receive(event dto.GameEvent) {
	p := event.Payload
	switch event.MessageType {
	case "GameLobbySetup":
		r.armed = false
		r.lobby = readRankLobby(p)
	case "LoggedOut":
		r.End()
	case "SetGlueScreen":
		screen, _ := p["screen"].(string)
		switch {
		case screen == "LOADING_SCREEN":
			r.armed = true
		case slices.Contains(menuScreens, screen):
			r.End()
		}
	case "UpdateScoreInfo":
		info := object(p["scoreInfo"])
		won, ok := info["localPlayerWon"].(bool)
		if !ok {
			return
		}
		if !r.armed {
			r.skipped("not armed")
			return
		}
		lobby := r.lobby
		r.End()
		if reason := unrankable(info); reason != "" {
			r.skipped(reason)
			return
		}
		if players := readScorePlayers(info); players.usable {
			r.recordScore(lobby, players, won)
			return
		}
		if lobby == nil {
			r.skipped("no lobby")
			return
		}
		r.record(lobby, won)
	}
}

func (r *Ranking) skipped(reason string) {
	r.logger.Info("ranking.skipped", slog.String("reason", reason))
}

// End disarms the match and forgets the lobby: the match is over without a
// result, or the game is gone.
func (r *Ranking) End() { r.armed, r.lobby = false, nil }

// Clear forgets every entry and any match in progress.
func (r *Ranking) Clear() {
	r.entries = nil
	r.changed = false
	r.End()
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

// record counts one match for the player and every other human who took
// part, from the lobby snapshot alone.
func (r *Ranking) record(l *rankLobby, localWon bool) {
	if l.selfObserver {
		r.skipped("local player observed")
		return
	}
	self := slices.IndexFunc(l.seats, func(s rankSeat) bool { return s.self })
	if self < 0 || l.seats[self].team == noTeam {
		r.skipped("local player unknown")
		return
	}
	myTeam := l.seats[self].team
	now := r.now()
	counted := map[string]bool{}
	var t tally
	for _, s := range l.seats {
		key := tagKey(s.name)
		if !s.human || key == "" || counted[key] || s.team == noTeam {
			continue
		}
		var win bool
		switch {
		case s.team == myTeam:
			win = localWon
		case localWon:
			win = false
		case l.teams == 2 && !l.unknown:
			win = true
		default:
			continue
		}
		counted[key] = true
		r.add(s.name, win, now)
		t.count(win)
	}
	if len(counted) == 0 {
		r.skipped("no players")
		return
	}
	t.log(r.logger, "lobby")
}

// recordScore counts one match from the players of the score: every human,
// the local player (the lobby's own seat) included. Computers never count.
func (r *Ranking) recordScore(l *rankLobby, sp scorePlayers, localWon bool) {
	if l != nil && l.selfObserver {
		r.skipped("local player observed")
		return
	}
	localKey := ""
	if l != nil {
		if i := slices.IndexFunc(l.seats, func(s rankSeat) bool { return s.self }); i >= 0 {
			localKey = tagKey(l.seats[i].name)
		}
	}
	local := slices.IndexFunc(sp.humans, func(p scorePlayer) bool { return localKey != "" && tagKey(p.tag) == localKey })
	if sp.finished {
		if reason := sp.inconsistent(local, localWon); reason != "" {
			if me := local >= 0 && sp.humans[local].wonKnown; me && sp.humans[local].won != localWon && sp.humans[local].teamKnown {
				r.recordByTeam(sp, local, localWon)
				return
			}
			r.skipped(reason)
			return
		}
	}
	now := r.now()
	counted := map[string]bool{}
	var t tally
	source, othersSkipped := "flags", false
	if !sp.finished {
		source = "local-only"
	}
	for _, p := range sp.humans {
		key := tagKey(p.tag)
		isLocal := localKey != "" && key == localKey
		if counted[key] {
			continue
		}
		var win bool
		switch {
		case sp.finished:
			if !p.wonKnown {
				continue
			}
			win = p.won
		case isLocal:
			win = localWon
		case localWon && local >= 0 && p.teamKnown && sp.humans[local].teamKnown && p.team == sp.humans[local].team:
			// Unfinished but the player won: their teammates did too.
			win = true
		default:
			// Unfinished: the others' outcome is unknown.
			othersSkipped = true
			continue
		}
		counted[key] = true
		r.add(p.tag, win, now)
		t.count(win)
	}
	if len(counted) > 0 {
		t.log(r.logger, source)
	}
	switch {
	case len(counted) == 0 && othersSkipped:
		r.skipped("unfinished, local player unknown, others unknown")
	case len(counted) == 0:
		r.skipped("no players")
	case othersSkipped:
		r.skipped("unfinished, others unknown")
	}
}

// recordByTeam counts a match from localPlayerWon alone, when the won flags
// can't be trusted: the player's teammates share their result; when the
// player won everyone else lost, and when they lost the other team won if
// there were exactly two teams.
func (r *Ranking) recordByTeam(sp scorePlayers, local int, localWon bool) {
	myTeam := sp.humans[local].team
	now := r.now()
	counted := map[string]bool{}
	var t tally
	othersSkipped := false
	for _, p := range sp.humans {
		key := tagKey(p.tag)
		if counted[key] {
			continue
		}
		var win bool
		switch {
		case !p.teamKnown:
			othersSkipped = true
			continue
		case p.team == myTeam:
			win = localWon
		case localWon:
			win = false
		case sp.teams == 2 && !sp.teamsUnknown:
			win = true
		default:
			othersSkipped = true
			continue
		}
		counted[key] = true
		r.add(p.tag, win, now)
		t.count(win)
	}
	t.log(r.logger, "local-team")
	if othersSkipped {
		r.skipped("inconsistent flags, others unknown")
	}
}

// tally counts the results of one recorded match for its log line.
type tally struct{ wins, losses int }

func (t *tally) count(win bool) {
	if win {
		t.wins++
	} else {
		t.losses++
	}
}

func (t tally) log(logger *slog.Logger, source string) {
	logger.Info("ranking.recorded", slog.Int("wins", t.wins), slog.Int("losses", t.losses), slog.String("source", source))
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

// unrankable gives the reason a score isn't for a custom-map game that was
// played live, or "". A melee game hosted from Custom Games reports custom
// too; ladder, ranked and campaign games and replays don't count.
func unrankable(info map[string]any) string {
	m := object(info["mapInfo"])
	if isTrue(m["isReplay"]) {
		return "replay"
	}
	if t, ok := m["gameType"].(string); ok && t != "" && !strings.EqualFold(t, "custom") {
		return "not a custom game"
	}
	return ""
}

type scorePlayer struct {
	tag       string
	team      int64
	teamKnown bool
	won       bool
	wonKnown  bool
}

// scorePlayers is scoreInfo.players. usable is set when the score is
// usable (some human is named and some player has a won flag); finished
// when a player won, so the match was played to its end.
type scorePlayers struct {
	humans   []scorePlayer
	usable   bool
	finished bool
	// teams counts the distinct teams of every player, computers included;
	// teamsUnknown is set when a player has no team.
	teams        int
	teamsUnknown bool
}

// inconsistent gives the reason a finished score's won flags can't be
// trusted, or "": humans on two or more teams all won, or the local player's
// flag (local indexes humans, -1 when not found) disagrees with
// localPlayerWon.
func (sp scorePlayers) inconsistent(local int, localWon bool) string {
	if local >= 0 && sp.humans[local].wonKnown && sp.humans[local].won != localWon {
		return "inconsistent flags: local result"
	}
	winners := map[int64]bool{}
	for _, p := range sp.humans {
		if p.wonKnown && p.won && p.teamKnown {
			winners[p.team] = true
		}
	}
	if len(winners) > 1 {
		return "inconsistent flags: winners on several teams"
	}
	return ""
}

// readScorePlayers reads scoreInfo.players: humans are entries that aren't
// computers and have a BattleTag.
func readScorePlayers(info map[string]any) scorePlayers {
	raw, _ := list(info["players"])
	var out scorePlayers
	anyFlag := false
	teams := map[int64]bool{}
	for _, r := range raw {
		x := object(r)
		team, teamOK := intValue(x["teamIndex"])
		if teamOK {
			teams[team] = true
		} else {
			out.teamsUnknown = true
		}
		won, wonOK := x["won"].(bool)
		if wonOK {
			anyFlag = true
			if won {
				out.finished = true
			}
		}
		tag, _ := x["battleTag"].(string)
		tag = strings.TrimSpace(tag)
		if isTrue(x["isAi"]) || tag == "" {
			continue
		}
		out.humans = append(out.humans, scorePlayer{tag: tag, team: team, teamKnown: teamOK, won: won, wonKnown: wonOK})
	}
	out.usable = anyFlag && len(out.humans) > 0
	out.teams = len(teams)
	return out
}
