package replay

import (
	"fmt"
	"sort"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Record ids in the game data part.
const (
	recLeave     = 0x17
	recStart1    = 0x1A
	recStart2    = 0x1B
	recStart3    = 0x1C
	recTimeSlot0 = 0x1E
	recTimeSlot  = 0x1F
	recChat      = 0x20
	recChecksum  = 0x22
	recUnknown23 = 0x23
	recUnknown2F = 0x2F
)

// gameData is what the records after the slot table add up to.
type gameData struct {
	elapsed    time.Duration
	leaves     []dto.ReplayLeave
	chat       []dto.ReplayChat
	stats      map[int]*actionStats
	lastAction map[int]time.Duration
	unknown    map[byte]int // unknown action ids and how often each stopped a block
	orders     []dto.ReplayOrder
	// actionChat holds the chat lines read from map chat actions, before
	// the ones the chat records also hold are dropped.
	actionChat []dto.ReplayChat
}

func readGameData(c *cursor, post202 bool, warn func(string, ...any)) gameData {
	g := gameData{stats: map[int]*actionStats{}, lastAction: map[int]time.Duration{}, unknown: map[byte]int{}}
	for c.left() > 0 {
		start := c.off
		id := c.u8()
		switch id {
		case 0x00:
			// The last block is padded with zeros.
			return g
		case recLeave:
			l := dto.ReplayLeave{Time: g.elapsed, RawReason: c.u32(), PlayerID: int(c.u8()), RawResult: c.u32(), Counter: c.u32()}
			l.Reason, l.Result = leaveReason(l.RawReason), leaveResult(l.RawResult)
			g.leaves = append(g.leaves, l)
		case recStart1, recStart2, recStart3:
			c.skip(4)
		case recTimeSlot, recTimeSlot0:
			n := int(c.u16())
			slot := cursor{b: c.take(n)}
			g.elapsed += time.Duration(slot.u16()) * time.Millisecond
			for slot.left() > 0 && slot.err == nil {
				pid := int(slot.u8())
				actions := slot.take(int(slot.u16()))
				if slot.err != nil {
					break
				}
				s := g.stats[pid]
				if s == nil {
					s = &actionStats{}
					g.stats[pid] = s
				}
				g.lastAction[pid] = g.elapsed
				var found blockFinds
				if bad, ok := readActions(actions, post202, s, &found); !ok {
					g.unknown[bad]++
				}
				for _, code := range found.codes {
					g.orders = append(g.orders, dto.ReplayOrder{Time: g.elapsed, PlayerID: pid, Code: code})
				}
				for _, line := range found.chat {
					g.actionChat = append(g.actionChat, dto.ReplayChat{Time: g.elapsed, PlayerID: pid, Mode: "unseen", Message: line})
				}
			}
			if slot.err != nil {
				warn("time slot at %d: %v", start, slot.err)
			}
		case recChat:
			pid := int(c.u8())
			body := cursor{b: c.take(int(c.u16()))}
			line := dto.ReplayChat{Time: g.elapsed, PlayerID: pid, Mode: "lobby"}
			if body.u8() == 0x20 {
				line.Mode = chatMode(body.u32())
			}
			line.Message = body.cstr()
			g.chat = append(g.chat, line)
		case recChecksum:
			c.skip(int(c.u8()))
		case recUnknown23:
			c.skip(10)
		case recUnknown2F:
			c.skip(8)
		default:
			warn("unknown game data record 0x%02X at %d; stopped reading", id, start)
			return g
		}
		if c.err != nil {
			warn("game data record 0x%02X at %d: %v", id, start, c.err)
			return g
		}
	}
	return g
}

func leaveReason(v uint32) string {
	switch v {
	case 0x01:
		return "remote"
	case 0x0C:
		return "local"
	case 0x0E:
		return "remote (0x0E)"
	}
	return fmt.Sprintf("0x%02X", v)
}

func leaveResult(v uint32) string {
	switch v {
	case 0x01:
		return "left"
	case 0x07:
		return "left (0x07)"
	case 0x08:
		return "lost"
	case 0x09:
		return "won"
	case 0x0A:
		return "draw"
	case 0x0B:
		return "observer left"
	case 0x0D:
		// Seen from 1.32 to 3.0 for players who quit, mid-game or at the end;
		// its meaning is not documented.
		return "left (0x0D)"
	}
	return fmt.Sprintf("0x%02X", v)
}

func chatMode(v uint32) string {
	switch v {
	case 0x00:
		return "all"
	case 0x01:
		return "allies"
	case 0x02:
		return "observers"
	}
	return "private"
}

// sameLine is how far apart a chat record and the map chat action for the
// same line may be. The record comes between time slots and the action in
// one, so they differ by about one slot.
const sameLine = 2 * time.Second

// mergeChat adds the map chat actions that no chat record holds: the lines
// the saver couldn't see. The game records one action for each of the map's
// chat triggers a line matches, so actions with the same player, time and
// text are one line. A record and an action match when the same player sent
// the same text within sameLine; each record matches at most one line.
func mergeChat(records, actions []dto.ReplayChat) []dto.ReplayChat {
	out := append([]dto.ReplayChat{}, records...)
	used := make([]bool, len(records))
	for k, a := range actions {
		if k > 0 && actions[k-1] == a {
			continue
		}
		seen := false
		for i, r := range records {
			if !used[i] && r.PlayerID == a.PlayerID && r.Message == a.Message && (r.Time-a.Time).Abs() <= sameLine {
				used[i], seen = true, true
				break
			}
		}
		if !seen {
			out = append(out, a)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Time < out[j].Time })
	return out
}
