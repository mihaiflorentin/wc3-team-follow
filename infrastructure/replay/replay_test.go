package replay

import (
	"bytes"
	"testing"
	"time"
)

func parseGame(t *testing.T, g fxGame) []byte {
	t.Helper()
	return encodeReplay(t, 10200, 7003, 482_000, g.bytes())
}

func TestParseReadsTheHeader(t *testing.T) {
	r, err := Parse(bytes.NewReader(parseGame(t, defaultGame())))
	if err != nil {
		t.Fatal(err)
	}
	if r.VersionNumber != 10200 || r.Version != "3.00" || r.Build != 7003 {
		t.Errorf("version = %d %q build %d", r.VersionNumber, r.Version, r.Build)
	}
	if r.Duration != 482*time.Second || !r.Expansion || !r.Multiplayer {
		t.Errorf("duration %v expansion %v multiplayer %v", r.Duration, r.Expansion, r.Multiplayer)
	}
}

func TestParseRejectsAFileThatIsNotAReplay(t *testing.T) {
	if _, err := Parse(bytes.NewReader([]byte("hello, not a replay"))); err == nil {
		t.Fatal("parsed garbage")
	}
}

func TestParseRejectsACorruptHeader(t *testing.T) {
	b := parseGame(t, defaultGame())
	b[60] ^= 0xFF // duration, covered by the header checksum
	if _, err := Parse(bytes.NewReader(b)); err == nil {
		t.Fatal("accepted a header with a bad checksum")
	}
}

func TestParseSurvivesDataCutAnywhere(t *testing.T) {
	g := defaultGame()
	g.reforged = reforgedRecord(0x38, 0x03, pbString(1, string(append(pbVarint(1, 1), pbString(2, "Player1#1234")...))))
	g.gameData = append(timeSlot(100, command(1, []byte{0x61}, (&buf{}).u8(0x16).u8(1).u16(1).raw(make([]byte, 8)).b)), chat(1, 0, "hi")...)
	g.gameData = append(g.gameData, leave(0x0C, 1, 0x09, 1)...)
	data := g.bytes()
	for n := range len(data) {
		// Must return a result or an error, never panic.
		_, _ = Parse(bytes.NewReader(encodeReplay(t, 10200, 7003, 1000, data[:n])))
	}
}

func TestParseDecompressesEveryBlock(t *testing.T) {
	g := defaultGame()
	// Chat lines long enough to span three blocks; the last one must still
	// be read.
	long := string(bytes.Repeat([]byte("x"), 200))
	for range 100 {
		g.gameData = append(g.gameData, chat(1, 0, long)...)
	}
	g.gameData = append(g.gameData, chat(2, 0, "last")...)
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Chat) != 101 || r.Chat[100].Message != "last" {
		t.Fatalf("chat lines = %d", len(r.Chat))
	}
}

func TestParseReadsGameInfoAndSlots(t *testing.T) {
	g := defaultGame()
	g.slots = append(g.slots,
		fxSlot{playerID: 0, status: 0}, // open
		fxSlot{playerID: 3, status: 2, computer: 1, team: 1, race: 0x08, ai: 2, handicap: 100},
		fxSlot{playerID: 4, status: 2, team: 24, color: 24, race: 0x60, handicap: 100},
	)
	g.players = append(g.players, fxPlayer{id: 4, name: "Watcher"})
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	if r.GameName != "Sample game" || r.HostID != 1 {
		t.Errorf("game %q host %d", r.GameName, r.HostID)
	}
	if r.Map.Path != "Maps/Download/Sample (4).w3x" || r.Map.File != "Sample (4).w3x" || r.Map.Creator != "Map Maker" || r.Map.Checksum != "efbeadde" {
		t.Errorf("map = %+v", r.Map)
	}
	if r.Settings.Speed != 2 || r.Settings.Observers != "full" || !r.Settings.FixedTeams {
		t.Errorf("settings = %+v", r.Settings)
	}
	if len(r.Players) != 4 {
		t.Fatalf("players = %+v", r.Players)
	}
	p1, p2, cpu, obs := r.Players[0], r.Players[1], r.Players[2], r.Players[3]
	if p1.Name != "Player1" || p1.Race != "human" || p1.Team != 0 || p1.Handicap != 100 {
		t.Errorf("p1 = %+v", p1)
	}
	if p2.Name != "Player2" || p2.Race != "orc" || p2.Team != 1 || p2.Color != 1 || p2.Handicap != 90 {
		t.Errorf("p2 = %+v", p2)
	}
	if !cpu.Computer || cpu.Name != "Computer" || cpu.Race != "undead" || cpu.AIStrength != 2 || cpu.Slot != 3 {
		t.Errorf("computer = %+v", cpu)
	}
	if !obs.Observer || obs.Name != "Watcher" || obs.Race != "random" {
		t.Errorf("observer = %+v", obs)
	}
}

