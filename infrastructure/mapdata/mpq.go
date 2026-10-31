package mapdata

import (
	"bytes"
	"compress/bzip2"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// A Warcraft III map is an MPQ archive. This reader finds files by name
// through the archive's hash table; maps protected against editing often
// drop the file list and put a decoy header first, so it never needs one.
// It reads stored, zlib and bzip2 files, encrypted or not.

const (
	hashOffset = 0
	hashA      = 1
	hashB      = 2
	hashKey    = 3

	flagImplode    = 0x00000100
	flagCompress   = 0x00000200
	flagEncrypted  = 0x00010000
	flagFixKey     = 0x00020000
	flagSingleUnit = 0x01000000
	flagSectorCRC  = 0x04000000

	compressZlib  = 0x02
	compressBzip2 = 0x10

	noBlock = 0xFFFFFFFF
)

var cryptTable = func() [0x500]uint32 {
	var t [0x500]uint32
	seed := uint32(0x00100001)
	for i := range 0x100 {
		for j := i; j < 0x500; j += 0x100 {
			seed = (seed*125 + 3) % 0x2AAAAB
			hi := (seed & 0xFFFF) << 16
			seed = (seed*125 + 3) % 0x2AAAAB
			t[j] = hi | seed&0xFFFF
		}
	}
	return t
}()

// hashString is the archive's string hash; kind picks the table part.
func hashString(s string, kind uint32) uint32 {
	s1, s2 := uint32(0x7FED7FED), uint32(0xEEEEEEEE)
	for _, ch := range []byte(strings.ToUpper(s)) {
		s1 = cryptTable[kind<<8+uint32(ch)] ^ (s1 + s2)
		s2 = uint32(ch) + s1 + s2 + s2<<5 + 3
	}
	return s1
}

// decrypt decrypts whole 32-bit words of b in place; a trailing partial
// word is stored plain.
func decrypt(b []byte, key uint32) {
	seed := uint32(0xEEEEEEEE)
	for i := 0; i+4 <= len(b); i += 4 {
		seed += cryptTable[0x400+key&0xFF]
		v := binary.LittleEndian.Uint32(b[i:]) ^ (key + seed)
		key = (^key<<0x15 + 0x11111111) | key>>0x0B
		seed = v + seed + seed<<5 + 3
		binary.LittleEndian.PutUint32(b[i:], v)
	}
}

type hashEntry struct {
	a, b  uint32
	block uint32
}

type blockEntry struct {
	offset, size, fileSize, flags uint32
}

type archive struct {
	data   []byte
	base   int
	sector int
	hashes []hashEntry
	blocks []blockEntry
}

// openArchive reads the header and tables. data is the whole map file.
func openArchive(data []byte) (*archive, error) {
	base := -1
	// The real header sits at a multiple of 512; a protected map may put a
	// decoy first, with a header size no version has or tables outside the
	// file.
	for off := 0; off+32 <= len(data); off += 512 {
		h := data[off:]
		if !bytes.Equal(h[:4], []byte("MPQ\x1a")) {
			continue
		}
		switch binary.LittleEndian.Uint32(h[4:]) {
		case 32, 44, 68, 208:
		default:
			continue
		}
		if hashAt, blockAt := binary.LittleEndian.Uint32(h[16:]), binary.LittleEndian.Uint32(h[20:]); int64(hashAt) < int64(len(h)) && int64(blockAt) < int64(len(h)) {
			base = off
			break
		}
	}
	if base < 0 {
		return nil, errors.New("not a map archive")
	}
	h := data[base:]
	a := &archive{data: data, base: base, sector: 512 << binary.LittleEndian.Uint16(h[14:])}
	hashAt, blockAt := int(binary.LittleEndian.Uint32(h[16:])), int(binary.LittleEndian.Uint32(h[20:]))
	hashN, blockN := int(binary.LittleEndian.Uint32(h[24:])), int(binary.LittleEndian.Uint32(h[28:]))
	table := func(at, n int, name string) ([]byte, error) {
		if at < 0 || at > len(h) {
			return nil, fmt.Errorf("%s is outside the file", name)
		}
		// Protected maps overstate table sizes; read what the file holds.
		n = min(n, (len(h)-at)/16)
		b := append([]byte{}, h[at:at+n*16]...)
		decrypt(b, hashString(name, hashKey))
		return b, nil
	}
	hb, err := table(hashAt, hashN, "(hash table)")
	if err != nil {
		return nil, err
	}
	bb, err := table(blockAt, blockN, "(block table)")
	if err != nil {
		return nil, err
	}
	for i := 0; i+16 <= len(hb); i += 16 {
		a.hashes = append(a.hashes, hashEntry{binary.LittleEndian.Uint32(hb[i:]), binary.LittleEndian.Uint32(hb[i+4:]), binary.LittleEndian.Uint32(hb[i+12:])})
	}
	for i := 0; i+16 <= len(bb); i += 16 {
		a.blocks = append(a.blocks, blockEntry{binary.LittleEndian.Uint32(bb[i:]), binary.LittleEndian.Uint32(bb[i+4:]), binary.LittleEndian.Uint32(bb[i+8:]), binary.LittleEndian.Uint32(bb[i+12:])})
	}
	if len(a.hashes) == 0 {
		return nil, errors.New("the map archive has no files")
	}
	return a, nil
}

// errNoFile says the archive has no file of that name.
var errNoFile = errors.New("no such file in the map")

// read returns one file's contents.
func (a *archive) read(name string) ([]byte, error) {
	blk, ok := a.find(name)
	if !ok {
		return nil, errNoFile
	}
	start, end := a.base+int(blk.offset), a.base+int(blk.offset)+int(blk.size)
	if start < 0 || end > len(a.data) || end < start {
		return nil, fmt.Errorf("%s is outside the file", name)
	}
	raw := append([]byte{}, a.data[start:end]...)
	size := int(blk.fileSize)
	if blk.flags&flagImplode != 0 {
		return nil, fmt.Errorf("%s uses an old compression this reader doesn't support", name)
	}
	var key uint32
	if blk.flags&flagEncrypted != 0 {
		base := name[strings.LastIndexAny(name, `\/`)+1:]
		key = hashString(base, hashKey)
		if blk.flags&flagFixKey != 0 {
			key = (key + blk.offset) ^ blk.fileSize
		}
	}
	if blk.flags&flagSingleUnit != 0 {
		if blk.flags&flagEncrypted != 0 {
			decrypt(raw, key)
		}
		if blk.flags&flagCompress != 0 && len(raw) < size {
			return decompress(raw, size)
		}
		return trim(raw, size)
	}
	sectors := (size + a.sector - 1) / a.sector
	if blk.flags&flagCompress == 0 {
		for s := range sectors {
			if blk.flags&flagEncrypted != 0 {
				decrypt(raw[s*a.sector:min((s+1)*a.sector, len(raw))], key+uint32(s))
			}
		}
		return trim(raw, size)
	}
	count := sectors + 1
	if blk.flags&flagSectorCRC != 0 {
		count++
	}
	if len(raw) < count*4 {
		return nil, fmt.Errorf("%s is cut short", name)
	}
	offsets := append([]byte{}, raw[:count*4]...)
	if blk.flags&flagEncrypted != 0 {
		decrypt(offsets, key-1)
	}
	out := make([]byte, 0, size)
	for s := range sectors {
		from, to := int(binary.LittleEndian.Uint32(offsets[s*4:])), int(binary.LittleEndian.Uint32(offsets[s*4+4:]))
		if from < 0 || to > len(raw) || to < from {
			return nil, fmt.Errorf("%s has a bad sector table", name)
		}
		chunk := raw[from:to]
		if blk.flags&flagEncrypted != 0 {
			decrypt(chunk, key+uint32(s))
		}
		want := min(a.sector, size-s*a.sector)
		if len(chunk) < want {
			d, err := decompress(chunk, want)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", name, err)
			}
			chunk = d
		}
		out = append(out, chunk...)
	}
	return trim(out, size)
}

