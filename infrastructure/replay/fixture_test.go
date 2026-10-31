package replay

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"hash/crc32"
	"testing"
)

// The helpers below build synthetic replays byte by byte, following the
// format the parser reads, so the tests need no real replay file.

// buf appends little-endian values.
type buf struct{ b []byte }

func (w *buf) u8(v byte) *buf     { w.b = append(w.b, v); return w }
func (w *buf) u16(v uint16) *buf  { w.b = binary.LittleEndian.AppendUint16(w.b, v); return w }
func (w *buf) u32(v uint32) *buf  { w.b = binary.LittleEndian.AppendUint32(w.b, v); return w }
func (w *buf) raw(v []byte) *buf  { w.b = append(w.b, v...); return w }
func (w *buf) cstr(s string) *buf { w.b = append(append(w.b, s...), 0); return w }

// encodeReplay wraps decompressed replay data in a header and zlib blocks of
// 8192 bytes, the last one padded with zeros as the game does.
func encodeReplay(t *testing.T, version uint32, build uint16, durationMS uint32, data []byte) []byte {
	t.Helper()
	const blockSize = 8192
	padded := append([]byte(nil), data...)
	if rem := len(padded) % blockSize; rem != 0 || len(padded) == 0 {
		padded = append(padded, make([]byte, blockSize-rem)...)
	}
	var blocks buf
	n := 0
	for off := 0; off < len(padded); off += blockSize {
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		if _, err := zw.Write(padded[off : off+blockSize]); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
		head := (&buf{}).u32(uint32(z.Len())).u32(blockSize).u32(0).b
		blocks.u32(uint32(z.Len())).u32(blockSize).u16(fold(crc32.ChecksumIEEE(head))).u16(fold(crc32.ChecksumIEEE(z.Bytes())))
		blocks.raw(z.Bytes())
		n++
	}
	var h buf
	h.raw([]byte(signature)).u32(68).u32(uint32(68 + len(blocks.b))).u32(1).u32(uint32(len(padded))).u32(uint32(n))
	h.raw([]byte("PX3W")).u32(version).u16(build).u16(0x8000).u32(durationMS).u32(0)
	binary.LittleEndian.PutUint32(h.b[64:], crc32.ChecksumIEEE(h.b))
	return append(h.b, blocks.b...)
}

// encodeMeta reverses the game's encoding of the map settings string: each
// group of seven bytes is preceded by a mask byte, and even bytes are stored
// plus one so that no byte is zero.
func encodeMeta(plain []byte) []byte {
	var out []byte
	for i := 0; i < len(plain); i += 7 {
		end := min(i+7, len(plain))
		mask := byte(1)
		var chunk []byte
		for j, c := range plain[i:end] {
			if c%2 == 0 {
				chunk = append(chunk, c+1)
			} else {
				mask |= 1 << (j + 1)
				chunk = append(chunk, c)
			}
		}
		out = append(out, mask)
		out = append(out, chunk...)
	}
	return out
}

type fxPlayer struct {
	id   byte
	name string
}

type fxSlot struct {
	playerID, status, computer, team, color, race, ai, handicap byte
}

type fxGame struct {
	gameName  string
	mapPath   string
	creator   string
	host      fxPlayer
	players   []fxPlayer
	reforged  []byte // 0x38/0x39 records, written between players and slots
	slots     []fxSlot
	gameData  []byte
	checksum  uint32
	sha1      [20]byte
	speedByte byte
}

func defaultGame() fxGame {
	return fxGame{
		gameName: "Sample game",
		mapPath:  "Maps/Download/Sample (4).w3x",
		creator:  "Map Maker",
		host:     fxPlayer{id: 1, name: "Player1"},
		players:  []fxPlayer{{id: 2, name: "Player2"}},
		slots: []fxSlot{
			{playerID: 1, status: 2, team: 0, color: 0, race: 0x01, ai: 1, handicap: 100},
			{playerID: 2, status: 2, team: 1, color: 1, race: 0x02, ai: 1, handicap: 90},
		},
		checksum:  0xDEADBEEF,
		speedByte: 2,
	}
}

// bytes returns the decompressed replay data for the game.
func (g fxGame) bytes() []byte {
	var meta buf
	meta.u8(g.speedByte).u8(0x30).u8(0x06).u8(0x00).raw(make([]byte, 5)).u32(g.checksum)
	meta.cstr(g.mapPath).cstr(g.creator).u8(0).raw(g.sha1[:])

	var w buf
	w.u32(0x110).u8(0x00)
	w.u8(g.host.id).cstr(g.host.name).u8(1).u8(0)
	w.cstr(g.gameName).cstr("").raw(encodeMeta(meta.b)).u8(0)
	w.u32(uint32(len(g.slots))).raw([]byte{0x09, 0, 0, 0}).raw([]byte{0, 0, 0, 0})
	for _, p := range g.players {
		w.u8(0x16).u8(p.id).cstr(p.name).u8(1).u8(0).u32(0)
	}
	w.raw(g.reforged)
	w.u8(0x19).u16(uint16(1 + 9*len(g.slots) + 6)).u8(byte(len(g.slots)))
	for _, s := range g.slots {
		w.raw([]byte{s.playerID, 100, s.status, s.computer, s.team, s.color, s.race, s.ai, s.handicap})
	}
	w.u32(12345).u8(0).u8(byte(len(g.slots)))
	w.raw(g.gameData)
	return w.b
}

// reforgedRecord builds a 0x38 or 0x39 player metadata record.
func reforgedRecord(id, subtype byte, body []byte) []byte {
	return (&buf{}).u8(id).u8(subtype).u32(uint32(len(body))).raw(body).b
}

// pbVarint and pbString build protobuf fields.
func pbVarint(field int, v uint64) []byte {
	out := binary.AppendUvarint(nil, uint64(field<<3))
	return binary.AppendUvarint(out, v)
}

func pbString(field int, s string) []byte {
	out := binary.AppendUvarint(nil, uint64(field<<3|2))
	out = binary.AppendUvarint(out, uint64(len(s)))
	return append(out, s...)
}

// timeSlot builds a 0x1F record with command blocks per player.
func timeSlot(incrementMS uint16, commands ...[]byte) []byte {
	var body []byte
	for _, c := range commands {
		body = append(body, c...)
	}
	return (&buf{}).u8(0x1F).u16(uint16(2 + len(body))).u16(incrementMS).raw(body).b
}

func command(playerID byte, actions ...[]byte) []byte {
	var body []byte
	for _, a := range actions {
		body = append(body, a...)
	}
	return (&buf{}).u8(playerID).u16(uint16(len(body))).raw(body).b
}

func leave(reason uint32, playerID byte, result uint32, unknown uint32) []byte {
	return (&buf{}).u8(0x17).u32(reason).u8(playerID).u32(result).u32(unknown).b
}

func chat(playerID byte, mode uint32, msg string) []byte {
	body := (&buf{}).u8(0x20).u32(mode).cstr(msg).b
	return (&buf{}).u8(0x20).u8(playerID).u16(uint16(len(body))).raw(body).b
}