func TestParseReadsReforgedPlayerMetadata(t *testing.T) {
	player := func(id uint64, tag, clan string) []byte {
		b := append(pbVarint(1, id), pbString(2, tag)...)
		if clan != "" {
			b = append(b, pbString(3, clan)...)
		}
		return append(b, pbString(4, "p042")...)
	}
	skins := append(pbVarint(1, 2), pbString(2, "skin-a")...)
	skins = append(skins, pbString(2, "skin-b")...)
	for _, tc := range []struct {
		name    string
		id      byte
		post202 bool
	}{{"before 2.0.2 (0x39)", 0x39, false}, {"from 2.0.2 (0x38)", 0x38, true}} {
		t.Run(tc.name, func(t *testing.T) {
			g := defaultGame()
			g.reforged = append(reforgedRecord(tc.id, 0x03, player(1, "Player1#1234", "CLAN")),
				reforgedRecord(tc.id, 0x03, player(2, "Player2#5678", ""))...)
			g.reforged = append(g.reforged, reforgedRecord(tc.id, 0x04, skins)...)
			r, err := Parse(bytes.NewReader(parseGame(t, g)))
			if err != nil {
				t.Fatal(err)
			}
			if r.Post202 != tc.post202 {
				t.Errorf("post202 = %v", r.Post202)
			}
			if p := r.Players[0]; p.BattleTag != "Player1#1234" || p.Clan != "CLAN" {
				t.Errorf("p1 = %+v", p)
			}
			if p := r.Players[1]; p.BattleTag != "Player2#5678" || p.Clan != "" || p.Skins != 2 {
				t.Errorf("p2 = %+v", p)
			}
		})
	}
}

func TestParseReadsReforgedPlayersNestedInOneRecord(t *testing.T) {
	// Current replays (2.0 and 3.0) hold every player in one subtype 0x03
	// record, each as a nested message in repeated field 1.
	nested := func(id uint64, tag, clan, portrait string) []byte {
		p := append(pbVarint(1, id), pbString(2, tag)...)
		p = append(p, pbString(3, clan)...)
		p = append(p, pbString(4, portrait)...)
		p = append(p, pbVarint(5, 20)...)
		return pbString(1, string(p))
	}
	body := append(nested(1, "Player1#1234", "CLAN", "p001"), nested(2, "Player2#5678", "", "")...)
	g := defaultGame()
	g.reforged = append(reforgedRecord(0x38, 0x04, nil), reforgedRecord(0x38, 0x03, body)...)
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	if p := r.Players[0]; p.BattleTag != "Player1#1234" || p.Clan != "CLAN" || p.Portrait != "p001" {
		t.Errorf("p1 = %+v", p)
	}
	if p := r.Players[1]; p.BattleTag != "Player2#5678" || p.Clan != "" {
		t.Errorf("p2 = %+v", p)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("warnings = %v", r.Warnings)
	}
}

