package mapdata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"regexp"
	"strconv"
)

// objectFile is one object table in a map and the field holding a name.
type objectFile struct {
	name string
	// levels is true for tables whose fields carry a level and a data
	// pointer (abilities, upgrades, doodads).
	levels bool
	field  string
}

// objectFiles are read in order; the first name found for an id wins. Since
// Reforged 1.33 names and art sit in the war3mapSkin tables.
var objectFiles = []objectFile{
	{"war3mapSkin.w3u", false, "unam"}, {"war3map.w3u", false, "unam"},
	{"war3mapSkin.w3t", false, "unam"}, {"war3map.w3t", false, "unam"},
	{"war3mapSkin.w3q", true, "gnam"}, {"war3map.w3q", true, "gnam"},
	{"war3mapSkin.w3a", true, "anam"}, {"war3map.w3a", true, "anam"},
}

var errObjects = errors.New("object table is cut short")

// objectNames reads one object table's names into names, keeping names
// already set. trig resolves TRIGSTR_ references.
func objectNames(data []byte, levels bool, field string, trig map[int]string, names map[string]string) error {
	r := bytes.NewReader(data)
	i32 := func() (int32, error) {
		var v int32
		err := binary.Read(r, binary.LittleEndian, &v)
		return v, err
	}
	id := func() (string, error) {
		var b [4]byte
		if _, err := r.Read(b[:]); err != nil {
			return "", err
		}
		return string(b[:]), nil
	}
	cstr := func() (string, error) {
		var s []byte
		for {
			c, err := r.ReadByte()
			if err != nil {
				return "", err
			}
			if c == 0 {
				return string(s), nil
			}
			s = append(s, c)
		}
	}
	version, err := i32()
	if err != nil {
		return errObjects
	}
	for custom := range 2 {
		count, err := i32()
		if err != nil || count < 0 {
			return errObjects
		}
		for range count {
			orig, err1 := id()
			created, err2 := id()
			if err1 != nil || err2 != nil {
				return errObjects
			}
			key := orig
			if custom == 1 {
				key = created
			}
			sets := int32(1)
			if version >= 3 {
				if sets, err = i32(); err != nil {
					return errObjects
				}
			}
			for range sets {
				if version >= 3 {
					if _, err := i32(); err != nil { // set flags
						return errObjects
					}
				}
				mods, err := i32()
				if err != nil {
					return errObjects
				}
				for range mods {
					mod, err := id()
					if err != nil {
						return errObjects
					}
					kind, err := i32()
					if err != nil {
						return errObjects
					}
					if levels {
						if _, err := i32(); err != nil {
							return errObjects
						}
						if _, err := i32(); err != nil {
							return errObjects
						}
					}
					var value string
					switch kind {
					case 0, 1, 2: // int, real, unreal
						_, err = i32()
					default:
						value, err = cstr()
					}
					if err != nil {
						return errObjects
					}
					if _, err := id(); err != nil { // end of field
						return errObjects
					}
					if mod == field && value != "" && names[key] == "" {
						names[key] = cleanName(resolve(value, trig))
					}
				}
			}
		}
	}
	return nil
}

var (
	trigRef   = regexp.MustCompile(`^TRIGSTR_0*(\d+)$`)
	wtsString = regexp.MustCompile(`(?s)STRING (\d+)[^{]*\{\r?\n(.*?)\r?\n\}`)
	colours   = regexp.MustCompile(`\|c[0-9a-fA-F]{8}|\|r`)
)

// mapStrings reads war3map.wts: the map's strings by number.
func mapStrings(wts []byte) map[int]string {
	out := map[int]string{}
	for _, m := range wtsString.FindAllSubmatch(wts, -1) {
		if n, err := strconv.Atoi(string(m[1])); err == nil {
			out[n] = string(m[2])
		}
	}
	return out
}

func resolve(value string, trig map[int]string) string {
	if m := trigRef.FindStringSubmatch(value); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return trig[n]
		}
	}
	return value
}

// cleanName drops the game's colour codes, such as |cffff0000 and |r.
func cleanName(s string) string { return colours.ReplaceAllString(s, "") }
