package follow_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/domain/follow"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const verifiedBuild = "3.0.1.24371"

type command struct {
	message string
	payload map[string]any
}

type fixture struct {
	t        *testing.T
	engine   *follow.Engine
	time     time.Time
	commands []command
	// fail makes the sender report a closed connection for this command.
	fail string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, time: time.Unix(1000, 0)}
	f.engine = follow.NewEngine(func(message string, payload map[string]any) bool {
		f.commands = append(f.commands, command{message, payload})
		return message != f.fail
	}, func() time.Time { return f.time }, []string{verifiedBuild})
	f.engine.Connection(true, verifiedBuild)
	f.roster(member("Friend#1234"))
	return f
}

func (f *fixture) advance(d time.Duration) { f.time = f.time.Add(d) }

func (f *fixture) receive(kind string, payload map[string]any) {
	f.engine.Receive(dto.GameEvent{MessageType: kind, Payload: payload})
}

func member(tag string) map[string]any {
	return map[string]any{"battleTag": tag, "isSelf": false, "isFriend": true, "auroraId": 42}
}

func (f *fixture) roster(members ...map[string]any) {
	party := []any{map[string]any{"battleTag": "Self#1111", "isSelf": true}}
	for _, m := range members {
		party = append(party, m)
	}
	f.receive("TeamsInformation", map[string]any{"message": map[string]any{"partyMembers": party}})
}

func attrs(pairs ...string) []any {
	var out []any
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, map[string]any{"key": pairs[i], "value": pairs[i+1]})
	}
	return out
}

func friend(overrides map[string]any) map[string]any {
	f := map[string]any{
		"battleTag": "Friend#1234", "accountId": 42, "isOnline": true, "isFriend": true,
		"currentProgram": "W3", "localRichPresenceAttributes": attrs("currentGameState", "open"),
	}
	for k, v := range overrides {
		f[k] = v
	}
	return f
}

func (f *fixture) presence(overrides map[string]any) {
	f.receive("FriendsFriendData", map[string]any{"data": map[string]any{"friends": []any{friend(overrides)}}})
}

func (f *fixture) arm(diagnostic bool) error {
	return f.engine.Configure(dto.FollowConfig{Enabled: ptr(true), Diagnostic: ptr(diagnostic), FromMenus: true})
}

func (f *fixture) mustArm(diagnostic bool) {
	f.t.Helper()
	if err := f.arm(diagnostic); err != nil {
		f.t.Fatalf("arm: %v", err)
	}
}

func (f *fixture) count(message string) int {
	n := 0
	for _, c := range f.commands {
		if c.message == message {
			n++
		}
	}
	return n
}

func (f *fixture) joins() []command {
	var out []command
	for _, c := range f.commands {
		if c.message == follow.CmdJoinFriendGameLobby {
			out = append(out, c)
		}
	}
	return out
}

func (f *fixture) state() dto.FollowState { return f.engine.Snapshot() }

func ptr[T any](v T) *T { return &v }

func wantErr(t *testing.T, err error, fragment string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), fragment) {
		t.Fatalf("error = %v, want one containing %q", err, fragment)
	}
}

func lobby(name string) map[string]any {
	return map[string]any{"localRichPresenceAttributes": attrs("currentGameState", "open", "currentGameName", name)}
}

func TestArmExcludesSelfFromTheRoster(t *testing.T) {
	f := newFixture(t)
	f.mustArm(true)
	w := f.state().Watched
	if len(w) != 1 || w[0].BattleTag != "Friend#1234" {
		t.Fatalf("watched = %+v", w)
	}
}

func TestJoinsAnOpenLobbyWithTheAccountID(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(nil)
	got := f.commands[len(f.commands)-3:]
	want := []string{follow.CmdCancelTeamInvitations, follow.CmdTeamLeave, follow.CmdJoinFriendGameLobby}
	for i, c := range got {
		if c.message != want[i] {
			t.Fatalf("commands = %+v", got)
		}
	}
	if got[2].payload["auroraId"] != 42 {
		t.Errorf("auroraId = %v", got[2].payload["auroraId"])
	}
	if s := f.state(); !s.Enabled || s.LastJoin == nil || s.LastJoin.Confirmed {
		t.Errorf("state = %+v", s)
	}
}

func TestPreviewNeverLeavesTheTeamOrJoins(t *testing.T) {
	f := newFixture(t)
	f.mustArm(true)
	f.presence(nil)
	f.presence(nil)
	if len(f.joins()) != 0 {
		t.Fatal("preview joined")
	}
	if !strings.Contains(f.state().Status, "Preview mode didn't join") {
		t.Errorf("status = %q", f.state().Status)
	}
	for _, c := range f.commands {
		if c.message != follow.CmdFriendsGetFriends {
			t.Errorf("unexpected command %s", c.message)
		}
	}
}

