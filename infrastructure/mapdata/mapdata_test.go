package mapdata

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// encrypt is decrypt's inverse, for building test archives.
func encrypt(b []byte, key uint32) {
	seed := uint32(0xEEEEEEEE)
	for i := 0; i+4 <= len(b); i += 4 {
		seed += cryptTable[0x400+key&0xFF]
		v := binary.LittleEndian.Uint32(b[i:])
		binary.LittleEndian.PutUint32(b[i:], v^(key+seed))
		key = (^key<<0x15 + 0x11111111) | key>>0x0B
		seed = v + seed + seed<<5 + 3
	}
}

type testFile struct {
	name string
	data []byte
	// sectored files are zlib-compressed in 512-byte sectors and encrypted
	// with the fixed key; the others are stored whole and plain.
	sectored bool
}

// buildArchive writes a map archive with a decoy header at 0 and the real
// one at 512, as protected maps do.
func buildArchive(t *testing.T, files []testFile) []byte {
	t.Helper()
	const sector = 512
	var body []byte // after the real header; offsets are from its start
	type placed struct {
		name                          string
		offset, size, fileSize, flags uint32
	}
	var blocks []placed
	for _, f := range files {
		offset := uint32(32 + len(body))
		if !f.sectored {
			body = append(body, f.data...)
			blocks = append(blocks, placed{f.name, offset, uint32(len(f.data)), uint32(len(f.data)), 0x80000000 | flagSingleUnit})
			continue
		}
		key := (hashString(f.name, hashKey) + offset) ^ uint32(len(f.data))
		n := (len(f.data) + sector - 1) / sector
		var chunks [][]byte
		for s := range n {
			plain := f.data[s*sector : min((s+1)*sector, len(f.data))]
			var z bytes.Buffer
			w := zlib.NewWriter(&z)
			_, _ = w.Write(plain)
			_ = w.Close()
			chunk := append([]byte{compressZlib}, z.Bytes()...)
			if len(chunk) >= len(plain) {
				chunk = append([]byte{}, plain...)
			}
			encrypt(chunk, key+uint32(s))
			chunks = append(chunks, chunk)
		}
		table := make([]byte, (n+1)*4)
		at := uint32(len(table))
		for s, c := range chunks {
			binary.LittleEndian.PutUint32(table[s*4:], at)
			at += uint32(len(c))
		}
		binary.LittleEndian.PutUint32(table[n*4:], at)
		encrypt(table, key-1)
		start := len(body)
		body = append(body, table...)
		for _, c := range chunks {
			body = append(body, c...)
		}
		blocks = append(blocks, placed{f.name, offset, uint32(len(body) - start), uint32(len(f.data)), 0x80000000 | flagCompress | flagEncrypted | flagFixKey})
	}
	const hashN = 8
	hashes := make([]byte, hashN*16)
	for i := range hashes {
		hashes[i] = 0xFF
	}
	for i, b := range blocks {
		slot := hashString(b.name, hashOffset) % hashN
		for binary.LittleEndian.Uint32(hashes[slot*16+12:]) != noBlock {
			slot = (slot + 1) % hashN
		}
		e := hashes[slot*16:]
		binary.LittleEndian.PutUint32(e, hashString(b.name, hashA))
		binary.LittleEndian.PutUint32(e[4:], hashString(b.name, hashB))
		binary.LittleEndian.PutUint32(e[8:], 0)
		binary.LittleEndian.PutUint32(e[12:], uint32(i))
	}
	blockTable := make([]byte, len(blocks)*16)
	for i, b := range blocks {
		e := blockTable[i*16:]
		binary.LittleEndian.PutUint32(e, b.offset)
		binary.LittleEndian.PutUint32(e[4:], b.size)
		binary.LittleEndian.PutUint32(e[8:], b.fileSize)
		binary.LittleEndian.PutUint32(e[12:], b.flags)
	}
	encrypt(hashes, hashString("(hash table)", hashKey))
	encrypt(blockTable, hashString("(block table)", hashKey))
	hashAt := uint32(32 + len(body))
	blockAt := hashAt + uint32(len(hashes))

	decoy := make([]byte, 512)
	copy(decoy, "MPQ\x1a\x02\x01\x20\x20")
	header := make([]byte, 32)
	copy(header, "MPQ\x1a")
	binary.LittleEndian.PutUint32(header[4:], 32)
	binary.LittleEndian.PutUint32(header[8:], blockAt+uint32(len(blockTable)))
	binary.LittleEndian.PutUint16(header[14:], 0) // 512-byte sectors
	binary.LittleEndian.PutUint32(header[16:], hashAt)
	binary.LittleEndian.PutUint32(header[20:], blockAt)
	binary.LittleEndian.PutUint32(header[24:], hashN)
	binary.LittleEndian.PutUint32(header[28:], uint32(len(blocks)))
	out := append(decoy, header...)
	out = append(out, body...)
	out = append(out, hashes...)
	return append(out, blockTable...)
}

