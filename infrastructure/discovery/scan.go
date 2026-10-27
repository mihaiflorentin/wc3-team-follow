package discovery

import (
	"bytes"
	"context"
	"time"
)

const (
	scanBudget = 12 * time.Second
	scanLimit  = 2 << 30
	chunkSize  = 1 << 20
	// chunkOverlap keeps a session that straddles two reads findable.
	chunkOverlap = 128
)

var sessionPrefixes = [][]byte{
	[]byte("/webui/index.html?guid="),
	[]byte("/webui-socket/"),
}

// searchSession returns the first 11–20 digit session that follows one of
// the menu URL prefixes in buf, or "".
func searchSession(buf []byte) string {
	for _, prefix := range sessionPrefixes {
		rest := buf
		for {
			i := bytes.Index(rest, prefix)
			if i < 0 {
				break
			}
			start := i + len(prefix)
			end := start
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			// The digits must be followed by something: a session cut off
			// at the end of the buffer is found again in the next chunk.
			if n := end - start; end < len(rest) && n >= 11 && n <= 20 {
				return string(rest[start:end])
			}
			rest = rest[start:]
		}
	}
	return ""
}

// region is a span of the game's address space to search.
type region struct {
	start, end uint64
}

// readAt reads game memory at addr into buf and returns how many bytes it
// read. A partial read is fine.
type readAt func(buf []byte, addr uint64) (int, error)

// scanRegions searches regions in order until it finds a session, runs out
// of time or has read scanLimit bytes. Unreadable chunks are skipped.
func scanRegions(ctx context.Context, regions []region, read readAt) string {
	deadline := time.Now().Add(scanBudget)
	buf := make([]byte, chunkSize)
	var total uint64
	for _, r := range regions {
		for addr := r.start; addr < r.end; {
			if ctx.Err() != nil || time.Now().After(deadline) || total >= scanLimit {
				return ""
			}
			count := min(uint64(len(buf)), r.end-addr)
			n, _ := read(buf[:count], addr)
			total += count
			if n > 0 {
				if s := searchSession(buf[:n]); s != "" {
					return s
				}
			}
			if count > chunkOverlap {
				addr += count - chunkOverlap
			} else {
				addr += count
			}
		}
	}
	return ""
}
