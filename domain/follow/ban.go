package follow

import (
	"regexp"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
	"slices"
	"strings"
	"time"
)

// CmdBanPlayer is the command the game's own Ban menu item sends from a
// lobby slot's name menu (GlueManager.js on 3.0.1.24371), payload {slot}.
// Only SendPendingBan sends it (docs/adr/0008).
const CmdBanPlayer = "BanPlayerFromGameLobby"

// banWait is how long a ban the game hasn't answered blocks the next one.
// The game answered within about 70 ms when checked live.
const banWait = 3 * time.Second

// bannedLine is the game's lobby chat confirmation of a ban (checked live
// on 3.0.1.24371): "Host banned player [Name#1234]".
var bannedLine = regexp.MustCompile(`^Host banned player \[(.+)\]$`)

// banRequest is one ban: who is meant, in which slot, and when it was sent.
type banRequest struct {
	tag    string
	slot   any
	sentAt time.Time
}

// banState is auto-ban's progress in the lobby the player is in.
type banState struct {
	// players are the slots of the newest update of the lobby the player
	// hosts; nil outside one.
	players []any
	// banned holds the blacklisted players already banned from this lobby
	// while they are still listed; one who drops out of a lobby update is
	// forgotten, so a rejoin is banned again.
	banned map[string]bool
	// locked mirrors LobbyCountdown's details.countdownLocked: nothing is
	// banned once the game won't stop the countdown.
	locked bool
	// pending is the ban decided on the newest lobby update, waiting for
	// SendPendingBan.
	pending *banRequest
	// sent is the latest ban the game hasn't confirmed yet, while it
	// blocks the next one: until the confirmation, a lobby update without
	// that player in that slot, or banWait.
	sent *banRequest
	// awaiting lists bans sent and not confirmed yet, oldest first, to
	// check each confirmation against; they expire after banWait.
	awaiting []banRequest
}

// TakeBanMistakes returns, once, the warnings about players the game
// banned instead of the blacklisted player meant, for the log.
func (e *Engine) TakeBanMistakes() []string {
	out := e.banMistakes
	e.banMistakes = nil
	return out
}

// TakeIgnoredBanLines returns, once, the senders of ban confirmations that
// weren't sent as the player and were ignored, for the log. A sender is a
// display name or BattleTag; "" is a line without one.
func (e *Engine) TakeIgnoredBanLines() []string {
	out := e.ignoredBanLines
	e.ignoredBanLines = nil
	return out
}

// clanPrefix is the "<CLAN> " the game may put before a sender's name.
var clanPrefix = regexp.MustCompile(`^<[^<>]*>\s*`)

// sentBySelf reports whether a chat line's sender is the player: their
// BattleTag or its name part, in any case, after an optional clan prefix.
// An unknown own tag or an empty sender is never the player.
func (e *Engine) sentBySelf(sender string) bool {
	name := tagKey(clanPrefix.ReplaceAllString(strings.TrimSpace(sender), ""))
	self := tagKey(e.selfTag)
	if name == "" || self == "" {
		return false
	}
	short, _, _ := strings.Cut(self, "#")
	return name == self || name == short
}