func (a *archive) find(name string) (blockEntry, bool) {
	n := uint32(len(a.hashes))
	ha, hb := hashString(name, hashA), hashString(name, hashB)
	start := hashString(name, hashOffset) % n
	for i := range n {
		e := a.hashes[(start+i)%n]
		if e.block == noBlock {
			return blockEntry{}, false
		}
		if e.a == ha && e.b == hb && int(e.block) < len(a.blocks) {
			return a.blocks[e.block], true
		}
	}
	return blockEntry{}, false
}

func trim(b []byte, size int) ([]byte, error) {
	if len(b) < size {
		return nil, errors.New("file is cut short")
	}
	return b[:size], nil
}

// decompress undoes one sector's compression: a mask byte, then the data.
func decompress(b []byte, size int) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New("empty sector")
	}
	mask, data := b[0], b[1:]
	if mask&^(compressZlib|compressBzip2) != 0 {
		return nil, fmt.Errorf("compression 0x%02X isn't supported", mask)
	}
	var r io.Reader = bytes.NewReader(data)
	if mask&compressBzip2 != 0 {
		r = bzip2.NewReader(r)
	}
	if mask&compressZlib != 0 {
		z, err := zlib.NewReader(r)
		if err != nil {
			return nil, err
		}
		r = z
	}
	out, err := io.ReadAll(io.LimitReader(r, int64(size)+1))
	if err != nil {
		return nil, err
	}
	return trim(out, size)
}