type objWriter struct{ b []byte }

func (w *objWriter) i32(v int32) *objWriter {
	w.b = binary.LittleEndian.AppendUint32(w.b, uint32(v))
	return w
}
func (w *objWriter) id(s string) *objWriter   { w.b = append(w.b, s...); return w }
func (w *objWriter) cstr(s string) *objWriter { w.b = append(append(w.b, s...), 0); return w }

func TestNamesReadsObjectNamesFromAProtectedMap(t *testing.T) {
	// A version 3 unit table: Bowman's name in the strings file, a changed
	// game unit whose name has colour codes, and a unit without a name.
	units := (&objWriter{}).i32(3).
		i32(1).id("hpea").id("\x00\x00\x00\x00").i32(1).i32(0).i32(1).
		id("unam").i32(3).cstr("|cffffcc00Builder|r").id("hpea").
		i32(2).
		id("hrif").id("h04P").i32(1).i32(0).i32(2).
		id("uhpm").i32(0).i32(70).id("h04P").
		id("unam").i32(3).cstr("TRIGSTR_007").id("h04P").
		id("hrif").id("h04Q").i32(1).i32(0).i32(1).
		id("uhpm").i32(0).i32(80).id("h04Q").b
	// An upgrade table carries a level and a data pointer on every field.
	upgrades := (&objWriter{}).i32(3).i32(0).i32(1).
		id("Rhme").id("R003").i32(1).i32(0).i32(1).
		id("gnam").i32(3).i32(1).i32(0).cstr("Lumberjack Training").id("R003").b
	wts := "STRING 4\n{\nPlayer 1\n}\n\nSTRING 7\n// Units: h04P (Bowman), Name\n{\nBowman\n}\n" + strings.Repeat("\n", 1200)
	archive := buildArchive(t, []testFile{
		{name: "war3map.wts", data: []byte(wts), sectored: true},
		{name: "war3mapSkin.w3u", data: units, sectored: true},
		{name: "war3map.w3q", data: upgrades},
	})
	path := filepath.Join(t.TempDir(), "test.w3x")
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	names, err := New().Names(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"hpea": "Builder", "h04P": "Bowman", "R003": "Lumberjack Training"}
	if len(names) != len(want) {
		t.Errorf("names = %v", names)
	}
	for id, name := range want {
		if names[id] != name {
			t.Errorf("%s = %q, want %q (all: %v)", id, names[id], name, names)
		}
	}
}

func TestNamesRefusesAFileThatIsNotAMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("not a map", 200)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New().Names(context.Background(), path); err == nil {
		t.Error("no error for a file that isn't a map")
	}
}

func TestNamesSurvivesAMapCutAnywhere(t *testing.T) {
	units := (&objWriter{}).i32(3).i32(0).i32(1).
		id("hrif").id("h04P").i32(1).i32(0).i32(1).
		id("unam").i32(3).cstr("Bowman").id("h04P").b
	archive := buildArchive(t, []testFile{{name: "war3map.w3u", data: units, sectored: true}})
	for n := range len(archive) {
		// Must return, with or without an error, and never panic.
		_, _ = names(archive[:n])
	}
	if got, err := names(archive); err != nil || got["h04P"] != "Bowman" {
		t.Errorf("whole archive: %v %v", got, err)
	}
}
