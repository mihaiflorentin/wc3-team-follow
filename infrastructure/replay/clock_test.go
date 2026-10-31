package replay

import (
	"bytes"
	"testing"
	"time"
)

func clockAction(value, counter uint32) []byte {
	return (&buf{}).u8(0x62).u32(0xABCD).u32(value).u32(counter).b
}

func TestParseFindsWhenTheGameClockStopped(t *testing.T) {
	g := defaultGame()
	// The clock rises once a second, then stops at 3 and is sent again.
	for i := uint32(1); i <= 3; i++ {
		g.gameData = append(g.gameData, timeSlot(1000, command(1, clockAction(i, 1)), command(2, clockAction(i, 1)))...)
	}
	for n := uint32(2); n <= 4; n++ {
		g.gameData = append(g.gameData, timeSlot(1000, command(1, clockAction(3, n)), command(2, clockAction(3, n)))...)
	}
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	if r.Ended != 3*time.Second || r.GameTime != 6*time.Second || len(r.Warnings) != 0 {
		t.Errorf("ended %v of %v, warnings %v", r.Ended, r.GameTime, r.Warnings)
	}
	// Still counted as actions, not for APM.
	if p := r.Players[0]; p.Actions != 6 || p.APMActions != 0 {
		t.Errorf("p1 = %+v", p)
	}
}

func TestParseWithoutAStoppedClockHasNoEnd(t *testing.T) {
	g := defaultGame()
	// A repeated value, then the clock goes on: not the end.
	for _, tick := range [][2]uint32{{1, 1}, {1, 2}, {2, 1}, {3, 1}} {
		g.gameData = append(g.gameData, timeSlot(1000, command(1, clockAction(tick[0], tick[1])))...)
	}
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	if r.Ended != 0 {
		t.Errorf("ended = %v", r.Ended)
	}
}