func TestParseReadsLeavesAndChat(t *testing.T) {
	g := defaultGame()
	g.gameData = append(g.gameData, chat(1, 0, "glhf")...)
	g.gameData = append(g.gameData, timeSlot(1500)...)
	g.gameData = append(g.gameData, chat(2, 1, "push")...)
	g.gameData = append(g.gameData, timeSlot(500)...)
	g.gameData = append(g.gameData, leave(0x01, 2, 0x08, 3)...)
	g.gameData = append(g.gameData, timeSlot(1000)...)
	g.gameData = append(g.gameData, leave(0x0C, 1, 0x09, 4)...)
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	if r.GameTime != 3*time.Second {
		t.Errorf("game time = %v", r.GameTime)
	}
	if len(r.Chat) != 2 || r.Chat[0].Mode != "all" || r.Chat[0].Time != 0 || r.Chat[1].Mode != "allies" || r.Chat[1].Time != 1500*time.Millisecond || r.Chat[1].PlayerID != 2 {
		t.Errorf("chat = %+v", r.Chat)
	}
	if len(r.Leaves) != 2 {
		t.Fatalf("leaves = %+v", r.Leaves)
	}
	if l := r.Leaves[0]; l.PlayerID != 2 || l.Reason != "remote" || l.Result != "lost" || l.Time != 2*time.Second {
		t.Errorf("leave 0 = %+v", l)
	}
	if r.SaverID != 1 || r.Players[0].Result != "won" || r.Players[1].Left != 2*time.Second || r.Players[1].Result != "lost" {
		t.Errorf("saver %d players %+v", r.SaverID, r.Players)
	}
}

func TestParseCountsActionsAndAPM(t *testing.T) {
	order := (&buf{}).u8(0x10).u16(0).raw([]byte("tlah")).raw(make([]byte, 8)).b
	pointOrder := (&buf{}).u8(0x11).u16(0).raw(make([]byte, 4+8+8)).b
	selectUnits := func(mode byte) []byte { return (&buf{}).u8(0x16).u8(mode).u16(1).raw(make([]byte, 8)).b }
	hotkey := []byte{0x18, 1, 0}
	sync := (&buf{}).u8(0x6B).cstr("f").cstr("m").cstr("k").u32(7).b // not counted for APM
	g := defaultGame()
	// Minute one: 4 APM actions for player 1 (a deselect then a select counts once).
	g.gameData = append(g.gameData, timeSlot(30000, command(1, order, selectUnits(2), selectUnits(1), hotkey, sync), command(2, pointOrder))...)
	g.gameData = append(g.gameData, timeSlot(30000, command(1, pointOrder))...)
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	p1, p2 := r.Players[0], r.Players[1]
	if p1.Actions != 6 || p1.APMActions != 4 || p1.LastAction != time.Minute || p1.APM != 4 {
		t.Errorf("p1 = %+v", p1)
	}
	if p2.Actions != 1 || p2.APMActions != 1 || p2.APM != 2 {
		t.Errorf("p2 = %+v", p2)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("warnings = %v", r.Warnings)
	}
}

func TestParseShiftsActionIdsFrom202(t *testing.T) {
	// From 2.0.2 the id written as 0x78 means 0x79 (8+4+4 bytes and a
	// string); read as the old 0x78 (two strings and 4 bytes) it would
	// misalign the block.
	act := (&buf{}).u8(0x78).raw(make([]byte, 16)).cstr("x").b
	g := defaultGame()
	g.reforged = reforgedRecord(0x38, 0x03, append(pbVarint(1, 1), pbString(2, "Player1#1234")...))
	g.gameData = timeSlot(100, command(1, act, []byte{0x61}))
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	if p := r.Players[0]; p.Actions != 2 || p.APMActions != 1 || len(r.Warnings) != 0 {
		t.Errorf("p1 = %+v warnings %v", p, r.Warnings)
	}
}

