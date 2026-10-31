package replay

import (
	"bytes"
	"encoding/binary"
	"errors"
)

// errShort is returned when a record runs past the end of its data.
var errShort = errors.New("unexpected end of data")

// cursor reads little-endian values from a byte slice. A read past the end
// sets err and returns zero values; callers check err once per record.
type cursor struct {
	b   []byte
	off int
	err error
}

func (c *cursor) left() int { return len(c.b) - c.off }

func (c *cursor) take(n int) []byte {
	if c.err != nil || n < 0 || n > c.left() {
		c.err = errShort
		c.off = len(c.b)
		return nil
	}
	v := c.b[c.off : c.off+n]
	c.off += n
	return v
}

func (c *cursor) skip(n int) { c.take(n) }

func (c *cursor) u8() byte {
	if v := c.take(1); v != nil {
		return v[0]
	}
	return 0
}

func (c *cursor) peek() (byte, bool) {
	if c.err != nil || c.left() < 1 {
		return 0, false
	}
	return c.b[c.off], true
}

func (c *cursor) u16() uint16 {
	if v := c.take(2); v != nil {
		return binary.LittleEndian.Uint16(v)
	}
	return 0
}

func (c *cursor) u32() uint32 {
	if v := c.take(4); v != nil {
		return binary.LittleEndian.Uint32(v)
	}
	return 0
}

// cstr reads a zero-terminated string.
func (c *cursor) cstr() string {
	if c.err != nil {
		return ""
	}
	i := bytes.IndexByte(c.b[c.off:], 0)
	if i < 0 {
		c.err = errShort
		c.off = len(c.b)
		return ""
	}
	s := string(c.b[c.off : c.off+i])
	c.off += i + 1
	return s
}

// cbytes reads a zero-terminated byte string, without the zero.
func (c *cursor) cbytes() []byte {
	if c.err != nil {
		return nil
	}
	i := bytes.IndexByte(c.b[c.off:], 0)
	if i < 0 {
		c.err = errShort
		c.off = len(c.b)
		return nil
	}
	v := c.b[c.off : c.off+i]
	c.off += i + 1
	return v
}
