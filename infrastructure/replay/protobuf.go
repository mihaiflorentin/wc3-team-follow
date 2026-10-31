package replay

import (
	"encoding/binary"
	"errors"
)

// Reforged stores each player's BattleTag and clan as a small protobuf
// message. These few lines read the wire format (varints and length-delimited
// fields) without a protobuf dependency.

var errProtobuf = errors.New("malformed protobuf message")

// pbField is one decoded field: varint holds wire type 0 values, data holds
// wire type 2 values.
type pbField struct {
	num    int
	wire   int
	varint uint64
	data   []byte
}

// pbFields decodes a message's top-level fields. Fixed-width fields (wire
// types 1 and 5) are skipped; groups are rejected.
func pbFields(b []byte) ([]pbField, error) {
	var out []pbField
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, errProtobuf
		}
		b = b[n:]
		f := pbField{num: int(key >> 3), wire: int(key & 7)}
		switch key & 7 {
		case 0:
			v, n := binary.Uvarint(b)
			if n <= 0 {
				return nil, errProtobuf
			}
			f.varint, b = v, b[n:]
		case 1:
			if len(b) < 8 {
				return nil, errProtobuf
			}
			b = b[8:]
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || l > uint64(len(b)-n) {
				return nil, errProtobuf
			}
			f.data, b = b[n:n+int(l)], b[n+int(l):]
		case 5:
			if len(b) < 4 {
				return nil, errProtobuf
			}
			b = b[4:]
		default:
			return nil, errProtobuf
		}
		out = append(out, f)
	}
	return out, nil
}

// reforgedPlayer is one player message: {1 playerId, 2 battleTag, 3 clan,
// 4 portrait, 5 unknown (10 or 20 seen), 6-8 unknown, empty so far}.
type reforgedPlayer struct {
	id        int
	battleTag string
	clan      string
	portrait  string
}

// decodeReforgedPlayers reads a subtype 0x03 record. Current replays (2.0
// and 3.0) nest every player in it as repeated field 1; w3gjs reads it as
// one flat player message, the layout earlier Reforged replays used. Both
// are accepted.
func decodeReforgedPlayers(b []byte) ([]reforgedPlayer, error) {
	fields, err := pbFields(b)
	if err != nil {
		return nil, err
	}
	nested := false
	for _, f := range fields {
		if f.num == 1 && f.wire == 2 {
			nested = true
		}
	}
	if !nested {
		return []reforgedPlayer{playerFrom(fields)}, nil
	}
	var out []reforgedPlayer
	for _, f := range fields {
		if f.num != 1 || f.wire != 2 {
			continue
		}
		inner, err := pbFields(f.data)
		if err != nil {
			return nil, err
		}
		out = append(out, playerFrom(inner))
	}
	return out, nil
}

func playerFrom(fields []pbField) reforgedPlayer {
	var p reforgedPlayer
	for _, f := range fields {
		switch {
		case f.num == 1 && f.wire == 0:
			p.id = int(f.varint)
		case f.num == 2 && f.wire == 2:
			p.battleTag = string(f.data)
		case f.num == 3 && f.wire == 2:
			p.clan = string(f.data)
		case f.num == 4 && f.wire == 2:
			p.portrait = string(f.data)
		}
	}
	return p
}

// decodeReforgedSkins is subtype 0x04: {1 playerId, 2 repeated skin}. It
// returns the player id and how many skins are listed.
func decodeReforgedSkins(b []byte) (int, int, error) {
	fields, err := pbFields(b)
	if err != nil {
		return 0, 0, err
	}
	id, skins := 0, 0
	for _, f := range fields {
		switch f.num {
		case 1:
			id = int(f.varint)
		case 2:
			skins++
		}
	}
	return id, skins, nil
}
