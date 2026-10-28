package follow_test

import (
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// hosted marks a lobby update as one the player hosts.
func hosted(p map[string]any) map[string]any {
	p["isHost"] = true
	p["playerHost"] = "Self#1111"
	return p
}

func (f *fixture) blacklist(tags ...string) {
	f.t.Helper()
	if err := f.engine.Configure(dto.FollowConfig{BlacklistAdd: tags}); err != nil {
		f.t.Fatalf("blacklist: %v", err)
	}
}

func (f *fixture) bans() []any {
	var slots []any
	for _, c := range f.commands {
		if c.message == follow.CmdBanPlayer {
			slots = append(slots, c.payload["slot"])
		}
	}
	return slots
}

func TestBansABlacklistedPlayerWhoJoinsMyLobbyOnce(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"}, seat{name: "Griefer#4242"})))
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"}, seat{name: "Griefer#4242"})))
	if got := f.bans(); len(got) != 1 || got[0] != 2 {
		t.Fatalf("bans = %v", got)
	}
	if s := f.state().Status; s != "Banned Griefer#4242 (on your blacklist)." {
		t.Fatalf("status = %q", s)
	}
}

func TestNeverBansInALobbyIDontHost(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	p := lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})
	p["isHost"], p["playerHost"] = false, "Griefer#4242"
	f.receive("GameLobbySetup", p)
	if got := f.bans(); len(got) != 0 {
		t.Fatalf("bans = %v", got)
	}
}

func TestWaitsForAJoiningPlayersBattleTag(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	joining := hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"}, seat{name: "Griefer#4242"}))
	slot := joining["players"].([]any)[2].(map[string]any)
	slot["name"] = ""
	f.receive("GameLobbySetup", joining)
	if got := f.bans(); len(got) != 0 {
		t.Fatalf("banned a blank name: %v", got)
	}
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"}, seat{name: "Griefer#4242"})))
	if got := f.bans(); len(got) != 1 || got[0] != 2 {
		t.Fatalf("bans = %v", got)
	}
}

func TestMatchesBattleTagsWhateverTheirCase(t *testing.T) {
	f := newFixture(t)
	f.blacklist("griefer#4242")
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "GRIEFER#4242"})))
	if got := f.bans(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("bans = %v", got)
	}
}

func TestBansAPlayerAgainWhenTheyRejoin(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})))
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{})))
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{}, seat{name: "Griefer#4242"})))
	if got := f.bans(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("bans = %v", got)
	}
}

func TestSendsNoBanOnceTheCountdownLocksOrTheMatchStarts(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{})))
	f.receive("LobbyCountdown", countdown(true, true))
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})))
	if got := f.bans(); len(got) != 0 {
		t.Fatalf("banned during a locked countdown: %v", got)
	}

	g := newFixture(t)
	g.blacklist("Griefer#4242")
	g.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{})))
	g.receive("SetGlueScreen", map[string]any{"screen": "LOADING_SCREEN"})
	g.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})))
	if got := g.bans(); len(got) != 0 {
		t.Fatalf("banned after the match started: %v", got)
	}
}

func TestBansWhileTheCountdownRunsUnlocked(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	f.receive("LobbyCountdown", countdown(true, false))
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})))
	if got := f.bans(); len(got) != 1 {
		t.Fatalf("bans = %v", got)
	}
}

func TestAutoBanOffKeepsTheListAndBansNobody(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	if err := f.engine.Configure(dto.FollowConfig{AutoBan: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})))
	if got := f.bans(); len(got) != 0 {
		t.Fatalf("bans = %v", got)
	}
	if s := f.state(); s.AutoBan || len(s.Blacklist) != 1 {
		t.Fatalf("state = %v %v", s.AutoBan, s.Blacklist)
	}
}

func TestNeverBansMyOwnSlotComputersOrOpenSlots(t *testing.T) {
	f := newFixture(t)
	if err := f.engine.Configure(dto.FollowConfig{Blacklist: &[]dto.BlacklistEntry{{Tag: "Self#1111"}, {Tag: "Computer#1234"}}}); err != nil {
		t.Fatal(err)
	}
	p := hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Computer#1234", computer: true}, seat{}))
	open := p["players"].([]any)[2].(map[string]any)
	open["name"] = "Computer#1234"
	f.receive("GameLobbySetup", p)
	if got := f.bans(); len(got) != 0 {
		t.Fatalf("bans = %v", got)
	}
}

