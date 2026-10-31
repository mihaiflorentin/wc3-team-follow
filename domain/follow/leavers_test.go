package follow_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// mmss is a game time written m:ss.
func mmss(s string) time.Duration {
	var m int
	var sec float64
	if _, err := fmt.Sscanf(s, "%d:%f", &m, &sec); err != nil {
		panic(s)
	}
	return time.Duration(m)*time.Minute + time.Duration(sec*float64(time.Second))
}

// endedAt sets when the replay's game clock stopped.
func endedAt(r dto.Replay, at string) dto.Replay {
	r.Ended = mmss(at)
	return r
}

// match builds a replay of human players Player#1001, Player#1002, … on two
// teams of four (ids 1–4 team 0, 5–8 team 1) who left at the given times
// ("" for no leave record), with chat lines as "id m:ss text".
func match(gameTime string, leaves []string, chat ...string) dto.Replay {
	r := dto.Replay{GameTime: mmss(gameTime)}
	for i, left := range leaves {
		id := i + 1
		r.Players = append(r.Players, dto.ReplayPlayer{ID: id, BattleTag: fmt.Sprintf("Player#%d", 1000+id), Team: i / 4})
		if left != "" {
			r.Leaves = append(r.Leaves, dto.ReplayLeave{Time: mmss(left), PlayerID: id, Result: "left"})
		}
	}
	slices.SortStableFunc(r.Leaves, func(a, b dto.ReplayLeave) int { return int(a.Time - b.Time) })
	for _, c := range chat {
		var id int
		var at, text string
		if _, err := fmt.Sscanf(c, "%d %s", &id, &at); err != nil {
			panic(c)
		}
		text = strings.SplitN(c, " ", 3)[2]
		r.Chat = append(r.Chat, dto.ReplayChat{Time: mmss(at), PlayerID: id, Mode: "unseen", Message: text})
	}
	return r
}

func leaverTags(r follow.LeaverReport) []string {
	var out []string
	for _, l := range r.Leavers {
		out = append(out, l.Tag)
	}
	return out
}

// lateEnd is a 38 minute match the others left at 37:50–38:05.
var lateEnd = []string{"37:50", "37:55", "38:00", "38:05", "38:05", "38:02", "37:58"}