func TestParseReadsRaw0x77From202AsTheOldSyncAction(t *testing.T) {
	// Real 2.0.2+ replays write map syncs ("SyncCode", "GamesPlayed") as raw
	// 0x77 with the old 0x78 layout: two strings and 4 bytes. w3gjs shifts
	// only ids above 0x77, which misreads them.
	sync := (&buf{}).u8(0x77).cstr("GamesPlayed").cstr("41").u32(0).b
	g := defaultGame()
	g.reforged = reforgedRecord(0x38, 0x03, append(pbVarint(1, 1), pbString(2, "Player1#1234")...))
	g.gameData = timeSlot(100, command(1, sync, []byte{0x61}))
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	if p := r.Players[0]; p.Actions != 2 || p.APMActions != 1 || len(r.Warnings) != 0 {
		t.Errorf("p1 = %+v warnings %v", p, r.Warnings)
	}
}

func TestParseDecidesTheWinnerFromLeaveRecords(t *testing.T) {
	// Players 1 and 3 are team 0, 2 and 4 team 1, 5 observes.
	game := func(leaves ...[]byte) fxGame {
		g := defaultGame()
		g.players = append(g.players, fxPlayer{id: 3, name: "Player3"}, fxPlayer{id: 4, name: "Player4"}, fxPlayer{id: 5, name: "Watcher"})
		g.slots = append(g.slots,
			fxSlot{playerID: 3, status: 2, team: 0, race: 1, handicap: 100},
			fxSlot{playerID: 4, status: 2, team: 1, race: 1, handicap: 100},
			fxSlot{playerID: 5, status: 2, team: 24, race: 1, handicap: 100},
		)
		for _, l := range leaves {
			g.gameData = append(g.gameData, timeSlot(1000)...)
			g.gameData = append(g.gameData, l...)
		}
		return g
	}
	for _, tc := range []struct {
		name       string
		leaves     [][]byte
		team       int
		draw       bool
		confidence string
	}{
		{"a player leaves with a won result", [][]byte{leave(0x01, 2, 0x09, 1), leave(0x0C, 1, 0x08, 2)}, 1, false, "certain"},
		{"the saver's game records a win", [][]byte{leave(0x0C, 2, 0x09, 1), leave(0x0C, 1, 0x09, 2)}, 0, false, "certain"},
		{"the saver's game records a loss", [][]byte{leave(0x0C, 2, 0x08, 1), leave(0x0C, 1, 0x08, 2)}, 1, false, "certain"},
		{"every player of a team is erased", [][]byte{leave(0x01, 2, 0x08, 1), leave(0x01, 4, 0x08, 1), leave(0x0C, 1, 0x01, 1)}, 0, false, "certain"},
		{"a draw", [][]byte{leave(0x01, 2, 0x0A, 1), leave(0x0C, 1, 0x0A, 1)}, -1, true, "certain"},
		{"the saver's last record is counted up", [][]byte{leave(0x01, 2, 0x07, 3), leave(0x0C, 1, 0x07, 4)}, 0, false, "likely"},
		{"the saver's last record is not counted up", [][]byte{leave(0x01, 2, 0x07, 3), leave(0x0C, 1, 0x07, 3)}, 1, false, "likely"},
		{"the first team to leave lost", [][]byte{leave(0x01, 5, 0x0B, 1), leave(0x01, 3, 0x01, 1), leave(0x0C, 2, 0x01, 1)}, 1, false, "guess"},
		{"no leave records", nil, -1, false, "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := Parse(bytes.NewReader(parseGame(t, game(tc.leaves...))))
			if err != nil {
				t.Fatal(err)
			}
			if w := r.Winner; w.Team != tc.team || w.Draw != tc.draw || w.Confidence != tc.confidence {
				t.Errorf("winner = %+v", w)
			}
		})
	}
}

func TestParseWarnsAboutAnUnknownActionAndKeepsGoing(t *testing.T) {
	g := defaultGame()
	g.gameData = append(timeSlot(100, command(1, []byte{0x61, 0xEE, 1, 2, 3})), leave(0x01, 2, 0x08, 1)...)
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	if r.Players[0].Actions != 1 || len(r.Leaves) != 1 || len(r.Warnings) != 1 {
		t.Errorf("actions %d leaves %d warnings %v", r.Players[0].Actions, len(r.Leaves), r.Warnings)
	}
}
