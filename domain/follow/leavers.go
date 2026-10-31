package follow

import (
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// leaveCluster is the longest gap between two leave records of the same
// cluster: the remaining humans quitting together at the end of a match.
const leaveCluster = 30 * time.Second

// Leaver is a human who left a match early, by the replay's leave record.
type Leaver struct {
	Tag string
	// Left is the game time of the player's leave record.
	Left time.Duration
	// Team is the player's team in the lobby.
	Team int
}

// How a match ended, for LeaverReport.Ending.
const (
	// EndedPlayed is a match played out (or quit) without an end vote.
	EndedPlayed = ""
	// EndedDraw and EndedForfeit are matches ended by a -draw or -ff vote.
	EndedDraw    = "draw"
	EndedForfeit = "forfeit"
)

// LeaverReport is what EarlyLeavers found in one replay.
type LeaverReport struct {
	// End is when the match ended: when the game clock stopped, otherwise
	// the start of the last cluster of human leave records, or the game
	// time when there is none.
	End time.Duration
	// Length is the recorded game time, for reasons.
	Length time.Duration
	// Ending is EndedPlayed, or EndedDraw or EndedForfeit when the match
	// ended by vote; Vote is then the time of the first end vote, and
	// VoteTeam the team of who typed it.
	Ending   string
	Vote     time.Duration
	VoteTeam int
	// Leavers are the early leavers, in the order they left.
	Leavers []Leaver
}

// EarlyLeavers decides who left a match early (ADR 0013), from the leave
// times and the end votes: result codes on custom maps can't be trusted.
//
// The match ends when the game clock stopped (dto.Replay.Ended: a king
// died or an end vote passed). When the replay doesn't show that, it ends
// when the remaining humans quit: the start of the last cluster of human
// leave records (consecutive leaves at most 30 s apart), or the game time
// when no human has a leave record. An end vote is a
// human's chat line (any channel) reading "-draw" or "-ff", ignoring case
// and spaces, at or after end − finalRound; earlier ones change nothing.
// When there is one, the match ended by vote and the cut-off is the first
// end vote: whoever left before it is a leaver, even in the final round,
// so leaving and having the others vote the game over doesn't help.
// Without one, the cut-off is end − finalRound: anyone who left in the
// final round or later is never a leaver.
//
// Only human, non-observer players with a BattleTag and a leave record
// count. Who must never be blacklisted (the player, friends, the follow
// group) is the engine's call, in AddLeavers.
func EarlyLeavers(r dto.Replay, finalRound time.Duration) LeaverReport {
	length := r.GameTime
	if length <= 0 {
		length = r.Duration
	}
	humans := map[int]dto.ReplayPlayer{}
	for _, p := range r.Players {
		p.BattleTag = strings.TrimSpace(p.BattleTag)
		if p.Computer || p.Observer || !tagPattern.MatchString(p.BattleTag) {
			continue
		}
		humans[p.ID] = p
	}
	var leaves []Leaver
	seen := map[int]bool{}
	for _, l := range r.Leaves {
		p, ok := humans[l.PlayerID]
		if !ok || seen[l.PlayerID] {
			continue
		}
		seen[l.PlayerID] = true
		leaves = append(leaves, Leaver{Tag: p.BattleTag, Left: l.Time, Team: p.Team})
	}
	slices.SortStableFunc(leaves, func(a, b Leaver) int { return int(a.Left - b.Left) })

	report := LeaverReport{End: length, Length: length}
	if r.Ended > 0 {
		report.End = r.Ended
	} else if n := len(leaves); n > 0 {
		i := n - 1
		for i > 0 && leaves[i].Left-leaves[i-1].Left <= leaveCluster {
			i--
		}
		report.End = leaves[i].Left
	}
	cutoff := report.End - finalRound
	for _, c := range r.Chat {
		p, ok := humans[c.PlayerID]
		if !ok || c.Time < report.End-finalRound || report.Ending != EndedPlayed && c.Time >= report.Vote {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(c.Message)) {
		case "-draw":
			report.Ending = EndedDraw
		case "-ff":
			report.Ending = EndedForfeit
		default:
			continue
		}
		report.Vote, report.VoteTeam, cutoff = c.Time, p.Team, c.Time
	}
	for _, l := range leaves {
		if l.Left < cutoff {
			report.Leavers = append(report.Leavers, l)
		}
	}
	return report
}

// BlacklistLeavers reports whether players who leave Legion TD early join
// the blacklist (ADR 0013); off by default.
func (e *Engine) BlacklistLeavers() bool { return e.leavers }

// AddLeavers adds a match's early leavers to the blacklist, each with a
// reason, and notes one Activity line for the match (ADR 0013). mapName
// and version name the map, like "Legion TD" and "11.4c-hf9". It never
// adds the player, a Battle.net friend, a member of the current team or
// the follow group, a player without a BattleTag, or anyone already on the
// blacklist; and nobody while the game or its friends list isn't known.
// It returns who it added, with the reason each got, and marks the
// blacklist changed for saving.
func (e *Engine) AddLeavers(report LeaverReport, mapName, version string) []dto.BlacklistEntry {
	if !e.leavers || len(report.Leavers) == 0 || !e.connected || !e.rosterKnown {
		return nil
	}
	label := strings.TrimSpace(mapName + " " + version)
	now := e.now()
	var added []dto.BlacklistEntry
	var first Leaver
	for _, l := range report.Leavers {
		tag := strings.TrimSpace(l.Tag)
		if !tagPattern.MatchString(tag) || e.neverLeaver(tag) || e.blacklisted(tag) ||
			slices.ContainsFunc(added, func(b dto.BlacklistEntry) bool { return tagKey(b.Tag) == tagKey(tag) }) ||
			len(e.blacklist)+len(added) >= dto.MaxBlacklist {
			continue
		}
		if len(added) == 0 {
			first = l
		}
		added = append(added, dto.BlacklistEntry{Tag: tag, Added: now.UTC(), Reason: fmt.Sprintf("Left early: %s, %s, %s",
			label, leftInto(report, l), now.Format("2 Jan 2006"))})
	}
	if len(added) == 0 {
		return nil
	}
	e.blacklist = append(slices.Clone(added), e.blacklist...)
	e.blacklistChanged = true
	if kept := slices.DeleteFunc(slices.Clone(e.manualTags), e.blacklisted); len(kept) != len(e.manualTags) {
		e.manualTags = kept
		e.selectionChanged()
	}
	if len(added) == 1 {
		e.Note(fmt.Sprintf("Blacklisted %s for leaving %s early (%s).", added[0].Tag, mapName, leftInto(report, first)))
	} else {
		e.Note(fmt.Sprintf("Blacklisted %d players for leaving %s early.", len(added), mapName))
	}
	return added
}

// leftInto says when a player left, in whole minutes, and how a match that
// ended by vote ended: "12 min into a 38 min match", "3 min into a 5 min
// match that ended in a draw", "… that their team forfeited" (a -ff from
// the leaver's own team) or "… that ended by forfeit".
func leftInto(r LeaverReport, l Leaver) string {
	s := fmt.Sprintf("%d min into a %d min match", int(l.Left/time.Minute), int(r.Length/time.Minute))
	switch {
	case r.Ending == EndedDraw:
		s += " that ended in a draw"
	case r.Ending == EndedForfeit && r.VoteTeam == l.Team:
		s += " that their team forfeited"
	case r.Ending == EndedForfeit:
		s += " that ended by forfeit"
	}
	return s
}

// neverLeaver reports whether tag must never be blacklisted as a leaver:
// the player, a Battle.net friend, a member of the current team, or
// someone in the follow group.
func (e *Engine) neverLeaver(tag string) bool {
	key := tagKey(tag)
	if e.isSelf(tag) {
		return true
	}
	if record, ok := e.roster[key]; ok && isTrue(record.data["isFriend"]) {
		return true
	}
	if slices.ContainsFunc(e.team, func(m member) bool { return tagKey(m.battleTag) == key }) {
		return true
	}
	return slices.ContainsFunc(e.group(), func(t string) bool { return tagKey(t) == key })
}

// AppendNote adds line to a player's note on a new line, when the result
// fits in dto.MaxNoteLength characters. It reports false, leaving the
// note alone, when it doesn't fit or the note already holds the line.
func AppendNote(note, line string) (string, bool) {
	note, line = strings.TrimSpace(note), strings.TrimSpace(line)
	switch {
	case line == "" || strings.Contains(note, line):
		return note, false
	case note == "":
		return line, utf8.RuneCountInString(line) <= dto.MaxNoteLength
	}
	joined := note + "\n" + line
	if utf8.RuneCountInString(joined) > dto.MaxNoteLength {
		return note, false
	}
	return joined, true
}
