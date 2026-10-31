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
			name:   "only the leaver has a record: they are the end",
			replay: match("38:05", []string{"12:00", "", "", ""}),
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
		r.Leavers = append(r.Leavers, follow.Leaver{Tag: tag, Left: 12*time.Minute + 40*time.Second})
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
	), "Legion TD", "11.4c-hf9")
	if len(added) != 1 || added[0].Tag != "Stranger#3333" {
		t.Fatalf("added = %+v", added)
	}
	if !f.engine.TakeBlacklistChanged() || f.state().Blacklist[0].Tag != "Stranger#3333" {
		t.Fatalf("blacklist = %+v", f.state().Blacklist)
	}
}

func TestAddLeaversReasonAndActivity(t *testing.T) {
	f := leaverFixture(t)
	added := f.engine.AddLeavers(report("Leaver#1234"), "Legion TD", "11.4c-hf9")
	want := "Left early: Legion TD 11.4c-hf9, 12 min into a 38 min match, 31 Oct 2026"
	if len(added) != 1 || added[0].Reason != want || f.state().Blacklist[0].Reason != want {
		t.Fatalf("added = %+v", added)
	}
	if s := f.state().Status; s != "Blacklisted Leaver#1234 for leaving Legion TD early (12 min into a 38 min match)." {
		t.Fatalf("status = %q", s)
	}
	f.engine.AddLeavers(report("Leaver#2001", "Leaver#2002"), "Legion TD", "11.4c-hf9")
	if s := f.state().Status; s != "Blacklisted 2 players for leaving Legion TD early." {
		t.Fatalf("status = %q", s)
	}
}

func TestAddLeaversReasonSaysHowAVoteEnded(t *testing.T) {
	for _, c := range []struct {
		ending string
		team   int
		want   string
	}{
		{follow.EndedDraw, 1, "that ended in a draw"},
		{follow.EndedForfeit, 0, "that their team forfeited"},
		{follow.EndedForfeit, 1, "that ended by forfeit"},
	} {
		f := leaverFixture(t)
		r := follow.LeaverReport{Length: 5 * time.Minute, Ending: c.ending, VoteTeam: c.team,
			Leavers: []follow.Leaver{{Tag: "Leaver#1234", Left: 3 * time.Minute, Team: 0}}}
		added := f.engine.AddLeavers(r, "Legion TD", "11.4c-hf9")
		want := "Left early: Legion TD 11.4c-hf9, 3 min into a 5 min match " + c.want + ", 31 Oct 2026"
		if len(added) != 1 || added[0].Reason != want {
			t.Fatalf("reason = %+v, want %q", added, want)
		}
	}
}

func TestAddLeaversNeedsTheSwitchAndTheFriendsList(t *testing.T) {
	f := leaverFixture(t)
	if err := f.engine.Configure(dto.FollowConfig{BlacklistLeavers: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if added := f.engine.AddLeavers(report("Leaver#1234"), "Legion TD", "1"); added != nil || f.state().BlacklistLeavers {
		t.Fatalf("off: added %+v", added)
	}
	// Without the friends list, a friend can't be told apart.
	g := newFixture(t)
	if err := g.engine.Configure(dto.FollowConfig{BlacklistLeavers: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	if added := g.engine.AddLeavers(report("Leaver#1234"), "Legion TD", "1"); added != nil {
		t.Fatalf("no friends list: added %+v", added)
	}
	h := leaverFixture(t)
	h.engine.Connection(false, "")
	if added := h.engine.AddLeavers(report("Leaver#1234"), "Legion TD", "1"); added != nil {
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
