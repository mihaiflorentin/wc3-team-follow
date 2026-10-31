package replay

// Action lengths follow w3gjs's ActionParser (MIT), for game 1.13 and later.

// actionStats counts one player's actions.
type actionStats struct {
	all, apm   int
	lastDeselt bool
}

// readActions decodes one command block's actions, counting them. It stops
// at the first unknown action, whose length can't be known, and returns its
// id with ok false.
func readActions(b []byte, post202 bool, s *actionStats) (unknown byte, ok bool) {
	c := cursor{b: b}
	s.lastDeselt = false
	for c.left() > 0 {
		raw := c.u8()
		id := int(raw)
		if post202 && id >= 0x77 {
			// From 2.0.2 on ids from 0x77 are one lower than in older
			// replays: raw 0x77 has the old 0x78 layout (a map sync such as
			// "SyncCode"), seen in real 2.0.2+ replays. w3gjs shifts only
			// ids above 0x77.
			id++
		}
		counted, known := readAction(&c, id, s)
		if !known {
			return raw, false
		}
		if c.err != nil {
			return raw, false
		}
		s.all++
		if counted {
			s.apm++
		}
	}
	return 0, true
}

// readAction skips one action's body and says whether it counts for APM.
func readAction(c *cursor, id int, s *actionStats) (apm, known bool) {
	switch id {
	case 0x01, 0x03:
		c.skip(1)
	case 0x02, 0x04, 0x05:
	case 0x06:
		c.cstr()
		c.cstr()
		c.skip(1)
	case 0x07:
		c.skip(4)
	case 0x10: // order without target
		c.skip(2 + 4 + 8)
		return true, true
	case 0x11: // order with target point
		c.skip(2 + 4 + 8 + 8)
		return true, true
	case 0x12: // order with target point and object
		c.skip(2 + 4 + 8 + 8 + 8)
		return true, true
	case 0x13: // give item to unit
		c.skip(2 + 4 + 8 + 8 + 8 + 8)
		return true, true
	case 0x14: // order with two targets
		c.skip(2 + 4 + 8 + 8 + 4 + 4 + 4 + 1 + 8)
		return true, true
	case 0x15:
		c.skip(2 + 4 + 8 + 8 + 4 + 4 + 4 + 1 + 8 + 8)
	case 0x16: // change selection; a deselect followed by a select is one action
		mode := c.u8()
		c.skip(int(c.u16()) * 8)
		if mode == 0x02 {
			s.lastDeselt = true
			return true, true
		}
		counted := !s.lastDeselt
		s.lastDeselt = false
		return counted, true
	case 0x17: // assign group hotkey
		c.skip(1)
		c.skip(int(c.u16()) * 8)
		return true, true
	case 0x18: // select group hotkey
		c.skip(2)
		return true, true
	case 0x19:
		c.skip(4 + 8)
	case 0x1A:
	case 0x1B:
		c.skip(1 + 8)
	case 0x1C: // select ground item
		c.skip(1 + 8)
		return true, true
	case 0x1D: // cancel hero revival
		c.skip(8)
		return true, true
	case 0x1E: // remove unit from queue
		c.skip(1 + 4)
		return true, true
	case 0x1F:
		c.skip(1 + 4)
	case 0x20, 0x22, 0x23, 0x24, 0x25, 0x26, 0x29, 0x2A, 0x2B, 0x2C, 0x2F:
	case 0x21:
		c.skip(8)
	case 0x27, 0x28, 0x2D:
		c.skip(5)
	case 0x2E:
		c.skip(4)
	case 0x50:
		c.skip(1 + 4)
	case 0x51: // transfer resources
		c.skip(1 + 4 + 4)
	case 0x60:
		c.skip(8)
		c.cstr()
	case 0x61: // Esc
		return true, true
	case 0x62:
		c.skip(12)
	case 0x63, 0x64, 0x65:
		c.skip(8)
	case 0x66, 0x67: // skill and build submenus
		return true, true
	case 0x68: // minimap ping
		c.skip(12)
	case 0x69, 0x6A:
		c.skip(16)
	case 0x6B, 0x6C:
		skipCache(c)
		c.skip(4)
	case 0x6D:
		skipCache(c)
		c.skip(1)
	case 0x6E:
		skipCache(c)
		skipCacheUnit(c)
	case 0x70, 0x71, 0x72, 0x73:
		skipCache(c)
	case 0x75:
		c.skip(1)
	case 0x76:
		c.skip(1 + 8 + 1)
	case 0x77:
		c.skip(4 + 4)
		c.skip(int(c.u32()))
	case 0x78:
		c.cstr()
		c.cstr()
		c.skip(4)
	case 0x79:
		c.skip(8 + 4 + 4)
		c.cstr()
	case 0x7A:
		c.skip(20)
	case 0x7B:
		c.skip(16)
	case 0xA0:
		c.skip(14)
	case 0xA1:
		c.skip(9)
	default:
		return false, false
	}
	return false, true
}

func skipCache(c *cursor) {
	c.cstr() // file
	c.cstr() // mission key
	c.cstr() // key
}

func skipCacheUnit(c *cursor) {
	c.skip(4)                      // unit id
	c.skip(int(c.u32()) * 12)      // items
	c.skip(12 * 4)                 // hero xp, level, attributes
	c.skip(int(c.u32()) * 8)       // abilities
	c.skip(3 * 4)                  // life, mana, sight
	c.skip(int(c.u32())*4 + 4 + 2) // damage, defense, control groups
}
