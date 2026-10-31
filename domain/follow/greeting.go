package follow

import (
	"math/rand/v2"
	"strings"
	"time"
	"unicode"
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
	minGreetDelay = 3 * time.Second
	maxGreetDelay = 10 * time.Second
	// minReplyDelay and maxReplyDelay replace that delay once someone else
	// greets in the lobby first.
	minReplyDelay = 2 * time.Second
	maxReplyDelay = 4 * time.Second
)

// greetingWords are the first words that make another player's lobby line a
// greeting.
var greetingWords = map[string]bool{
	"hi": true, "hey": true, "hello": true, "hiya": true, "heya": true, "hey all": true, "yo": true,
	"sup": true, "hola": true, "howdy": true, "glhf": true, "gl": true, "hai": true, "helo": true, "hallo": true,
}

// Greetings returns the only lines the engine types to greet a lobby
// (docs/adr/0010).
func Greetings() []string {
	return []string{"hi", "hey", "hello", "hi all", "hey everyone", "hello there", "hiya", "glhf"}
}

// greeting is the "say hi" for the lobby the player is in, as host or guest.
type greeting struct {
	// inLobby is set once a lobby was seen; lobby is its name.
	inLobby bool
	lobby   string
	// scheduled is set once a greeting was planned for this lobby, so it is
	// greeted at most once.
	scheduled bool
	// pending is set while a greeting waits for at.
	pending bool
	at      time.Time
	// replying is set once at was moved because someone else greeted.
	replying bool
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

// greetLobby schedules the greeting on a GameLobbySetup: 3 to 10 seconds
// after the first human the player doesn't follow is seated in the lobby the
// player sits in, whether the player hosts it (by hand or with Auto-host) or
// joined it, outside any match. Each lobby is greeted at most once.
func (e *Engine) greetLobby(p map[string]any, inSlot bool, name string) {
	if !inSlot || e.inMatch {
		return
	}
	g := &e.greet
	if !g.inLobby || g.lobby != name {
		*g = greeting{inLobby: true, lobby: name}
	}
	if g.scheduled || !e.strangerSeated(p) {
		return
	}
	g.scheduled, g.pending = true, true
	g.at = e.now().Add(e.randomDelay(minGreetDelay, maxGreetDelay))
}

// strangerSeated reports whether the lobby has a human player, other than
// the player, whom the player doesn't follow. Computers, open slots and
// players without a name yet don't count.
func (e *Engine) strangerSeated(p map[string]any) bool {
	followed := map[string]bool{}
	for _, tag := range e.group() {
		followed[tagKey(tag)] = true
	}
	players, _ := list(p["players"])
	for _, raw := range players {
		x := object(raw)
		name, _ := x["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" || isTrue(x["isSelf"]) || e.isSelf(name) || !numberIs(x["slotStatus"], slotOccupied) ||
			!numberIs(x["slotType"], slotHuman) || followed[tagKey(name)] {
			continue
		}
		return true
	}
	return false
}

func (e *Engine) randomDelay(lo, hi time.Duration) time.Duration {
	return lo + time.Duration(e.random.Float64()*float64(hi-lo))
}

// greetReply answers another player's greeting in the lobby chat: a waiting
// greeting is then typed 2 to 4 seconds after it, instead of after its own
// 3 to 10 second delay. Only the first greeting heard counts.
func (e *Engine) greetReply(p map[string]any) {
	g := &e.greet
	if !g.pending || g.replying {
		return
	}
	m := object(p["message"])
	source, _ := m["source"].(string)
	sender, _ := m["sender"].(string)
	content, _ := m["content"].(string)
	if source != "gameChat" || strings.TrimSpace(sender) == "" || e.sentBySelf(sender) || !IsGreeting(content) {
		return
	}
	g.replying = true
	g.at = e.now().Add(e.randomDelay(minReplyDelay, maxReplyDelay))
}

// IsGreeting reports whether a chat line opens with a greeting word, such
// as "hi", "Hey all!" or "GLHF".
func IsGreeting(line string) bool {
	words := strings.FieldsFunc(strings.ToLower(line), func(r rune) bool { return !unicode.IsLetter(r) })
	if len(words) == 0 {
		return false
	}
	return greetingWords[words[0]] || len(words) > 1 && greetingWords[words[0]+" "+words[1]]
}

// greetStep types the greeting once its time has come. A failed send is not
// retried.
func (e *Engine) greetStep() {
	g := &e.greet
	if !g.pending || e.now().Before(g.at) {
		return
	}
	g.pending = false
	if !e.connected || e.inMatch {
		return
	}
	line := Greetings()[e.random.IntN(len(Greetings()))]
	if e.sendChat(line) == nil {
		e.Note("Said “" + line + "” in the lobby.")
	}
}
