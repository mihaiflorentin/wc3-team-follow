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

func TestParseFindsTheStopPastAStrayTick(t *testing.T) {
	// A real 3.0 replay: the clock stopped at 20:54.37, one tick with a
	// higher value came at 20:55.02, then the stopped value repeated.
	g := defaultGame()
	for _, tick := range [][2]uint32{{1, 1}, {2, 1}, {3, 1}, {3, 2}, {4, 1}, {3, 3}, {3, 4}} {
		g.gameData = append(g.gameData, timeSlot(1000, command(1, clockAction(tick[0], tick[1])))...)
	}
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	if r.Ended != 3*time.Second {
		t.Errorf("ended = %v", r.Ended)
	}
}

func TestParseFindsWhenRoundsBegan(t *testing.T) {
	// Each round opens a timer phase: ticks sent again (counter above 1)
	// after a quiet stretch. The phase at the end of the match is no round.
	g := defaultGame()
	tick := func(value, counter uint32) []byte {
		return timeSlot(1000, command(1, clockAction(value, counter)))
	}
	var data []byte
	value := uint32(1)
	phase := func() { // rounds 1–3: a repeated value for 3 s
		for n := uint32(2); n <= 4; n++ {
			data = append(data, tick(value, n)...)
		}
		value++
	}
	quiet := func(seconds int) {
		for range seconds {
			data = append(data, tick(value, 1)...)
			value++
		}
	}
	phase() // round 1 at 1 s
	quiet(40)
	phase() // round 2 at 44 s
	quiet(40)
	phase() // round 3 at 87 s
	quiet(40)
	// The end: the clock stops at 130 s and repeats.
	data = append(data, tick(value, 1)...)
	for n := uint32(2); n <= 4; n++ {
		data = append(data, tick(value, n)...)
	}
	g.gameData = data
	r, err := Parse(bytes.NewReader(parseGame(t, g)))
	if err != nil {
		t.Fatal(err)
	}
	want := []time.Duration{time.Second, 44 * time.Second, 87 * time.Second}
	if len(r.Rounds) != len(want) || r.Ended != 130*time.Second {
		t.Fatalf("rounds %v, ended %v", r.Rounds, r.Ended)
	}
	for i := range want {
		if r.Rounds[i] != want[i] {
			t.Errorf("round %d at %v, want %v", i+1, r.Rounds[i], want[i])
		}
	}
}