func TestRefusesMyOwnBattleTagAndBadTags(t *testing.T) {
	f := newFixture(t)
	wantErr(t, f.engine.Configure(dto.FollowConfig{BlacklistAdd: []string{"self#1111"}}), "yourself")
	wantErr(t, f.engine.Configure(dto.FollowConfig{BlacklistAdd: []string{"Griefer"}}), "full BattleTag")
	if got := f.state().Blacklist; len(got) != 0 {
		t.Fatalf("blacklist = %v", got)
	}
}

func TestBlacklistingTakesPlayersOutOfTheFollowGroup(t *testing.T) {
	f := newFixture(t)
	if err := f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Friend#1234", "Player#1234"}}); err != nil {
		t.Fatal(err)
	}
	f.advance(time.Minute)
	f.blacklist("player#1234", "Griefer#4242", "Griefer#4242")
	s := f.state()
	if len(s.ManualTags) != 1 || s.ManualTags[0] != "Friend#1234" {
		t.Fatalf("group = %v", s.ManualTags)
	}
	if len(s.Blacklist) != 2 || s.Blacklist[0].Tag != "player#1234" || !s.Blacklist[0].Added.Equal(f.time) {
		t.Fatalf("blacklist = %+v", s.Blacklist)
	}
	if s.Status != "Added 2 players to your blacklist." {
		t.Fatalf("status = %q", s.Status)
	}
	wantErr(t, f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Griefer#4242"}}), "blacklist")
	if err := f.engine.Configure(dto.FollowConfig{BlacklistRemove: []string{"GRIEFER#4242"}}); err != nil {
		t.Fatal(err)
	}
	if got := f.state().Blacklist; len(got) != 1 {
		t.Fatalf("after remove = %+v", got)
	}
}

// banned is the game's ban confirmation, sent as the local player (the
// host), whose display name is the name part of Self#1111.
func banned(tag string) map[string]any { return bannedBy("Self", tag) }

// bannedBy is a lobby chat line "Host banned player [tag]" from sender;
// "" leaves the sender out.
func bannedBy(sender, tag string) map[string]any {
	m := map[string]any{"source": "gameChat", "type": "chat", "content": "Host banned player [" + tag + "]"}
	if sender != "" {
		m["sender"] = sender
	}
	return map[string]any{"message": m}
}

func TestABanLineFromAGuestIsIgnored(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	f.engine.TakeBlacklistChanged()
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"}, seat{name: "Victim#1234"})))
	// A guest types the confirmation text in the lobby chat, naming
	// someone else, while our ban of Griefer#4242 waits for its own.
	f.receive("ChatMessage", bannedBy("Victim#1234", "Victim#1234"))
	f.receive("ChatMessage", bannedBy("Victim", "Victim#1234"))
	s := f.state()
	if len(s.Blacklist) != 1 || f.engine.TakeBlacklistChanged() {
		t.Fatalf("a guest's line changed the blacklist: %+v", s.Blacklist)
	}
	if got := f.engine.TakeBanMistakes(); len(got) != 0 {
		t.Fatalf("a guest's line raised a mistake: %v", got)
	}
	if got := f.engine.TakeIgnoredBanLines(); len(got) != 2 || got[0] != "Victim#1234" || got[1] != "Victim" {
		t.Fatalf("ignored = %v", got)
	}
	// Our ban still waits: the real confirmation is checked against it.
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Victim#1234"}, seat{})))
	f.receive("ChatMessage", banned("Victim#1234"))
	if got := f.engine.TakeBanMistakes(); len(got) != 1 {
		t.Fatalf("the waiting ban was touched by the guest's line: %v", got)
	}
}

func TestABanLineWithoutASenderIsIgnored(t *testing.T) {
	f := newFixture(t)
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"})))
	f.receive("ChatMessage", bannedBy("", "Player#1234"))
	if got := f.state().Blacklist; len(got) != 0 {
		t.Fatalf("blacklist = %+v", got)
	}
	if got := f.engine.TakeIgnoredBanLines(); len(got) != 1 || got[0] != "" {
		t.Fatalf("ignored = %q", got)
	}
}

