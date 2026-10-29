package follow

import (
	"math/rand/v2"
	"time"
)

// Random is the randomness the greeting needs; *rand.Rand meets it, and
// tests pass a fixed one.
type Random interface {
	// IntN returns a number in [0, n).
	IntN(n int) int
	// Float64 returns a number in [0, 1).
	Float64() float64
}

const (
	minGreetDelay = time.Second
	maxGreetDelay = 5 * time.Second
)

// Greetings returns the only lines the engine types when the player joins
// a lobby (docs/adr/0010).
func Greetings() []string {
	return []string{"hi", "hey", "hello", "hi all", "hey everyone", "hello there", "hiya", "glhf"}
}

// greeting is the "say hi" for the lobby the player joined as a guest.
type greeting struct {
	// inLobby is set once a lobby was seen (so it is greeted at most once);
	// lobby is its name.
	inLobby bool
	lobby   string
	// pending is set while a greeting waits for at.
	pending bool
	at      time.Time
}

// SetRandom sets the randomness used to pick the greeting and its delay.
// NewEngine starts with a randomly seeded generator.
func (e *Engine) SetRandom(r Random) { e.random = r }

func newRandom() Random { return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())) }

// endGreeting forgets the lobby and cancels a waiting greeting: the player
// left it, or a match began.
func (e *Engine) endGreeting() { e.greet = greeting{} }

// cancelGreeting cancels a waiting greeting but keeps the lobby, so it is
// not greeted again.
func (e *Engine) cancelGreeting() { e.greet.pending = false }

// greetLobby schedules the greeting on a GameLobbySetup: the first one of a
// lobby the player sits in as a guest, outside Auto-host and any match.
// isHost must be present and false.
func (e *Engine) greetLobby(p map[string]any, inSlot bool, name string) {
	isHost, known := p["isHost"].(bool)
	if !inSlot || !known || isHost || e.host.enabled || e.inMatch {
		return
	}
	g := &e.greet
	if g.inLobby && g.lobby == name {
		return
	}
	span := maxGreetDelay - minGreetDelay
	*g = greeting{inLobby: true, lobby: name, pending: true,
		at: e.now().Add(minGreetDelay + time.Duration(e.random.Float64()*float64(span)))}
}

// greetStep types the greeting once its time has come. A failed send is not
// retried.
func (e *Engine) greetStep() {
	g := &e.greet
	if !g.pending || e.now().Before(g.at) {
		return
	}
	g.pending = false
	if !e.connected || e.inMatch || e.host.enabled {
		return
	}
	line := Greetings()[e.random.IntN(len(Greetings()))]
	if e.sendChat(line) == nil {
		e.Note("Said “" + line + "” in the lobby.")
	}
}
