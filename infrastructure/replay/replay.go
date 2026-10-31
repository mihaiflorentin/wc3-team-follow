// Package replay reads Warcraft III replay files (.w3g), including current
// Reforged replays, into dto.Replay.
//
// The decoder is our own, written from two references:
//   - gowarcraft3 by Niels A.D. (https://github.com/nielsAD/gowarcraft3,
//     MPL-2.0): the header and block layout and the w3g_format.txt notes on
//     leave records, from which the winner rules come. No gowarcraft3 code is
//     copied into this package.
//   - w3gjs (https://github.com/PBug90/w3gjs, MIT): Reforged's 0x38/0x39
//     player metadata records and their protobuf messages, the action
//     lengths, the post-2.0.2 action id shift and the APM counting rules.
//
// See NOTICE in this folder. The package only reads the file it is given;
// it writes nothing and contacts nothing.
package replay

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Reader reads replay files from disk.
type Reader struct{}

// New returns a replay reader.
func New() contract.ReplayReader { return Reader{} }

// ReadFile parses the replay at path.
func (Reader) ReadFile(ctx context.Context, path string) (dto.Replay, error) {
	if err := ctx.Err(); err != nil {
		return dto.Replay{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return dto.Replay{}, fmt.Errorf("open replay: %w", err)
	}
	defer func() { _ = f.Close() }()
	return Parse(f)
}

// Parse reads one replay. Damage after the slot table (an unknown record or
// action) does not fail the parse: what was read is returned, with Warnings
// saying where reading stopped.
func Parse(r io.Reader) (dto.Replay, error) {
	file, err := io.ReadAll(io.LimitReader(r, maxFileSize+1))
	if err != nil {
		return dto.Replay{}, fmt.Errorf("read replay: %w", err)
	}
	if len(file) > maxFileSize {
		return dto.Replay{}, fmt.Errorf("replay is larger than %d bytes", maxFileSize)
	}
	h, body, err := readHeader(file)
	if err != nil {
		return dto.Replay{}, err
	}
	out := dto.Replay{
		VersionNumber: h.version,
		Version:       formatVersion(h.version),
		Build:         h.build,
		Expansion:     h.product == "PX3W",
		Multiplayer:   h.multiplayer,
		Duration:      time.Duration(h.durationMS) * time.Millisecond,
		SaverID:       -1,
	}
	warn := func(format string, args ...any) { out.Warnings = append(out.Warnings, fmt.Sprintf(format, args...)) }
	data, err := decompress(h, body, warn)
	if err != nil {
		return dto.Replay{}, fmt.Errorf("decompress replay: %w", err)
	}
	c := cursor{b: data}
	meta, err := readMetadata(&c, &out, warn)
	if err != nil {
		return dto.Replay{}, fmt.Errorf("read game info: %w", err)
	}
	out.Post202 = meta.post202
	g := readGameData(&c, meta.post202, warn)
	out.GameTime = g.elapsed
	out.Ended = g.clock.ended()
	out.Leaves = g.leaves
	out.Chat = mergeChat(g.chat, g.actionChat)
	out.Orders = g.orders
	if len(g.leaves) > 0 {
		out.SaverID = g.leaves[len(g.leaves)-1].PlayerID
	}
	ids := make([]int, 0, len(g.unknown))
	for id := range g.unknown {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)
	for _, id := range ids {
		warn("unknown action 0x%02X stopped %d command blocks; their remaining actions were not counted", id, g.unknown[byte(id)])
	}

	out.Players = buildPlayers(meta, h.version)
	for i := range out.Players {
		p := &out.Players[i]
		if p.Computer {
			continue
		}
		if s := g.stats[p.ID]; s != nil {
			p.Actions, p.APMActions = s.all, s.apm
			p.LastAction = g.lastAction[p.ID]
			if mins := p.LastAction.Minutes(); mins > 0 {
				p.APM = int(float64(s.apm)/mins + 0.5)
			}
		}
		for _, l := range g.leaves {
			if l.PlayerID != p.ID {
				continue
			}
			p.Left = l.Time
			// A local leave record describes the saver's result, not the
			// leaving player's, except for the saver's own.
			if l.RawReason != reasonLocal || l.PlayerID == out.SaverID {
				p.Result = l.Result
			}
		}
	}
	out.Winner = decideWinner(out.Players, out.Leaves)
	return out, nil
}

// formatVersion turns the header version into the usual form: 26 is
// "1.26", 10032 is "1.32", 10100 is "2.00" and 10200 is "3.00".
func formatVersion(v uint32) string {
	switch {
	case v == 10030:
		return "1.30.2+"
	case v > 10030 && v < 10100:
		return fmt.Sprintf("1.%02d", v%100)
	case v >= 10100:
		return strconv.Itoa(int(v/100-99)) + fmt.Sprintf(".%02d", v%100)
	}
	return fmt.Sprintf("1.%02d", v)
}