func TestABanLineFromMeIsTrustedWithAnyCaseTagOrClan(t *testing.T) {
	for _, sender := range []string{"Self", "self#1111", "<CLAN> Self", "<Ab> SELF#1111"} {
		f := newFixture(t)
		f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"})))
		f.receive("ChatMessage", bannedBy(sender, "Player#1234"))
		if got := f.state().Blacklist; len(got) != 1 {
			t.Errorf("sender %q: blacklist = %+v", sender, got)
		}
		if got := f.engine.TakeIgnoredBanLines(); len(got) != 0 {
			t.Errorf("sender %q: ignored = %v", sender, got)
		}
	}
}

func TestBansOnlyFromTheNewestLobbyUpdateNeverFromATimer(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	if err := f.engine.Configure(dto.FollowConfig{AutoBan: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})))
	if err := f.engine.Configure(dto.FollowConfig{AutoBan: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	f.advance(10 * time.Second)
	f.engine.Tick()
	if got := f.bans(); len(got) != 0 {
		t.Fatalf("banned from an old snapshot: %v", got)
	}
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"}, seat{name: "Griefer#4242"})))
	if got := f.bans(); len(got) != 1 || got[0] != 2 {
		t.Fatalf("bans = %v", got)
	}
}

func TestSendsOneBanAtATime(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242", "Arthas#1123")
	both := func() map[string]any {
		return hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"}, seat{name: "Arthas#1123"}))
	}
	f.receive("GameLobbySetup", both())
	if got := f.bans(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("first update: bans = %v", got)
	}
	// The game hasn't handled the ban yet: the update still shows them.
	f.receive("GameLobbySetup", both())
	if got := f.bans(); len(got) != 1 {
		t.Fatalf("sent a second ban while one was in flight: %v", got)
	}
	f.receive("ChatMessage", banned("Griefer#4242"))
	if got := f.bans(); len(got) != 1 {
		t.Fatalf("banned on the confirmation itself: %v", got)
	}
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{}, seat{name: "Arthas#1123"})))
	if got := f.bans(); len(got) != 2 || got[1] != 2 {
		t.Fatalf("after the confirmation: bans = %v", got)
	}
}

func TestAnUnansweredBanStopsBlockingAfterAShortWait(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242", "Arthas#1123")
	both := hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"}, seat{name: "Arthas#1123"}))
	f.receive("GameLobbySetup", both)
	f.advance(time.Second)
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"}, seat{name: "Arthas#1123"})))
	if got := f.bans(); len(got) != 1 {
		t.Fatalf("bans = %v", got)
	}
	f.advance(3 * time.Second)
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"}, seat{name: "Arthas#1123"})))
	if got := f.bans(); len(got) != 2 || got[1] != 2 {
		t.Fatalf("after the wait: bans = %v", got)
	}
}

func TestWarnsWhenTheGameBannedSomeoneElse(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})))
	f.receive("ChatMessage", banned("Player#5678"))
	want := "Banned Player#5678 by mistake: Griefer#4242 left just before the ban."
	if s := f.state().Status; s != want {
		t.Fatalf("status = %q", s)
	}
	if got := f.engine.TakeBanMistakes(); len(got) != 1 || got[0] != want {
		t.Fatalf("mistakes = %v", got)
	}
	if got := f.engine.TakeBanMistakes(); len(got) != 0 {
		t.Fatalf("mistakes taken twice: %v", got)
	}
}

func TestAConfirmationMatchingTheBanIsQuiet(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	if err := f.engine.Configure(dto.FollowConfig{BlacklistManualBans: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})))
	f.receive("ChatMessage", banned("GRIEFER#4242"))
	// A ban the player made by hand, with no ban of ours waiting.
	f.receive("ChatMessage", banned("Player#5678"))
	if s := f.state().Status; s != "Banned Griefer#4242 (on your blacklist)." {
		t.Fatalf("status = %q", s)
	}
	if got := f.engine.TakeBanMistakes(); len(got) != 0 {
		t.Fatalf("mistakes = %v", got)
	}
}

func TestAPendingBanFollowsTheNewestLobbyUpdate(t *testing.T) {
	update := func(f *fixture, p map[string]any) {
		f.engine.Receive(dto.GameEvent{MessageType: "GameLobbySetup", Payload: p})
	}
	moved := newFixture(t)
	moved.blacklist("Griefer#4242")
	update(moved, hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"}, seat{})))
	update(moved, hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#5678"}, seat{name: "Griefer#4242"})))
	moved.engine.SendPendingBan()
	if got := moved.bans(); len(got) != 1 || got[0] != 2 {
		t.Fatalf("moved: bans = %v", got)
	}

	left := newFixture(t)
	left.blacklist("Griefer#4242")
	update(left, hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})))
	update(left, hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#5678"})))
	left.engine.SendPendingBan()
	if got := left.bans(); len(got) != 0 || left.engine.HasPendingBan() {
		t.Fatalf("left: bans = %v", got)
	}
	// They come back: banned again.
	left.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#5678"}, seat{name: "Griefer#4242"})))
	if got := left.bans(); len(got) != 1 || got[0] != 2 {
		t.Fatalf("rejoined: bans = %v", got)
	}
}