// blacklistedSlot reports whether slot x is an occupied human slot, not
// the player's own, whose non-empty name is on the blacklist, and returns
// that name.
func (e *Engine) blacklistedSlot(x map[string]any) (string, bool) {
	name, _ := x["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" || isTrue(x["isSelf"]) || e.isSelf(name) || !numberIs(x["slotType"], slotHuman) ||
		!numberIs(x["slotStatus"], slotOccupied) || !e.blacklisted(name) {
		return "", false
	}
	return name, true
}

// canBan reports whether auto-ban may act now: on, before the countdown
// locks and before the match.
func (e *Engine) canBan() bool {
	return e.autoBan && !e.ban.locked && !e.inMatch && e.ban.players != nil
}

// banBlacklisted reads a lobby update and, in a lobby the player hosts,
// picks one blacklisted player in it to ban (docs/adr/0008). Only occupied
// human slots with a non-empty name that matches a blacklisted BattleTag
// count; never the player's own slot, computers or open slots. Each player
// is banned at most once while they stay listed, one ban is in flight at a
// time, and nothing is banned once the countdown is locked or the match
// has started.
//
// Nothing is sent here: the game's Ban takes only a slot, so the companion
// first reads every event already waiting, then calls SendPendingBan,
// which checks the choice against the newest lobby update.
func (e *Engine) banBlacklisted(p map[string]any) {
	b := &e.ban
	if !isTrue(p["isHost"]) {
		*b = banState{}
		return
	}
	players, ok := list(p["players"])
	if !ok {
		return
	}
	b.players = players
	now := e.now()
	b.awaiting = slices.DeleteFunc(b.awaiting, func(r banRequest) bool { return now.Sub(r.sentAt) >= banWait })
	if r := b.sent; r != nil && (now.Sub(r.sentAt) >= banWait || !e.holdsSlot(r.tag, r.slot)) {
		b.sent = nil
	}
	present := map[string]bool{}
	for _, raw := range players {
		x := object(raw)
		name, ok := e.blacklistedSlot(x)
		if !ok {
			continue
		}
		key := tagKey(name)
		present[key] = true
		if b.banned[key] || b.sent != nil || b.pending != nil || !e.canBan() {
			continue
		}
		if b.banned == nil {
			b.banned = map[string]bool{}
		}
		b.banned[key] = true
		b.pending = &banRequest{tag: name, slot: x["slot"]}
	}
	for key := range b.banned {
		if !present[key] {
			delete(b.banned, key)
		}
	}
}

// HasPendingBan reports whether a ban waits for SendPendingBan.
func (e *Engine) HasPendingBan() bool { return e.ban.pending != nil }

// SendPendingBan sends the ban banBlacklisted chose, after the companion
// has fed the engine every game event already waiting. It looks for the
// player again in the newest lobby update: it bans the slot they are in
// now, or nobody when they left or the lobby, countdown or match no longer
// allows it. It is never called from a timer.
func (e *Engine) SendPendingBan() {
	b := &e.ban
	r := b.pending
	if r == nil {
		return
	}
	b.pending = nil
	slot, found := any(nil), false
	if e.canBan() {
		for _, raw := range b.players {
			x := object(raw)
			if name, ok := e.blacklistedSlot(x); ok && tagKey(name) == tagKey(r.tag) {
				slot, found = x["slot"], true
				break
			}
		}
	}
	if !found {
		delete(b.banned, tagKey(r.tag))
		return
	}
	if !e.send(CmdBanPlayer, map[string]any{"slot": slot}) {
		return
	}
	sent := banRequest{tag: r.tag, slot: slot, sentAt: e.now()}
	b.sent = &sent
	b.awaiting = append(b.awaiting, sent)
	e.Note("Banned " + r.tag + " (on your blacklist).")
}

// confirmBan reads the game's ban confirmation in the lobby chat and checks
// it against the bans still waiting for one. A confirmation naming a player
// we meant confirms that ban. One naming who the newest lobby update shows
// in a slot we banned means the slot changed hands just before the ban:
// unless that player is blacklisted too, the player is warned, by name,
// since nothing can undo a ban. Any other confirmation is the player's own
// ban (bannedByHand), and our bans keep waiting. The line is the English
// client's; other languages don't match it.
//
// Anyone in the lobby can type the same text, so a line counts only when
// the game sent it as the local player (the host): one with any other
// sender, or none, is ignored and kept for TakeIgnoredBanLines.
func (e *Engine) confirmBan(p map[string]any) {
	m := object(p["message"])
	content, _ := m["content"].(string)
	match := bannedLine.FindStringSubmatch(strings.TrimSpace(content))
	if m["source"] != "gameChat" || match == nil {
		return
	}
	if sender, _ := m["sender"].(string); !e.sentBySelf(sender) {
		e.ignoredBanLines = append(e.ignoredBanLines, strings.TrimSpace(sender))
		return
	}
	b := &e.ban
	now := e.now()
	b.awaiting = slices.DeleteFunc(b.awaiting, func(r banRequest) bool { return now.Sub(r.sentAt) >= banWait })
	who := strings.TrimSpace(match[1])
	i := slices.IndexFunc(b.awaiting, func(r banRequest) bool { return tagKey(r.tag) == tagKey(who) })
	if i < 0 {
		// Not who we meant: ours only when the newest lobby update shows
		// who in a slot one of our bans targeted. Otherwise the player
		// banned someone by hand, and our bans keep waiting.
		i = slices.IndexFunc(b.awaiting, func(r banRequest) bool { return e.holdsSlot(who, r.slot) })
	}
	if i < 0 {
		e.bannedByHand(who)
		return
	}
	meant := b.awaiting[i]
	b.awaiting = slices.Delete(b.awaiting, i, i+1)
	if b.sent != nil && tagKey(b.sent.tag) == tagKey(meant.tag) {
		b.sent = nil
	}
	if tagKey(who) == tagKey(meant.tag) || e.blacklisted(who) {
		return
	}
	warning := "Banned " + who + " by mistake: " + meant.tag + " left just before the ban."
	e.banMistakes = append(e.banMistakes, warning)
	e.Note(warning)
}

// holdsSlot reports whether the newest lobby update shows tag in slot.
func (e *Engine) holdsSlot(tag string, slot any) bool {
	return slices.ContainsFunc(e.ban.players, func(raw any) bool {
		x := object(raw)
		name, _ := x["name"].(string)
		return sameNumber(x["slot"], slot) && tagKey(name) == tagKey(tag)
	})
}

// bannedByHand adds a player the player banned by hand, in a lobby they
// host, to the blacklist, when that switch is on. Text that isn't a
// BattleTag, the player's own tag and players already listed are left
// alone.
func (e *Engine) bannedByHand(who string) {
	if !e.manualBans || e.ban.players == nil || !tagPattern.MatchString(who) || e.isSelf(who) || e.blacklisted(who) ||
		len(e.blacklist) >= dto.MaxBlacklist {
		return
	}
	e.blacklist = append([]dto.BlacklistEntry{{Tag: who, Added: e.now().UTC()}}, e.blacklist...)
	if kept := slices.DeleteFunc(slices.Clone(e.manualTags), e.blacklisted); len(kept) != len(e.manualTags) {
		e.manualTags = kept
		e.selectionChanged()
	}
	e.blacklistChanged = true
	e.Note("Added " + who + " to your blacklist (you banned them).")
}
