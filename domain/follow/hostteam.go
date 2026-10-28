package follow

import (
	"fmt"
	"strings"
)

// joinedLobby is a lobby Auto-join joined, while the player is in it.
type joinedLobby struct {
	// friend is the group member whose lobby was joined.
	friend string
	// moves counts SetTeam requests per target team, so a refused move
	// isn't repeated forever.
	moves map[string]int
	// noted is set once the player was told the lobby locks teams.
	noted bool
	// countdown mirrors LobbyCountdown's details.countdownActive: no move
	// is asked for while the lobby counts down to the match.
	countdown bool
}

// moveToHostTeam moves the player's own slot to the host's team in a lobby
// Auto-join joined, when the group member it followed hosts it and their
// team has an open slot. Every player may move their own slot, as the
// lobby's team header does (GlueManager.js); the game's !swap can't use
// an open slot, so this is how a member reaches the host's team there.
// Only the player's own slot moves, at most maxMoves times per team, and
// never while the lobby counts down.
func (e *Engine) moveToHostTeam(p map[string]any) {
	j := e.joined
	if j == nil || !e.joinHostTeam || e.host.enabled || j.countdown || isTrue(p["isHost"]) {
		return
	}
	hostName, _ := p["playerHost"].(string)
	if !sameTag(hostName, j.friend) {
		return
	}
	players, openTeams, self := readLobby(p)
	host := findPlayer(players, j.friend)
	if self == nil || !self.occupied || host == nil || sameNumber(self.team, host.team) || !hasTeam(openTeams, host.team) {
		return
	}
	if !self.canMove {
		if !j.noted {
			j.noted = true
			e.Note(j.friend + "'s lobby doesn't let players change teams, so you stay where the game put you.")
		}
		return
	}
	key := fmt.Sprint(host.team)
	if j.moves[key] >= maxMoves {
		return
	}
	j.moves[key]++
	if e.send(CmdSetTeam, map[string]any{"slot": self.slot, "team": host.team}) {
		e.Note("Moving you to " + j.friend + "'s team.")
	}
}

// sameTag reports whether name, as a lobby shows it, is the player with
// BattleTag tag: the full tag, or only its name part.
func sameTag(name, tag string) bool {
	n, key := tagKey(name), tagKey(tag)
	short, _, _ := strings.Cut(key, "#")
	return n != "" && (n == key || n == short)
}