func TestEarlyLeavers(t *testing.T) {
	const finalRound = 4 * time.Minute
	cases := []struct {
		name   string
		replay dto.Replay
		want   []string
		ending string
	}{
		{
			// The real forfeit: everyone left within 16 s after West's -ff.
			name: "forfeit, everyone left together",
			replay: match("21:58", []string{"21:42", "21:44", "21:46", "21:47", "21:49", "21:53", "21:58", "21:58"},
				"1 20:45 -ff", "2 21:00 -ff", "3 21:35 -FF"),
			ending: follow.EndedForfeit,
		},
		{
			name:   "forfeit without the votes, everyone left together",
			replay: match("21:58", []string{"21:42", "21:44", "21:46", "21:47", "21:49", "21:53", "21:58", "21:58"}),
		},
		{
			// The wrong map version: one left at once, the rest at 8:03.
			name:   "aborted game, the first to leave is a leaver",
			replay: match("8:03", []string{"0:20", "8:03", "8:03", "8:03", "8:03", "8:03", "8:03", "8:03"}),
			want:   []string{"Player#1001"},
		},
		{
			name:   "left at 12:00 of 38",
			replay: match("38:05", append([]string{"12:00"}, lateEnd...)),
			want:   []string{"Player#1001"},
		},
		{
			name:   "left at 2:30 of 38",
			replay: match("38:05", append([]string{"2:30"}, lateEnd...)),
			want:   []string{"Player#1001"},
		},
		{
			name:   "left at 34:30 of 38, in the final round",
			replay: match("38:05", append([]string{"34:30"}, lateEnd...)),
		},
		{
			name:   "no votes, left at 35:00 of 38:00",
			replay: match("38:00", []string{"35:00", "38:00", "38:00", "38:00", "38:00", "38:00", "38:00", "38:00"}),
		},
		{
			name:   "left at 1:30, the others voted -draw",
			replay: match("3:00", []string{"1:30", "2:50", "2:52", "2:55", "2:56", "2:58", "3:00", "3:00"}, "5 2:10 -draw", "6 2:20 -draw", "7 2:40  -Draw "),
			want:   []string{"Player#1001"},
			ending: follow.EndedDraw,
		},
		{
			name: "left just before the -ff votes",
			replay: match("21:58", []string{"20:30", "21:44", "21:46", "21:47", "21:49", "21:53", "21:58", "21:58"},
				"5 20:45 -ff", "6 21:00 -ff"),
			want:   []string{"Player#1001"},
			ending: follow.EndedForfeit,
		},
		{
			name:   "a -draw at 5:00 of a match played to the end changes nothing",
			replay: match("38:05", append([]string{"34:30"}, lateEnd...), "2 5:00 -draw"),
		},
		{
			// The real forfeit, with the clock: the last -ff at 21:35.6, the
			// clock stopped at 21:36.49, everyone left 21:42–21:58.
			name: "forfeit by the clock, everyone left after the votes",
			replay: endedAt(match("21:58", []string{"21:42", "21:44", "21:46", "21:47", "21:49", "21:53", "21:58", "21:58"},
				"1 20:45 -ff", "2 21:00 -ff", "3 21:35.6 -ff"), "21:36.49"),
			ending: follow.EndedForfeit,
		},
		{
			// The real bot game: a king died, the clock stopped at 20:54.37
			// and the player left at 21:07.8.
			name:   "a king died, the player left after the end",
			replay: endedAt(match("21:08", []string{"21:07.8"}), "20:54.37"),
		},
		{
			name:   "a king died, a leave before the final round",
			replay: endedAt(match("21:08", []string{"16:00", "21:07.8"}), "20:54.37"),
			want:   []string{"Player#1001"},
		},
		{
			// By the leave cluster (21:07.8) 17:00 would be early; by the
			// clock it is in the final round.
			name:   "the clock decides the end over the leaves",
			replay: endedAt(match("21:08", []string{"17:00", "21:07.8"}), "20:54.37"),
		},
		{
			name:   "without the clock the leaves decide",
			replay: match("21:08", []string{"17:00", "21:07.8"}),
			want:   []string{"Player#1001"},
		},
		{
			name:   "no leave records at all",
			replay: match("38:05", []string{"", "", "", ""}),
		},
		{
			// The recording went on 26 minutes after the only leave: the
			// match did too, so that leave was early.
			name:   "a lone leave long before the recording ends is early",
			replay: match("38:05", []string{"12:00", "", "", ""}),
			want:   []string{"Player#1001"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := follow.EarlyLeavers(c.replay, finalRound)
			if !slices.Equal(leaverTags(got), c.want) || got.Ending != c.ending {
				t.Fatalf("leavers = %v (ending %q), want %v (%q); end %v", leaverTags(got), got.Ending, c.want, c.ending, got.End)
			}
		})
	}
}

func TestEarlyLeaversCountOnlyHumansWithABattleTag(t *testing.T) {
	r := match("38:05", append([]string{"12:00", "12:00", "12:00", "12:00"}, lateEnd...))
	r.Players[1].Computer = true
	r.Players[2].Observer = true
	r.Players[3].BattleTag = "Computer"
	// A vote typed by a computer or an observer isn't a vote.
	r.Chat = []dto.ReplayChat{{Time: mmss("36:00"), PlayerID: 3, Message: "-draw"}}
	got := follow.EarlyLeavers(r, 4*time.Minute)
	if !slices.Equal(leaverTags(got), []string{"Player#1001"}) || got.Ending != follow.EndedPlayed {
		t.Fatalf("leavers = %v, ending %q", leaverTags(got), got.Ending)
	}
}

