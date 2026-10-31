package companion_test

import (
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/companion"
	mocknotes "github.com/mihaiflorentin/wc3-team-follow/mock/notes"
	mockprefs "github.com/mihaiflorentin/wc3-team-follow/mock/preferences"
	mockranking "github.com/mihaiflorentin/wc3-team-follow/mock/ranking"
	mockrecent "github.com/mihaiflorentin/wc3-team-follow/mock/recent"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const legionTD = `Maps\Download\Legion_TD_11.4c-hf9_Team_OZE.w3x`

func leaverOptions(o *companion.Options) {
	o.TrackedMaps = []dto.TrackedMap{{Name: "Legion TD", Repository: "team-oze/Legion-TD",
		Pattern: `(?i)^Legion_TD_(?P<version>\d[^_/\\ ]*)_Team_?OZE(?: \(\d+\))?\.w3x$`, Folder: "Download"}}
	o.LeaverMap = "Legion TD"
	o.LeaverFinalRound = 4 * time.Minute
	o.ReplayPoll = 5 * time.Millisecond
	o.ReplayWait = 2 * time.Second
}

func leaverPrefs(on bool) *mockprefs.Store {
	return mockprefs.New(dto.Preferences{Tags: []string{}, IncludeTeam: true, JoinHostTeam: true, AutoBan: true, BlacklistManualBans: true,
		Blacklist: []dto.BlacklistEntry{}, BlacklistLeavers: on})
}

// legionReplay is a 38 minute match of the player, Self#1111, in which
// Leaver#1234 left at 12:40 and Buddy#2222, a friend, at 12:00; the rest
// left together at the end.
func legionReplay(mapPath string) dto.Replay {
	r := dto.Replay{GameTime: 38*time.Minute + 5*time.Second, Map: dto.ReplayMap{Path: mapPath}}
	tags := []string{"Self#1111", "Leaver#1234", "Buddy#2222", "Player#3001", "Player#3002"}
	left := []time.Duration{38 * time.Minute, 12*time.Minute + 40*time.Second, 12 * time.Minute, 37*time.Minute + 50*time.Second, 38 * time.Minute}
	for i, tag := range tags {
		r.Players = append(r.Players, dto.ReplayPlayer{ID: i + 1, BattleTag: tag, Team: i % 2})
		r.Leaves = append(r.Leaves, dto.ReplayLeave{Time: left[i], PlayerID: i + 1})
	}
	r.Players = append(r.Players, dto.ReplayPlayer{ID: 9, Name: "Computer", Computer: true})
	return r
}

func newLeaverHarness(t *testing.T, on bool, notes *mocknotes.Store) *harness {
	t.Helper()
	h := newHarnessFull(t, leaverPrefs(on), mockrecent.New(), notes, mockranking.New(), leaverOptions)
	h.session.Push(dto.GameEvent{MessageType: "TeamsInformation", Payload: map[string]any{"message": map[string]any{
		"partyMembers": []any{map[string]any{"battleTag": "Self#1111", "isSelf": true}}}}})
	h.session.Push(dto.GameEvent{MessageType: "FriendsFriendData", Payload: map[string]any{"data": map[string]any{"friends": []any{
		map[string]any{"battleTag": "Buddy#2222", "isFriend": true, "isOnline": true}}}}})
	h.run(t)
	waitFor(t, "the player's tag", func() bool { return h.svc.State().SelfTag == "Self#1111" })
	return h
}

// playMatch has the game enter a match and leave it for the score screen.
func playMatch(t *testing.T, h *harness) {
	t.Helper()
	h.session.Push(dto.GameEvent{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": "LOADING_SCREEN"}})
	waitFor(t, "the match", func() bool { return h.svc.State().InMatch })
	h.session.Push(dto.GameEvent{MessageType: "SetGlueScreen", Payload: map[string]any{"screen": "SCORE_SCREEN"}})
	waitFor(t, "the end of the match", func() bool { return !h.svc.State().InMatch })
}

func replayFile(modified time.Time) dto.ReplayFile {
	return dto.ReplayFile{Path: "/fake/BattleNet/12345678/Replays/LastReplay.w3g", Modified: modified, Size: 4096}
}

func TestLeaversOffReadsNoReplay(t *testing.T) {
	h := newLeaverHarness(t, false, mocknotes.New(nil))
	h.replayLoc.Set(replayFile(time.Now().Add(time.Hour)), nil)
	h.replays.Set(legionReplay(legionTD))
	playMatch(t, h)
	time.Sleep(100 * time.Millisecond)
	if h.replayLoc.Calls() != 0 || h.replays.Reads() != 0 || len(h.svc.State().Blacklist) != 0 {
		t.Fatalf("looked %d times, read %d", h.replayLoc.Calls(), h.replays.Reads())
	}
}

func TestLeaversOnBlacklistsTheEarlyLeaverWithANote(t *testing.T) {
	notes := mocknotes.New(map[string]dto.PlayerNote{"leaver#1234": {Text: "Feeds.", Updated: time.Unix(100, 0)}})
	h := newLeaverHarness(t, true, notes)
	h.replays.Set(legionReplay(legionTD))
	playMatch(t, h)
	// The replay appears only after the match, and is read once it is steady.
	h.replayLoc.Set(replayFile(time.Now().Add(time.Second)), nil)
	waitFor(t, "the leaver", func() bool { return len(h.svc.State().Blacklist) == 1 })
	s := h.svc.State()
	reason := "Left Legion TD early (minute 13)"
	if b := s.Blacklist[0]; b.Tag != "Leaver#1234" || b.Reason != reason {
		t.Fatalf("blacklist = %+v", s.Blacklist)
	}
	if got := notes.Saved()["leaver#1234"].Text; got != "Feeds.\n"+reason {
		t.Fatalf("note = %q", got)
	}
	if s.Status != "Blacklisted Leaver#1234 for leaving Legion TD early (minute 13)." {
		t.Fatalf("status = %q", s.Status)
	}
	if saved, _ := h.prefs.Load(); len(saved.Blacklist) != 1 || saved.Blacklist[0].Reason != reason || !saved.BlacklistLeavers {
		t.Fatalf("saved %+v", saved)
	}
	if h.replays.LastPath != replayFile(time.Time{}).Path {
		t.Fatalf("read %q", h.replays.LastPath)
	}
}

func TestLeaversReadTheSameReplayOnce(t *testing.T) {
	h := newLeaverHarness(t, true, mocknotes.New(nil))
	h.replays.Set(legionReplay(legionTD))
	h.replayLoc.Set(replayFile(time.Now().Add(time.Hour)), nil)
	playMatch(t, h)
	waitFor(t, "the leaver", func() bool { return len(h.svc.State().Blacklist) == 1 })
	// The next match leaves the same file behind: it isn't read again.
	if _, err := h.svc.Configure(dto.FollowConfig{BlacklistRemove: []string{"Leaver#1234"}}); err != nil {
		t.Fatal(err)
	}
	playMatch(t, h)
	waitFor(t, "another look", func() bool { return h.replayLoc.Calls() >= 4 })
	time.Sleep(50 * time.Millisecond)
	if h.replays.Reads() != 1 || len(h.svc.State().Blacklist) != 0 {
		t.Fatalf("read %d times, blacklist %+v", h.replays.Reads(), h.svc.State().Blacklist)
	}
}

func TestLeaversIgnoreOtherMapsAndOthersMatches(t *testing.T) {
	for name, replay := range map[string]dto.Replay{
		"another map": legionReplay(`Maps\Download\Footmen_Frenzy_5.4.w3x`),
		"not my match": func() dto.Replay {
			r := legionReplay(legionTD)
			r.Players[0].BattleTag = "Someone#9999"
			return r
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			h := newLeaverHarness(t, true, mocknotes.New(nil))
			h.replays.Set(replay)
			h.replayLoc.Set(replayFile(time.Now().Add(time.Hour)), nil)
			playMatch(t, h)
			waitFor(t, "the replay", func() bool { return h.replays.Reads() == 1 })
			time.Sleep(50 * time.Millisecond)
			s := h.svc.State()
			if len(s.Blacklist) != 0 || strings.Contains(s.Status, "Blacklisted") {
				t.Fatalf("blacklist = %+v (%q)", s.Blacklist, s.Status)
			}
		})
	}
}

func TestLeaversWaitForAReplayWrittenAfterTheMatchBegan(t *testing.T) {
	h := newLeaverHarness(t, true, mocknotes.New(nil))
	h.replays.Set(legionReplay(legionTD))
	// An old replay, from before this match.
	h.replayLoc.Set(replayFile(time.Now().Add(-time.Hour)), nil)
	playMatch(t, h)
	waitFor(t, "a few looks", func() bool { return h.replayLoc.Calls() >= 5 })
	if h.replays.Reads() != 0 {
		t.Fatal("an old replay was read")
	}
}