func TestAPlayerIBanByHandJoinsTheBlacklist(t *testing.T) {
	f := newFixture(t)
	if err := f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Player#1234"}}); err != nil {
		t.Fatal(err)
	}
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"})))
	f.receive("ChatMessage", banned("Player#1234"))
	s := f.state()
	if len(s.Blacklist) != 1 || s.Blacklist[0].Tag != "Player#1234" || !s.Blacklist[0].Added.Equal(f.time) || len(s.ManualTags) != 0 {
		t.Fatalf("blacklist = %+v, group = %v", s.Blacklist, s.ManualTags)
	}
	if s.Status != "Added Player#1234 to your blacklist (you banned them)." {
		t.Fatalf("status = %q", s.Status)
	}
	if !f.engine.TakeBlacklistChanged() || f.engine.TakeBlacklistChanged() {
		t.Fatal("the change wasn't reported once for saving")
	}
	// Banned again in a later lobby: already listed, nothing changes.
	f.receive("ChatMessage", banned("player#1234"))
	if got := f.state().Blacklist; len(got) != 1 || f.engine.TakeBlacklistChanged() {
		t.Fatalf("added twice: %+v", got)
	}
}

func TestMyOwnBansAreNotAddedTwice(t *testing.T) {
	f := newFixture(t)
	f.blacklist("Griefer#4242")
	f.engine.TakeBlacklistChanged()
	f.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Griefer#4242"})))
	f.receive("ChatMessage", banned("Griefer#4242"))
	if got := f.state().Blacklist; len(got) != 1 || f.engine.TakeBlacklistChanged() {
		t.Fatalf("blacklist = %+v", got)
	}
	if s := f.state().Status; s != "Banned Griefer#4242 (on your blacklist)." {
		t.Fatalf("status = %q", s)
	}
}

func TestBansByHandAreIgnoredWhenNotHostingOrSwitchedOffOrMalformed(t *testing.T) {
	notHost := newFixture(t)
	p := lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"})
	p["isHost"] = false
	notHost.receive("GameLobbySetup", p)
	notHost.receive("ChatMessage", banned("Player#1234"))
	if got := notHost.state().Blacklist; len(got) != 0 {
		t.Fatalf("not host: %+v", got)
	}

	off := newFixture(t)
	if err := off.engine.Configure(dto.FollowConfig{BlacklistManualBans: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	off.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"})))
	off.receive("ChatMessage", banned("Player#1234"))
	if s := off.state(); len(s.Blacklist) != 0 || s.BlacklistManualBans {
		t.Fatalf("switched off: %+v", s.Blacklist)
	}

	bad := newFixture(t)
	bad.receive("GameLobbySetup", hosted(lobbyWith(seat{name: "Self#1111"}, seat{name: "Player#1234"})))
	bad.receive("ChatMessage", banned("Player"))
	bad.receive("ChatMessage", banned("Self#1111"))
	bad.receive("ChatMessage", map[string]any{"message": map[string]any{"source": "whisper", "content": "Host banned player [Arthas#1123]"}})
	if got := bad.state().Blacklist; len(got) != 0 {
		t.Fatalf("malformed: %+v", got)
	}
}

func TestSavedBlacklistLoadsNewestFirst(t *testing.T) {
	f := newFixture(t)
	old, recent := time.Unix(100, 0).UTC(), time.Unix(200, 0).UTC()
	saved := []dto.BlacklistEntry{{Tag: "Arthas#1123", Added: old}, {Tag: "Player#1234", Added: recent}, {Tag: "arthas#1123", Added: recent}}
	if err := f.engine.Configure(dto.FollowConfig{Blacklist: &saved}); err != nil {
		t.Fatal(err)
	}
	got := f.state().Blacklist
	if len(got) != 2 || got[0].Tag != "Player#1234" || got[1].Tag != "Arthas#1123" {
		t.Fatalf("blacklist = %+v", got)
	}
}
