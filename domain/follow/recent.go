package follow

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// slotHuman is the slotType of a human player.
const slotHuman = 0

// colorCode matches the game's text colour codes (|cAARRGGBB and |r), which
// map names may carry.
var colorCode = regexp.MustCompile(`(?i)\|c[0-9a-f]{8}|\|r`)

// Recent remembers the humans seen in a lobby or match with the player. It
// only reads events and never sends anything to the game.
//
// Each GameLobbySetup replaces who is present and stamps them with the
// clock. Lobby updates stop once a match loads, so the last lobby's
// players stay present, and Tick keeps refreshing their last-seen time,
// until the score screen, a menu screen, leaving or a disconnect.
//
// A player's lobby count grows once per lobby: when a lobby starts with
// them in it, or when they join it, never on later updates of the same
// lobby. A lobby ends with End, or when an update names a different lobby.
type Recent struct {
	now     func() time.Time
	players []dto.RecentPlayer
	present []string
	mapName string
	// lobbyName names the current lobby; counted holds the tag keys whose
	// lobby count already includes it.
	lobbyName string
	counted   map[string]bool
	// changed is set when a player or map was added or changed, touched
	// when only last-seen times moved, since the list was last saved.
	changed, touched bool
}

// NewRecent returns a recorder holding the saved list.
func NewRecent(now func() time.Time, saved []dto.RecentPlayer) *Recent {
	r := &Recent{now: now, players: slices.Clone(saved)}
	for i, p := range r.players {
		// Lists saved before first-seen and lobby counts existed.
		if p.FirstSeen.IsZero() || p.FirstSeen.After(p.LastSeen) {
			r.players[i].FirstSeen = p.LastSeen
		}
		if p.Lobbies < 1 {
			r.players[i].Lobbies = 1
		}
	}
	r.order()
	return r
}

// Receive applies one game event.
func (r *Recent) Receive(event dto.GameEvent) {
	p := event.Payload
	switch event.MessageType {
	case "GameLobbySetup":
		name, _ := p["lobbyName"].(string)
		name = strings.TrimSpace(name)
		if name != r.lobbyName && name != "" && r.lobbyName != "" {
			r.counted = nil
		}
		r.lobbyName = name
		r.present, r.mapName = lobbyOthers(p), lobbyMap(p)
		r.touch()
	case "MultiplayerGameLeave", "ExitedGame", "LoggedOut":
		r.End()
	case "SetGlueScreen":
		screen, _ := p["screen"].(string)
		if slices.Contains(scoreScreens, screen) || slices.Contains(menuScreens, screen) {
			r.End()
		}
	}
}

// Tick refreshes the last-seen time of everyone present.
func (r *Recent) Tick() { r.touch() }

// End stamps everyone present one last time and forgets who is present:
// the lobby or match is over, or the game is gone.
func (r *Recent) End() {
	r.touch()
	if len(r.present) > 0 {
		r.changed = true
	}
	r.present, r.mapName, r.lobbyName, r.counted = nil, "", "", nil
}

// Clear forgets every player, including who is present now; the next
// lobby update records them again.
func (r *Recent) Clear() {
	r.players, r.present, r.mapName, r.lobbyName, r.counted = nil, nil, "", "", nil
	r.changed, r.touched = false, false
}

// Players returns the list, most recently seen first. Never nil.
func (r *Recent) Players() []dto.RecentPlayer {
	out := slices.Clone(r.players)
	if out == nil {
		out = []dto.RecentPlayer{}
	}
	return out
}

// Present returns the BattleTags of the other humans in the current lobby
// or match. Never nil.
func (r *Recent) Present() []string {
	out := slices.Clone(r.present)
	if out == nil {
		out = []string{}
	}
	return out
}

// Unsaved reports whether players or maps changed (changed) or only
// last-seen times moved (touched) since MarkSaved.
func (r *Recent) Unsaved() (changed, touched bool) { return r.changed, r.touched }

// MarkSaved records that the list was saved.
func (r *Recent) MarkSaved() { r.changed, r.touched = false, false }

func (r *Recent) touch() {
	if len(r.present) == 0 {
		return
	}
	now := r.now()
	if r.counted == nil {
		r.counted = map[string]bool{}
	}
	for _, tag := range r.present {
		key := tagKey(tag)
		newLobby := !r.counted[key]
		r.counted[key] = true
		i := slices.IndexFunc(r.players, func(p dto.RecentPlayer) bool { return tagKey(p.Tag) == key })
		if i < 0 {
			r.players = append(r.players, dto.RecentPlayer{Tag: tag, Map: r.mapName, LastSeen: now, FirstSeen: now, Lobbies: 1})
			r.changed = true
			continue
		}
		p := r.players[i]
		if p.Map != r.mapName || p.Tag != tag || newLobby {
			r.changed = true
		}
		p.Tag, p.Map, p.LastSeen = tag, r.mapName, now
		if newLobby {
			p.Lobbies++
		}
		r.players[i] = p
	}
	r.touched = true
	r.order()
}

// order sorts the list newest first and keeps dto.MaxRecentPlayers.
func (r *Recent) order() {
	slices.SortStableFunc(r.players, func(a, b dto.RecentPlayer) int { return b.LastSeen.Compare(a.LastSeen) })
	if len(r.players) > dto.MaxRecentPlayers {
		r.players = r.players[:dto.MaxRecentPlayers]
	}
}

// lobbyOthers returns the BattleTags of the humans in a lobby update other
// than the player: occupied human slots, observers included. Computers,
// open and closed slots, and names that aren't BattleTags are left out.
func lobbyOthers(p map[string]any) []string {
	players, _ := list(p["players"])
	var self []string
	for _, raw := range players {
		if x := object(raw); isTrue(x["isSelf"]) {
			name, _ := x["name"].(string)
			self = append(self, name)
		}
	}
	var out []string
	for _, raw := range players {
		x := object(raw)
		name, _ := x["name"].(string)
		name = strings.TrimSpace(name)
		if isTrue(x["isSelf"]) || !numberIs(x["slotType"], slotHuman) || !numberIs(x["slotStatus"], slotOccupied) ||
			!tagPattern.MatchString(name) || slices.ContainsFunc(self, func(s string) bool { return tagKey(s) == tagKey(name) }) ||
			slices.ContainsFunc(out, func(t string) bool { return tagKey(t) == tagKey(name) }) {
			continue
		}
		out = append(out, name)
	}
	return out
}

// lobbyMap names a lobby's map: mapData's name, else the map file's name
// without its folder and extension, else the lobby's name.
func lobbyMap(p map[string]any) string {
	data := object(p["mapData"])
	if name, _ := data["mapName"].(string); strings.TrimSpace(colorCode.ReplaceAllString(name, "")) != "" {
		return strings.TrimSpace(colorCode.ReplaceAllString(name, ""))
	}
	if file, _ := data["mapPath"].(string); strings.TrimSpace(file) != "" {
		base := path.Base(strings.ReplaceAll(strings.TrimSpace(file), `\`, "/"))
		if name := strings.TrimSpace(strings.TrimSuffix(base, path.Ext(base))); name != "" && name != "." && name != "/" {
			return name
		}
	}
	name, _ := p["lobbyName"].(string)
	return strings.TrimSpace(name)
}