// leaverFixture is a connected engine with the friends list known, the
// player Self#1111 in a team with Friend#1234, and leavers switched on.
func leaverFixture(t *testing.T) *fixture {
	t.Helper()
	f := newFixture(t)
	f.time = time.Date(2026, time.October, 31, 21, 30, 0, 0, time.Local)
	f.receive("FriendsFriendData", map[string]any{"data": map[string]any{"friends": []any{
		friend(map[string]any{"battleTag": "Buddy#2222"}),
		friend(map[string]any{"battleTag": "Stranger#3333", "isFriend": false}),
	}}})
	if err := f.engine.Configure(dto.FollowConfig{BlacklistLeavers: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	return f
}

func report(tags ...string) follow.LeaverReport {
	r := follow.LeaverReport{End: 37 * time.Minute, Length: 38*time.Minute + 5*time.Second}
	for _, tag := range tags {
		r.Leavers = append(r.Leavers, follow.Leaver{Tag: tag, Left: 12*time.Minute + 40*time.Second, Round: 3})
	}
	return r
}

func TestAddLeaversNeverBlacklistsTheExcluded(t *testing.T) {
	f := leaverFixture(t)
	if err := f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Chosen#5555"}}); err != nil {
		t.Fatal(err)
	}
	f.blacklist("Known#6666")
	f.engine.TakeBlacklistChanged()
	added := f.engine.AddLeavers(report(
		"Self#1111",     // the player
		"Buddy#2222",    // a Battle.net friend
		"Friend#1234",   // in the team, so in the follow group
		"Chosen#5555",   // chosen to follow
		"Known#6666",    // already blacklisted
		"Computer",      // not a BattleTag
		"Stranger#3333", // on the list the game sent, not a friend
	), "Legion TD")
	if len(added) != 1 || added[0].Tag != "Stranger#3333" {
		t.Fatalf("added = %+v", added)
	}
	if !f.engine.TakeBlacklistChanged() || f.state().Blacklist[0].Tag != "Stranger#3333" {
		t.Fatalf("blacklist = %+v", f.state().Blacklist)
	}
}

func TestAddLeaversReasonAndActivity(t *testing.T) {
	f := leaverFixture(t)
	added := f.engine.AddLeavers(report("Leaver#1234"), "Legion TD")
	want := "Left Legion TD early (round 3)"
	if len(added) != 1 || added[0].Reason != want || f.state().Blacklist[0].Reason != want {
		t.Fatalf("added = %+v", added)
	}
	if s := f.state().Status; s != "Blacklisted Leaver#1234 for leaving Legion TD early (round 3)." {
		t.Fatalf("status = %q", s)
	}
	f.engine.AddLeavers(report("Leaver#2001", "Leaver#2002"), "Legion TD")
	if s := f.state().Status; s != "Blacklisted 2 players for leaving Legion TD early." {
		t.Fatalf("status = %q", s)
	}
}

func TestLeftEarlySaysTheRoundOrElseTheMinute(t *testing.T) {
	if got := follow.LeftEarly("Legion TD", follow.Leaver{Left: 4 * time.Minute, Round: 2}); got != "Left Legion TD early (round 2)" {
		t.Errorf("with rounds: %q", got)
	}
	if got := follow.LeftEarly("Legion TD", follow.Leaver{Left: 12*time.Minute + 40*time.Second}); got != "Left Legion TD early (minute 13)" {
		t.Errorf("without rounds: %q", got)
	}
}

func TestAddLeaversNeedsTheSwitchAndTheFriendsList(t *testing.T) {
	f := leaverFixture(t)
	if err := f.engine.Configure(dto.FollowConfig{BlacklistLeavers: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if added := f.engine.AddLeavers(report("Leaver#1234"), "Legion TD"); added != nil || f.state().BlacklistLeavers {
		t.Fatalf("off: added %+v", added)
	}
	// Without the friends list, a friend can't be told apart.
	g := newFixture(t)
	if err := g.engine.Configure(dto.FollowConfig{BlacklistLeavers: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if added := g.engine.AddLeavers(report("Leaver#1234"), "Legion TD"); added != nil {
		t.Fatalf("no friends list: added %+v", added)
	}
	h := leaverFixture(t)
	h.engine.Connection(false, "")
	if added := h.engine.AddLeavers(report("Leaver#1234"), "Legion TD"); added != nil {
		t.Fatalf("disconnected: added %+v", added)
	}
}

func TestABanByHandGetsAReason(t *testing.T) {
	f := newFixture(t)
	f.time = time.Date(2026, time.October, 31, 21, 30, 0, 0, time.Local)
	lobby := hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"}))
	lobby["lobbyName"] = "LTD x20 come"
	f.receive("GameLobbySetup", lobby)
	f.receive("ChatMessage", banned("Player#1234"))
	got := f.state().Blacklist
	if len(got) != 1 || got[0].Reason != "You banned them in LTD x20 come, 31 Oct 2026" {
		t.Fatalf("blacklist = %+v", got)
	}
}

func TestBlacklistReasonsSurviveReplacing(t *testing.T) {
	f := newFixture(t)
	list := []dto.BlacklistEntry{{Tag: "Player#1234", Added: f.time, Reason: " Left  early:\nLegion TD "}, {Tag: "Arthas#1123", Added: f.time}}
	if err := f.engine.Configure(dto.FollowConfig{Blacklist: &list}); err != nil {
		t.Fatal(err)
	}
	got := f.state().Blacklist
	if len(got) != 2 || got[0].Reason != "Left early: Legion TD" || got[1].Reason != "" {
		t.Fatalf("blacklist = %+v", got)
	}
}

func TestAppendNote(t *testing.T) {
	line := "Left early: Legion TD 11.4c-hf9, 12 min into a 38 min match, 31 Oct 2026"
	if got, ok := follow.AppendNote("", line); !ok || got != line {
		t.Fatalf("empty: %q %v", got, ok)
	}
	if got, ok := follow.AppendNote("Feeds.", line); !ok || got != "Feeds.\n"+line {
		t.Fatalf("append: %q %v", got, ok)
	}
	if got, ok := follow.AppendNote("Feeds.\n"+line, line); ok || got != "Feeds.\n"+line {
		t.Fatalf("again: %q %v", got, ok)
	}
	long := strings.Repeat("a", dto.MaxNoteLength-len(line))
	if got, ok := follow.AppendNote(long, line); ok || got != long {
		t.Fatalf("too long: %v", ok)
	}
}

// withRounds sets the replay's round starts, written m:ss.
func withRounds(r dto.Replay, starts ...string) dto.Replay {
	for _, s := range starts {
		r.Rounds = append(r.Rounds, mmss(s))
	}
	return r
}

func TestEarlyLeaversUseTheRealRounds(t *testing.T) {
	// The real forfeit game: seven rounds, the clock stopped at 21:36.49.
	rounds := []string{"0:02.9", "3:11.5", "5:08.9", "8:32.0", "12:23.6", "15:31.7", "19:36.2"}
	everyone := []string{"21:44", "21:58", "21:58", "21:49", "21:46", "21:42", "21:53", "21:47"}
	votes := []string{"1 20:45 -ff", "4 21:07 -ff", "2 21:15 -ff", "3 21:35 -ff"}
	cases := []struct {
		name   string
		leaves []string
		votes  []string
		want   []string
		round  int
	}{
		{"real forfeit: nobody", everyone, votes, nil, 0},
		// 18:00 is inside the fixed 4-minute window (from 17:36) but before
		// the real final round (19:36): round 6, early.
		{"left in round 6 is early", replace(everyone, 5, "18:00"), nil, []string{"Player#1006"}, 6},
		{"left in the final round is not", replace(everyone, 5, "19:40"), nil, nil, 0},
		{"left in the final round before the votes is", replace(everyone, 5, "20:30"), votes, []string{"Player#1006"}, 7},
		{"left in round 1", replace(everyone, 5, "1:00"), nil, []string{"Player#1006"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := follow.EarlyLeavers(withRounds(endedAt(match("21:58", c.leaves, c.votes...), "21:36.49"), rounds...), 4*time.Minute)
			if got := leaverTags(r); !slices.Equal(got, c.want) {
				t.Fatalf("leavers = %v, want %v", got, c.want)
			}
			if r.FinalRound != mmss("19:36.2") {
				t.Errorf("final round = %v", r.FinalRound)
			}
			if len(c.want) == 1 && r.Leavers[0].Round != c.round {
				t.Errorf("round = %d, want %d", r.Leavers[0].Round, c.round)
			}
		})
	}
}

func TestEarlyLeaversWithoutRoundsUseTheFixedWindow(t *testing.T) {
	r := follow.EarlyLeavers(match("38:05", replace(lateEnd, 0, "34:30")), 4*time.Minute)
	if len(r.Leavers) != 0 || r.FinalRound != 0 {
		t.Fatalf("leavers = %v, final round %v", leaverTags(r), r.FinalRound)
	}
	r = follow.EarlyLeavers(match("38:05", replace(lateEnd, 0, "12:00")), 4*time.Minute)
	if got := leaverTags(r); len(got) != 1 || r.Leavers[0].Round != 0 {
		t.Fatalf("leavers = %+v", r.Leavers)
	}
}

// replace returns leaves with entry i set to left.
func replace(leaves []string, i int, left string) []string {
	out := slices.Clone(leaves)
	out[i] = left
	return out
}

func TestEarlyLeaversOneLeaveMidGameIsNotTheEnd(t *testing.T) {
	// An older replay: one player left at 2:47, the game was recorded to
	// 16:45 with no leave for the others and no stopped clock.
	r := withRounds(match("16:45", []string{"2:47", "", "", ""}), "0:04.8", "3:29.4", "6:00.6", "10:12.9", "13:37.3", "15:47.2")
	rep := follow.EarlyLeavers(r, 4*time.Minute)
	if rep.End != mmss("16:45") || len(rep.Leavers) != 1 || rep.Leavers[0].Round != 1 {
		t.Fatalf("end %v, leavers %+v", rep.End, rep.Leavers)
	}
}
