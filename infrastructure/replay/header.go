package replay

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

// signature starts every replay file.
const signature = "Warcraft III recorded game\x1A\x00"

// Limits that keep a damaged or hostile file from using unbounded memory.
const (
	maxFileSize         = 64 << 20
	maxDecompressedSize = 256 << 20
	maxBlockSize        = 1 << 20
)

// header is the fixed 68-byte header of a version 1 replay (game 1.07 on).
type header struct {
	product     string
	version     uint32
	build       uint16
	multiplayer bool
	durationMS  uint32
	blocks      uint32
	headerSize  uint32
	totalSize   uint32
}

func readHeader(file []byte) (header, []byte, error) {
	start := bytes.Index(file, []byte(signature))
	if start < 0 {
		return header{}, nil, errors.New("not a Warcraft III replay")
	}
	file = file[start:]
	if len(file) < 68 {
		return header{}, nil, errors.New("replay header is cut short")
	}
	c := cursor{b: file, off: len(signature)}
	var h header
	h.headerSize = c.u32()
	c.u32() // compressed file size
	if v := c.u32(); v != 1 {
		return header{}, nil, fmt.Errorf("replay header version %d is not supported (only game 1.07 and later)", v)
	}
	h.totalSize = c.u32()
	h.blocks = c.u32()
	h.product = string(c.take(4))
	h.version = c.u32()
	h.build = c.u16()
	h.multiplayer = c.u16()&0x8000 != 0
	h.durationMS = c.u32()
	sum := c.u32()

	check := append([]byte(nil), file[:68]...)
	binary.LittleEndian.PutUint32(check[64:], 0)
	if crc32.ChecksumIEEE(check) != sum {
		return header{}, nil, errors.New("replay header checksum does not match")
	}
	if h.headerSize < 68 || int(h.headerSize) > len(file) {
		return header{}, nil, errors.New("replay header size is invalid")
	}
	if h.totalSize > maxDecompressedSize {
		return header{}, nil, fmt.Errorf("replay data of %d bytes is too large", h.totalSize)
	}
	return h, file[h.headerSize:], nil
}

// decompress inflates every data block and joins them. A block whose zlib
// stream ends early keeps what it produced and adds a warning.
func decompress(h header, body []byte, warn func(string, ...any)) ([]byte, error) {
	// Before game 1.32 (version 10032) block headers used 16-bit sizes.
	wide := h.version >= 10032
	headLen := 8
	if wide {
		headLen = 12
	}
	out := make([]byte, 0, h.totalSize)
	c := cursor{b: body}
	for i := uint32(0); i < h.blocks; i++ {
		head := c.take(headLen)
		if c.err != nil {
			return nil, fmt.Errorf("block %d header: %w", i, c.err)
		}
		hc := cursor{b: head}
		var compSize, size uint32
		if wide {
			compSize, size = hc.u32(), hc.u32()
		} else {
			compSize, size = uint32(hc.u16()), uint32(hc.u16())
		}
		crcHead, crcData := hc.u16(), hc.u16()
		zeroed := append([]byte(nil), head...)
		copy(zeroed[headLen-4:], []byte{0, 0, 0, 0})
		if fold(crc32.ChecksumIEEE(zeroed)) != crcHead {
			return nil, fmt.Errorf("block %d header checksum does not match", i)
		}
		if size > maxBlockSize || compSize > maxBlockSize {
			return nil, fmt.Errorf("block %d is too large", i)
		}
		data := c.take(int(compSize))
		if c.err != nil {
			return nil, fmt.Errorf("block %d: %w", i, c.err)
		}
		if fold(crc32.ChecksumIEEE(data)) != crcData {
			warn("block %d data checksum does not match", i)
		}
		z, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", i, err)
		}
		plain := make([]byte, size)
		n, err := io.ReadFull(z, plain)
		_ = z.Close()
		if err != nil {
			if n == 0 && compSize == 0 {
				continue
			}
			warn("block %d inflated to %d of %d bytes", i, n, size)
		}
		if len(out)+n > maxDecompressedSize {
			return nil, errors.New("replay data is too large")
		}
		out = append(out, plain[:n]...)
	}
	return out, nil
}

func fold(c uint32) uint16 { return uint16(c ^ c>>16) }
