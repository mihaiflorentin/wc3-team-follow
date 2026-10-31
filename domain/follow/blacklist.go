package follow

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Blacklist returns the blacklist, most recently added first.
func (e *Engine) Blacklist() []dto.BlacklistEntry { return slices.Clone(e.blacklist) }

// AutoBan reports whether blacklisted players are banned automatically.
func (e *Engine) AutoBan() bool { return e.autoBan }

// BlacklistManualBans reports whether players banned by hand join the
// blacklist.
func (e *Engine) BlacklistManualBans() bool { return e.manualBans }

// TakeBlacklistChanged reports, once, that the engine itself changed the
// blacklist (a ban by hand), so the companion saves it.
func (e *Engine) TakeBlacklistChanged() bool {
	changed := e.blacklistChanged
	e.blacklistChanged = false
	return changed
}

func (e *Engine) blacklisted(tag string) bool {
	return slices.ContainsFunc(e.blacklist, func(b dto.BlacklistEntry) bool { return tagKey(b.Tag) == tagKey(tag) })
}

// configureBlacklist applies the blacklist part of cfg: replace, remove,
// add, then the Auto-ban switch. Blacklisted players leave the follow
// group.
func (e *Engine) configureBlacklist(cfg dto.FollowConfig) error {
	if cfg.Blacklist != nil {
		var list []dto.BlacklistEntry
		for _, b := range *cfg.Blacklist {
			tag := strings.TrimSpace(b.Tag)
			if !tagPattern.MatchString(tag) || slices.ContainsFunc(list, func(x dto.BlacklistEntry) bool { return tagKey(x.Tag) == tagKey(tag) }) {
				continue
			}
			list = append(list, dto.BlacklistEntry{Tag: tag, Added: b.Added.UTC(), Reason: reasonText(b.Reason)})
		}
		slices.SortStableFunc(list, func(a, b dto.BlacklistEntry) int { return b.Added.Compare(a.Added) })
		if len(list) > dto.MaxBlacklist {
			list = list[:dto.MaxBlacklist]
		}
		e.blacklist = list
	}
	for _, raw := range cfg.BlacklistRemove {
		e.blacklist = slices.DeleteFunc(e.blacklist, func(b dto.BlacklistEntry) bool { return tagKey(b.Tag) == tagKey(raw) })
	}
	if len(cfg.BlacklistAdd) > 0 {
		var added []string
		for _, raw := range cfg.BlacklistAdd {
			tag := strings.TrimSpace(raw)
			switch {
			case !tagPattern.MatchString(tag):
				return errors.New("Enter a full BattleTag, such as Player#1234.")
			case e.isSelf(tag):
				return errors.New("You can't blacklist yourself.")
			}
			if !e.blacklisted(tag) && !slices.ContainsFunc(added, func(a string) bool { return tagKey(a) == tagKey(tag) }) {
				added = append(added, tag)
			}
		}
		if len(e.blacklist)+len(added) > dto.MaxBlacklist {
			return fmt.Errorf("The blacklist holds up to %d players. Remove some first.", dto.MaxBlacklist)
		}
		now := e.now().UTC()
		entries := make([]dto.BlacklistEntry, 0, len(added))
		for _, tag := range added {
			entries = append(entries, dto.BlacklistEntry{Tag: tag, Added: now})
		}
		e.blacklist = append(entries, e.blacklist...)
		switch len(added) {
		case 0:
		case 1:
			e.Note("Added " + added[0] + " to your blacklist.")
		default:
			e.Note(fmt.Sprintf("Added %d players to your blacklist.", len(added)))
		}
	}
	if cfg.AutoBan != nil {
		e.autoBan = *cfg.AutoBan
	}
	if cfg.BlacklistManualBans != nil {
		e.manualBans = *cfg.BlacklistManualBans
	}
	if cfg.BlacklistLeavers != nil {
		e.leavers = *cfg.BlacklistLeavers
	}
	// Never follow and ban the same player.
	if kept := slices.DeleteFunc(slices.Clone(e.manualTags), e.blacklisted); len(kept) != len(e.manualTags) {
		e.manualTags = kept
		e.selectionChanged()
	}
	return nil
}

// maxReason is how many characters a blacklist reason keeps.
const maxReason = 300

// reasonText trims a saved reason to one plain line of at most maxReason
// characters.
func reasonText(reason string) string {
	reason = strings.Join(strings.Fields(reason), " ")
	if r := []rune(reason); len(r) > maxReason {
		reason = string(r[:maxReason])
	}
	return reason
}

// isSelf reports whether tag is the player's own BattleTag, as the team or
// a lobby showed it.
func (e *Engine) isSelf(tag string) bool {
	return e.selfTag != "" && tagKey(e.selfTag) == tagKey(tag)
}