func TestSwitchingModeNeedsANewStart(t *testing.T) {
	f := newFixture(t)
	f.mustArm(true)
	f.presence(nil)
	if err := f.engine.Configure(dto.FollowConfig{Diagnostic: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if f.state().Enabled || len(f.joins()) != 0 {
		t.Fatal("mode switch kept Follow on or joined")
	}
}

func TestIgnoresActivityCachedBeforeStarting(t *testing.T) {
	f := newFixture(t)
	f.presence(nil)
	f.mustArm(false)
	if len(f.joins()) != 0 {
		t.Fatal("acted on cached activity")
	}
	f.presence(nil)
	if len(f.joins()) != 1 {
		t.Fatal("did not join on fresh activity")
	}
}

func TestFollowsRememberedMembersAfterTheTeamDisbands(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.receive("TeamsDisbanded", nil)
	f.presence(nil)
	if len(f.joins()) != 1 || len(f.state().Watched) != 1 || f.count(follow.CmdTeamLeave) != 0 {
		t.Fatalf("joins=%d leaves=%d", len(f.joins()), f.count(follow.CmdTeamLeave))
	}
}

func TestDoesNotFollowMembersAddedAfterStarting(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.roster(member("Other#2222"))
	f.presence(map[string]any{"battleTag": "Other#2222"})
	if len(f.joins()) != 0 {
		t.Fatal("followed an outsider")
	}
}

func TestChoosesTeamOrderWhenTwoAreJoinable(t *testing.T) {
	f := newFixture(t)
	f.roster(member("Friend#1234"), member("Other#2222"))
	f.mustArm(false)
	f.receive("FriendsFriendData", map[string]any{"data": map[string]any{"friends": []any{
		friend(map[string]any{"battleTag": "Other#2222", "accountId": 77}), friend(nil),
	}}})
	j := f.joins()
	if len(j) != 1 || j[0].payload["auroraId"] != 42 {
		t.Fatalf("joins = %+v", j)
	}
}

func TestNeverChasesASecondLobbyWhileAJoinIsPending(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(nil)
	f.presence(nil)
	f.engine.Tick()
	if len(f.joins()) != 1 {
		t.Fatalf("joins = %d", len(f.joins()))
	}
	wantErr(t, f.arm(false), "pending")
}

func TestIgnoresUnjoinableActivity(t *testing.T) {
	variants := []map[string]any{
		{"isOnline": false}, {"isFriend": false}, {"currentProgram": "WoW"},
		{"accountId": nil}, {"accountId": 0}, {"accountId": "bad"}, {"accountId": "000"}, {"accountId": 1.5},
		{"localRichPresenceAttributes": attrs("currentGameState", "closed")},
		{"localRichPresenceAttributes": nil},
	}
	for _, v := range variants {
		f := newFixture(t)
		f.mustArm(false)
		f.presence(v)
		if len(f.joins()) != 0 {
			t.Errorf("joined with %v", v)
		}
	}
}

func TestAcceptsAccountIDsAsNumbersOrStrings(t *testing.T) {
	for _, id := range []any{json.Number("42"), float64(42), "42"} {
		f := newFixture(t)
		f.mustArm(false)
		f.presence(map[string]any{"accountId": id})
		if j := f.joins(); len(j) != 1 || j[0].payload["auroraId"] != id {
			t.Errorf("accountId %#v: joins = %+v", id, j)
		}
	}
}

func TestQueueLobbyLoadingAndMatchPauseWithoutSwitchingOff(t *testing.T) {
	events := []struct {
		kind    string
		payload map[string]any
	}{
		{"QueueEntered", nil}, {"GameLobbySetup", nil}, {"GameFound", nil}, {"PauseUI", nil},
		{"SetGlueScreen", map[string]any{"screen": "LOADING_SCREEN"}},
		{"IsGameUIActive", map[string]any{"isActive": "true"}},
		{"IsGameUIActive", map[string]any{"isActive": true}},
	}
	for _, ev := range events {
		f := newFixture(t)
		f.mustArm(false)
		f.receive(ev.kind, ev.payload)
		f.presence(nil)
		s := f.state()
		if len(f.joins()) != 0 || !s.Enabled || !s.Paused {
			t.Errorf("%s: joins=%d enabled=%v paused=%v", ev.kind, len(f.joins()), s.Enabled, s.Paused)
		}
	}
}

func TestStartingNeedsTeamFreshRosterKnownBuildConnectionAndMenus(t *testing.T) {
	f := newFixture(t)
	wantErr(t, f.engine.Configure(dto.FollowConfig{Enabled: ptr(true)}), "menus")
	f.roster()
	wantErr(t, f.arm(false), "Invite")
	f.roster(member("Friend#1234"))
	f.advance(21 * time.Second)
	wantErr(t, f.arm(false), "fresh")
	f.engine.Connection(false, "")
	wantErr(t, f.arm(false), "Start Warcraft")
	f.engine.Connection(true, "future-build")
	f.roster(member("Friend#1234"))
	wantErr(t, f.arm(false), "not been verified")
}

func TestStaleActivityIsNotJoinable(t *testing.T) {
	f := newFixture(t)
	f.mustArm(true)
	f.presence(nil)
	f.advance(21 * time.Second)
	if f.state().Watched[0].Open {
		t.Fatal("stale activity reported open")
	}
}

func TestDisconnectForgetsTheRosterAndSwitchesOff(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.engine.Connection(false, "")
	f.presence(nil)
	if len(f.joins()) != 0 || len(f.state().Watched) != 0 {
		t.Fatal("kept following after disconnect")
	}
}

func TestTimeoutsNeverRetryAndReportsHideIdentities(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(nil)
	f.advance(16 * time.Second)
	f.engine.Tick()
	f.presence(nil)
	if len(f.joins()) != 1 || f.state().JoinPending {
		t.Fatalf("joins=%d pending=%v", len(f.joins()), f.state().JoinPending)
	}
	report, _ := json.Marshal(f.engine.Diagnostics())
	for _, secret := range []string{"Friend#", "Self#", "auroraId", "accountId"} {
		if strings.Contains(string(report), secret) {
			t.Errorf("diagnostics contain %q: %s", secret, report)
		}
	}
}

func TestASendFailureStopsTheSequenceBeforeJoining(t *testing.T) {
	f := newFixture(t)
	f.fail = follow.CmdTeamLeave
	f.mustArm(false)
	f.presence(nil)
	if len(f.joins()) != 0 || !f.state().Enabled {
		t.Fatalf("joins=%d enabled=%v", len(f.joins()), f.state().Enabled)
	}
}

func TestMalformedEventsAndOtherTagsNeverJoin(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.engine.Receive(dto.GameEvent{})
	f.receive("TeamsInformation", map[string]any{"message": map[string]any{"partyMembers": nil}})
	f.presence(map[string]any{"battleTag": "Friend#9999"})
	f.receive("FriendsFriendUpdated", map[string]any{"data": map[string]any{}})
	f.receive("FriendsFriendData", map[string]any{"data": "nonsense"})
	if len(f.joins()) != 0 {
		t.Fatal("joined on malformed input")
	}
}

func TestManualFriendNeedsNoTeam(t *testing.T) {
	f := newFixture(t)
	f.roster()
	f.advance(25 * time.Second)
	f.presence(nil)
	if err := f.engine.Configure(dto.FollowConfig{Tags: &[]string{"friend#1234"}, IncludeTeam: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	f.mustArm(false)
	if len(f.state().Watched) != 1 || len(f.joins()) != 0 {
		t.Fatal("unexpected state after start")
	}
	f.presence(nil)
	if len(f.joins()) != 1 || f.count(follow.CmdTeamLeave) != 0 {
		t.Fatalf("joins=%d leaves=%d", len(f.joins()), f.count(follow.CmdTeamLeave))
	}
}

func TestManualTagsMergeWithTheTeamWithoutDuplicates(t *testing.T) {
	f := newFixture(t)
	f.presence(nil)
	if err := f.engine.Configure(dto.FollowConfig{Tags: &[]string{"friend#1234", "Friend#1234"}}); err != nil {
		t.Fatal(err)
	}
	f.mustArm(true)
	if len(f.engine.ManualTags()) != 1 || len(f.state().Watched) != 1 {
		t.Fatalf("tags=%v watched=%v", f.engine.ManualTags(), f.state().Watched)
	}
}

func TestInvalidUnknownSelfAndNonFriendTagsCannotStart(t *testing.T) {
	f := newFixture(t)
	wantErr(t, f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Friend"}}), "full BattleTag")
	many := make([]string, 13)
	for i := range many {
		many[i] = fmt.Sprintf("Friend#%d", 1000+i)
	}
	wantErr(t, f.engine.Configure(dto.FollowConfig{Tags: &many}), "12 friend")
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Unknown#1234"}, IncludeTeam: ptr(false)})
	wantErr(t, f.arm(false), "No activity yet")
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Self#1111"}})
	wantErr(t, f.arm(false), "your own")
	f.presence(map[string]any{"isFriend": false})
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Friend#1234"}})
	wantErr(t, f.arm(false), "friends list")
}

func TestSelectionChangesStopFollowAndReconnectNeverStartsIt(t *testing.T) {
	f := newFixture(t)
	f.mustArm(true)
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Friend#1234"}, IncludeTeam: ptr(false)})
	if f.state().Enabled {
		t.Fatal("selection change kept Follow on")
	}
	f.engine.Connection(false, "")
	s := f.state()
	if len(s.ManualTags) != 1 || s.IncludeTeam || s.Enabled {
		t.Fatalf("state = %+v", s)
	}
	report, _ := json.Marshal(f.engine.Diagnostics())
	if strings.Contains(string(report), "Friend#1234") {
		t.Error("diagnostics contain a BattleTag")
	}
}

func TestOpenLobbyConfirmationIsAnsweredOnceWithNoPassword(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(lobby("Open lobby"))
	f.receive("RequestForPassword", map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Open lobby"}})
	// The game list is loaded first: joining by name only finds lobbies in it.
	if last := f.commands[len(f.commands)-1]; last.message != follow.CmdSendGameListing {
		t.Fatalf("last command = %+v", last)
	}
	// A repeat of the same request while the list loads is not an answer.
	f.receive("RequestForPassword", map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Open lobby"}})
	if f.count(follow.CmdJoinGameByGameName) != 0 || f.state().PasswordRequest != nil {
		t.Fatal("confirmed before the game list loaded")
	}
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	last := f.commands[len(f.commands)-1]
	if last.message != follow.CmdJoinGameByGameName || last.payload["gameName"] != "Open lobby" || last.payload["gamePass"] != "" {
		t.Fatalf("last command = %+v", last)
	}
	if !f.state().Enabled || f.state().PasswordRequest != nil {
		t.Fatal("unexpected password prompt")
	}
	f.receive("GameLobbySetup", map[string]any{"lobbyName": "Open lobby", "players": []any{map[string]any{"isSelf": true, "slotStatus": json.Number("2")}}})
	if s := f.state(); !s.LastJoin.Confirmed || !strings.HasPrefix(s.Status, "Joined") {
		t.Fatalf("state = %+v", s)
	}
}

func TestRepeatedPasswordRequestSkipsTheLobbyUnlessPasswordsAreOn(t *testing.T) {
	f := newFixture(t)
	if f.state().AskPassword {
		t.Fatal("asking for passwords is on by default")
	}
	f.mustArm(false)
	f.presence(nil)
	event := map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Protected"}}
	f.receive("RequestForPassword", event)
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	f.receive("RequestForPassword", event)
	s := f.state()
	if s.PasswordRequest != nil || s.JoinPending || !strings.Contains(s.Status, "needs a password, so it was skipped") {
		t.Fatalf("password request %+v, pending %v, status %q", s.PasswordRequest, s.JoinPending, s.Status)
	}
	// The same lobby is not tried again while it stays open.
	f.advance(time.Minute)
	f.engine.Tick()
	f.presence(nil)
	if f.count(follow.CmdJoinFriendGameLobby) != 1 {
		t.Fatalf("joins = %d", f.count(follow.CmdJoinFriendGameLobby))
	}
}

func TestSwitchingPasswordsOffDropsAnOpenRequest(t *testing.T) {
	f := newFixture(t)
	if err := f.engine.Configure(dto.FollowConfig{AskPassword: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	f.mustArm(false)
	f.presence(nil)
	event := map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Protected"}}
	f.receive("RequestForPassword", event)
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	f.receive("RequestForPassword", event)
	if f.state().PasswordRequest == nil {
		t.Fatal("no password request")
	}
	if err := f.engine.Configure(dto.FollowConfig{AskPassword: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	if f.state().PasswordRequest != nil {
		t.Fatal("the password request stayed open")
	}
}

func TestRepeatedPasswordRequestPromptsAndThePasswordIsNotKept(t *testing.T) {
	f := newFixture(t)
	if err := f.engine.Configure(dto.FollowConfig{AskPassword: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	f.mustArm(false)
	f.presence(nil)
	event := map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Protected"}}
	f.receive("RequestForPassword", event)
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	f.receive("RequestForPassword", event)
	if f.count(follow.CmdJoinGameByGameName) != 1 {
		t.Fatalf("confirmations = %d", f.count(follow.CmdJoinGameByGameName))
	}
	if r := f.state().PasswordRequest; r == nil || r.Lobby != "Protected" {
		t.Fatalf("password request = %+v", r)
	}
	if err := f.engine.SubmitPassword("local-test-secret"); err != nil {
		t.Fatal(err)
	}
	if last := f.commands[len(f.commands)-1]; last.payload["gamePass"] != "local-test-secret" {
		t.Fatalf("last command = %+v", last)
	}
	state, _ := json.Marshal(f.state())
	report, _ := json.Marshal(f.engine.Diagnostics())
	if strings.Contains(string(state)+string(report), "local-test-secret") {
		t.Error("the password leaked into state or diagnostics")
	}
}

func TestJoinConfirmsAnywayWhenTheGameListDoesNotArrive(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(lobby("Open lobby"))
	f.receive("RequestForPassword", map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Open lobby"}})
	// An empty list isn't enough: the list is asked for again each second.
	f.receive("GameList", map[string]any{"games": []any{}})
	for range 3 {
		f.advance(time.Second)
		f.engine.Tick()
	}
	if f.count(follow.CmdJoinGameByGameName) != 0 || f.count(follow.CmdGetGameList) != 3 {
		t.Fatalf("confirmations %d, list requests %d", f.count(follow.CmdJoinGameByGameName), f.count(follow.CmdGetGameList))
	}
	f.advance(time.Second)
	f.engine.Tick()
	if f.count(follow.CmdJoinGameByGameName) != 1 {
		t.Fatal("did not confirm after waiting for the game list")
	}
}

func TestUnsolicitedMismatchedAndLateConfirmationsCannotJoin(t *testing.T) {
	f := newFixture(t)
	f.receive("RequestForPassword", map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Other"}})
	f.mustArm(false)
	f.presence(lobby("Wanted"))
	f.receive("RequestForPassword", map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Other"}})
	if f.count(follow.CmdJoinGameByGameName) != 0 {
		t.Fatal("answered a mismatched confirmation")
	}
	late := newFixture(t)
	late.mustArm(false)
	late.presence(nil)
	late.advance(16 * time.Second)
	late.engine.Tick()
	late.receive("RequestForPassword", map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Late"}})
	if late.count(follow.CmdJoinGameByGameName) != 0 {
		t.Fatal("answered a late confirmation")
	}
}

func TestStoppingPreviewOrQueueingBlocksPasswordFollowUps(t *testing.T) {
	stops := []func(*fixture){
		func(f *fixture) { _ = f.engine.Configure(dto.FollowConfig{Enabled: ptr(false)}) },
		func(f *fixture) { f.receive("QueueEntered", nil) },
		func(f *fixture) { _ = f.engine.Configure(dto.FollowConfig{Diagnostic: ptr(true)}) },
	}
	for i, stop := range stops {
		f := newFixture(t)
		f.mustArm(false)
		f.presence(nil)
		stop(f)
		f.receive("RequestForPassword", map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Open"}})
		if f.count(follow.CmdJoinGameByGameName) != 0 {
			t.Errorf("case %d answered a confirmation", i)
		}
	}
}

func TestPasswordPromptExpiresAndDiesWithTheConnection(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(nil)
	event := map[string]any{"message": map[string]any{"matchJoinGameByNameText": "Open"}}
	f.receive("RequestForPassword", event)
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	f.receive("RequestForPassword", event)
	f.advance(2*time.Minute + time.Millisecond)
	f.engine.Tick()
	if f.state().PasswordRequest != nil {
		t.Fatal("password request did not expire")
	}
	wantErr(t, f.engine.SubmitPassword("test"), "expired")
	f.engine.Connection(false, "")
	wantErr(t, f.engine.SubmitPassword("test"), "expired")
}

func TestFollowResumesAfterALobbyAMatchAndCustomGames(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(nil)
	if len(f.joins()) != 1 {
		t.Fatal("first join missing")
	}
	f.receive("GameLobbySetup", map[string]any{"players": []any{map[string]any{"isSelf": true, "slotStatus": 2}}, "lobbyName": "First"})
	if s := f.state(); !s.Enabled || !s.Paused {
		t.Fatal("not paused in lobby")
	}
	f.presence(lobby("Next lobby"))
	f.advance(11 * time.Second)
	f.engine.Tick()
	if len(f.joins()) != 1 {
		t.Fatal("joined from inside a lobby")
	}
	f.receive("IsGameUIActive", map[string]any{"isActive": "true"})
	f.receive("MultiplayerGameLeave", nil)
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	f.advance(3 * time.Second)
	f.engine.Tick()
	if len(f.joins()) != 1 || !f.state().InMatch {
		t.Fatal("resumed during a match")
	}
	f.receive("ExitedGame", nil)
	f.advance(3 * time.Second)
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	if s := f.state(); s.Busy || !s.Enabled {
		t.Fatalf("did not resume: %+v", s)
	}
	f.presence(lobby("Next lobby"))
	if len(f.joins()) != 2 || !f.state().Enabled {
		t.Fatalf("joins = %d", len(f.joins()))
	}
}

func TestLeavingALobbyDoesNotRejoinItImmediately(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(nil)
	f.receive("GameLobbySetup", nil)
	f.receive("MultiplayerGameLeave", nil)
	f.advance(11 * time.Second)
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	f.presence(nil)
	f.engine.Tick()
	if len(f.joins()) != 1 {
		t.Fatal("rejoined the same lobby")
	}
	f.presence(map[string]any{"localRichPresenceAttributes": attrs("currentGameState", "playing")})
	f.presence(nil)
	if len(f.joins()) != 2 {
		t.Fatal("did not join the reopened lobby")
	}
}

func TestAReturnSignalAloneCannotResume(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.receive("GameFound", nil)
	f.receive("MultiplayerGameLeave", nil)
	f.advance(3 * time.Second)
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	f.presence(nil)
	if len(f.joins()) != 0 {
		t.Fatal("resumed while loading")
	}
	f.receive("ExitedGame", nil)
	f.advance(3 * time.Second)
	f.presence(nil)
	f.engine.Tick()
	if len(f.joins()) != 0 {
		t.Fatal("resumed without menu evidence")
	}
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	f.presence(nil)
	if len(f.joins()) != 1 {
		t.Fatal("did not resume after menu evidence")
	}
}

func TestStopOverridesAutomaticResume(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.receive("GameFound", nil)
	_ = f.engine.Configure(dto.FollowConfig{Enabled: ptr(false)})
	f.receive("ExitedGame", nil)
	f.advance(3 * time.Second)
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	f.presence(nil)
	if len(f.joins()) != 0 || f.state().Enabled {
		t.Fatal("resumed after Stop")
	}
}

func TestFailedAttemptsWaitForANewLobbyAndTheCooldown(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(nil)
	f.receive("GameSearchFailedToJoinGame", nil)
	f.advance(11 * time.Second)
	f.presence(nil)
	if len(f.joins()) != 1 || !f.state().Enabled {
		t.Fatal("retried the failed lobby")
	}
	f.presence(lobby("A new lobby"))
	if len(f.joins()) != 2 {
		t.Fatal("did not try the new lobby")
	}
}

func TestRepeatedLobbyUpdatesConfirmOnce(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(nil)
	update := map[string]any{"lobbyName": "Lobby", "players": []any{map[string]any{"isSelf": true, "slotStatus": 2}}}
	f.receive("GameLobbySetup", update)
	f.receive("GameLobbySetup", update)
	joined := 0
	for _, e := range f.state().Log {
		if strings.HasPrefix(e.Message, "Joined ") {
			joined++
		}
	}
	if joined != 1 || len(f.joins()) != 1 || !f.state().Enabled {
		t.Fatalf("joined notes=%d joins=%d", joined, len(f.joins()))
	}
}

func TestMenuScreenReportsInMenus(t *testing.T) {
	f := newFixture(t)
	if f.state().InMenus {
		t.Fatal("in menus before any screen event")
	}
	f.receive("SetGlueScreen", map[string]any{"screen": "CUSTOM_LOBBIES"})
	if !f.state().InMenus {
		t.Fatal("custom games screen not reported")
	}
	f.receive("SetGlueScreen", map[string]any{"screen": "GAME_LOBBY"})
	if f.state().InMenus {
		t.Fatal("lobby reported as menus")
	}
}

func (f *fixture) friendsSnapshot(friends ...map[string]any) {
	list := make([]any, len(friends))
	for i, fr := range friends {
		list[i] = fr
	}
	f.receive("FriendsFriendData", map[string]any{"data": map[string]any{"friends": list}})
}

func closed(tag string, id int, extra map[string]any) map[string]any {
	fr := friend(map[string]any{"battleTag": tag, "accountId": id, "localRichPresenceAttributes": []any{}, "currentProgram": ""})
	for k, v := range extra {
		fr[k] = v
	}
	return fr
}

func TestRosterListsBattleNetFriendsSortedAndMarksTheGroup(t *testing.T) {
	f := newFixture(t)
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"jaina#2214"}})
	f.friendsSnapshot(
		closed("Thrall#1820", 3, map[string]any{"isOnline": false}),
		closed("Jaina#2214", 2, map[string]any{"isAway": true}),
		friend(map[string]any{"battleTag": "Arthas#1123", "accountId": 1}),
		closed("Stranger#9999", 4, map[string]any{"isFriend": false}),
	)
	r := f.state().Roster
	if len(r) != 3 || r[0].BattleTag != "Arthas#1123" || r[1].BattleTag != "Jaina#2214" || r[2].BattleTag != "Thrall#1820" {
		t.Fatalf("roster = %+v", r)
	}
	if !r[0].Open || !r[0].InGame || r[0].Label != "Lobby open" {
		t.Errorf("Arthas = %+v", r[0])
	}
	if !r[1].Selected || !r[1].Away || r[1].Label != "Away" {
		t.Errorf("Jaina = %+v", r[1])
	}
	if r[2].Online || r[2].Label != "Offline" || r[2].Selected {
		t.Errorf("Thrall = %+v", r[2])
	}
}

func TestRosterSurvivesStartingAndClearsOnDisconnect(t *testing.T) {
	f := newFixture(t)
	f.friendsSnapshot(closed("Jaina#2214", 2, nil))
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Jaina#2214"}, IncludeTeam: ptr(false)})
	f.mustArm(true)
	if len(f.state().Roster) != 1 {
		t.Fatal("starting Follow emptied the roster")
	}
	f.engine.Connection(false, "")
	if len(f.state().Roster) != 0 {
		t.Fatal("roster kept after disconnect")
	}
}

func TestAGroupFollowsWhoeverOpensALobbyFirst(t *testing.T) {
	f := newFixture(t)
	group := []string{"Arthas#1123", "Jaina#2214", "Thrall#1820"}
	f.friendsSnapshot(closed("Arthas#1123", 1, nil), closed("Jaina#2214", 2, nil), closed("Thrall#1820", 3, nil))
	if err := f.engine.Configure(dto.FollowConfig{Tags: &group, IncludeTeam: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	f.mustArm(false)
	f.friendsSnapshot(closed("Arthas#1123", 1, nil), closed("Jaina#2214", 2, nil), closed("Thrall#1820", 3, nil))
	if len(f.state().Watched) != 3 || len(f.joins()) != 0 {
		t.Fatalf("watched=%d joins=%d", len(f.state().Watched), len(f.joins()))
	}
	// Thrall, last in the list, opens a lobby first.
	f.receive("FriendsFriendUpdated", map[string]any{"data": map[string]any{"friend": friend(map[string]any{"battleTag": "Thrall#1820", "accountId": 3})}})
	j := f.joins()
	if len(j) != 1 || j[0].payload["auroraId"] != 3 {
		t.Fatalf("joins = %+v", j)
	}
}

func TestChangingTheGroupWhileFollowingSaysFollowStopped(t *testing.T) {
	f := newFixture(t)
	f.mustArm(true)
	_ = f.engine.Configure(dto.FollowConfig{IncludeTeam: ptr(false)})
	if s := f.state(); s.Enabled || !strings.Contains(s.Status, "Follow stopped") {
		t.Fatalf("state = %+v", s)
	}
}

func notes(f *fixture, fragment string) int {
	n := 0
	for _, e := range f.state().Log {
		if strings.Contains(e.Message, fragment) {
			n++
		}
	}
	return n
}

func TestWatchingAnnouncesGroupLobbiesWithoutStarting(t *testing.T) {
	f := newFixture(t)
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Thrall#1820"}, IncludeTeam: ptr(false)})
	f.friendsSnapshot(
		friend(map[string]any{"battleTag": "Thrall#1820", "accountId": 3, "localRichPresenceAttributes": attrs("currentGameState", "open", "currentGameName", "Legion TD")}),
		friend(map[string]any{"battleTag": "Stranger#4040", "accountId": 9}),
	)
	f.friendsSnapshot(friend(map[string]any{"battleTag": "Thrall#1820", "accountId": 3, "localRichPresenceAttributes": attrs("currentGameState", "open", "currentGameName", "Legion TD")}))
	if n := notes(f, "Thrall#1820 opened a lobby: Legion TD."); n != 1 {
		t.Fatalf("announcements = %d, log = %+v", n, f.state().Log)
	}
	if notes(f, "Stranger#4040") != 0 {
		t.Error("announced a friend outside the group")
	}
	if len(f.joins()) != 0 || f.state().Enabled {
		t.Fatal("watching joined or switched Follow on")
	}
	// A new lobby is announced again.
	f.friendsSnapshot(friend(map[string]any{"battleTag": "Thrall#1820", "accountId": 3, "localRichPresenceAttributes": attrs("currentGameState", "open", "currentGameName", "Footmen")}))
	if notes(f, "opened a lobby: Footmen.") != 1 {
		t.Error("a new lobby was not announced")
	}
}

func TestWatchingIncludesTheTeam(t *testing.T) {
	f := newFixture(t)
	f.presence(nil)
	if notes(f, "Friend#1234 opened a lobby.") != 1 {
		t.Fatalf("log = %+v", f.state().Log)
	}
	_ = f.engine.Configure(dto.FollowConfig{IncludeTeam: ptr(false)})
	f.presence(lobby("Elsewhere"))
	if notes(f, "Elsewhere") != 0 {
		t.Error("announced the team after it was left out")
	}
}

func TestAutoJoinAnnouncesThenJoins(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.presence(lobby("Open"))
	log := f.state().Log
	if len(f.joins()) != 1 || len(log) < 2 ||
		!strings.Contains(log[len(log)-2].Message, "opened a lobby: Open.") || !strings.HasPrefix(log[len(log)-1].Message, "Asked to join") {
		t.Fatalf("log = %+v", log)
	}
}

func TestChangingTheGroupCanReArmRightAway(t *testing.T) {
	f := newFixture(t)
	f.friendsSnapshot(closed("Jaina#2214", 2, nil), closed("Thrall#1820", 3, nil))
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Jaina#2214"}, IncludeTeam: ptr(false)})
	f.mustArm(false)
	// No fresh snapshot yet: the friend list from before arming still counts.
	_ = f.engine.Configure(dto.FollowConfig{Tags: &[]string{"Jaina#2214", "Thrall#1820"}})
	f.mustArm(false)
	if len(f.state().Watched) != 2 {
		t.Fatalf("watched = %+v", f.state().Watched)
	}
	// Joining still waits for fresh activity after arming.
	f.receive("FriendsFriendUpdated", map[string]any{"data": map[string]any{"friend": friend(map[string]any{"battleTag": "Thrall#1820", "accountId": 3})}})
	if j := f.joins(); len(j) != 1 || j[0].payload["auroraId"] != 3 {
		t.Fatalf("joins = %+v", j)
	}
}

func TestJoinsFromAnyMenuScreen(t *testing.T) {
	for _, screen := range []string{"MAIN_MENU", "CAMPAIGNS", "PROFILE", "LEADERBOARD", "REPLAYS_SCREEN", "VERSUS_SETUP", "CUSTOM_LOBBIES"} {
		f := newFixture(t)
		f.receive("SetGlueScreen", map[string]any{"screen": screen})
		if !f.state().InMenus {
			t.Errorf("%s: not reported as a menu", screen)
			continue
		}
		f.mustArm(false)
		f.presence(nil)
		if len(f.joins()) != 1 {
			t.Errorf("%s: joins = %d", screen, len(f.joins()))
		}
	}
}

func TestCreatingAGamePausesJoining(t *testing.T) {
	for _, screen := range []string{"CREATE_GAME_LOBBY", "SINGLE_PLAYER_CUSTOM_LOBBY", "LOGIN_DOORS"} {
		f := newFixture(t)
		f.mustArm(false)
		f.receive("SetGlueScreen", map[string]any{"screen": screen})
		f.presence(nil)
		if s := f.state(); len(f.joins()) != 0 || s.InMenus || !s.Paused {
			t.Errorf("%s: joins=%d inMenus=%v paused=%v", screen, len(f.joins()), s.InMenus, s.Paused)
		}
	}
}

func TestRankedScoreScreenThenAnyMenuResumes(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.receive("GameFound", nil)
	f.receive("SetGlueScreen", map[string]any{"screen": "RANKED_SCORE_SCREEN"})
	f.receive("SetGlueScreen", map[string]any{"screen": "PROFILE"})
	f.advance(3 * time.Second)
	f.engine.Tick()
	if s := f.state(); s.Busy || s.InMatch || !s.Enabled {
		t.Fatalf("did not resume: %+v", s)
	}
	f.presence(nil)
	if len(f.joins()) != 1 {
		t.Fatalf("joins = %d", len(f.joins()))
	}
}

// soloMatch replays the events a real single-player custom game sent on
// 3.0.1.24371, from creating it to quitting at the score screen.
func soloMatch(f *fixture) {
	f.receive("MultiplayerGameCreateStarted", nil)
	f.receive("SetGlueScreen", map[string]any{"screen": "GAME_LOBBY"})
	f.receive("GameLobbySetup", map[string]any{"players": []any{map[string]any{"isSelf": true, "slotStatus": 2}}})
	f.receive("LobbyCountdown", nil)
	f.receive("GameLobbyGracefulExit", nil)
	f.receive("SetGlueScreen", map[string]any{"screen": "LOADING_SCREEN"})
	f.receive("SetGlueScreen", map[string]any{"screen": "DISABLED_SCREEN"})
	f.receive("IsGameUIActive", map[string]any{"isActive": true})
	f.receive("MultiplayerGameCreateResult", nil)
	f.receive("ExitedGame", nil)
	f.receive("IsGameUIActive", map[string]any{"isActive": false})
	f.receive("SetGlueScreen", map[string]any{"screen": "SCORE_SCREEN"})
	f.receive("MultiplayerGameLeave", nil)
}

func TestFollowResumesFromTheScoreScreenOfARealMatch(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	soloMatch(f)
	f.presence(nil)
	if s := f.state(); len(f.joins()) != 0 || !s.Paused || notes(f, "The match is over") != 1 {
		t.Fatalf("joined too early or missing note: joins=%d log=%+v", len(f.joins()), s.Log)
	}
	f.advance(3 * time.Second)
	f.engine.Tick()
	if s := f.state(); s.Paused || !s.Enabled || !s.InMenus {
		t.Fatalf("did not resume after the score screen: %+v", s)
	}
	f.presence(nil)
	if len(f.joins()) != 1 {
		t.Fatalf("joins = %d", len(f.joins()))
	}
}

func TestLeavingALobbyStillWaitsForMenuEvidence(t *testing.T) {
	f := newFixture(t)
	f.mustArm(false)
	f.receive("SetGlueScreen", map[string]any{"screen": "GAME_LOBBY"})
	f.receive("GameLobbySetup", nil)
	f.receive("MultiplayerGameLeave", nil)
	f.advance(3 * time.Second)
	f.engine.Tick()
	f.presence(nil)
	if len(f.joins()) != 0 || !f.state().Paused {
		t.Fatal("resumed after leaving a lobby without menu evidence")
	}
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	f.presence(nil)
	if len(f.joins()) != 1 {
		t.Fatal("did not resume once the custom game list showed")
	}
}

func TestInMenusIsKnownFromTheGameListOrAMenuScreen(t *testing.T) {
	f := newFixture(t)
	if f.state().InMenus {
		t.Fatal("in menus before any evidence")
	}
	f.receive("GameList", map[string]any{"games": []any{map[string]any{}}})
	if !f.state().InMenus {
		t.Fatal("the custom game list did not count as menus")
	}
	f.receive("QueueEntered", nil)
	if f.state().InMenus {
		t.Fatal("in menus while queued")
	}
}
