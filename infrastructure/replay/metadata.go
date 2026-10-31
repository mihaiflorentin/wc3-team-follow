package replay

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Record ids in the game info part of the decompressed data.
const (
	recPlayer           = 0x16
	recSlots            = 0x19
	recReforgedPost202  = 0x38
	recReforgedPre202   = 0x39
	reforgedSubPlayer   = 0x03
	reforgedSubSkins    = 0x04
	slotStatusUsed      = 2
	observerTeam        = 24 // from game 1.29 on
	observerTeamLegacy  = 12
	observerTeamVersion = 29
)

type playerRecord struct {
	id   int
	name string
}

type slotRecord struct {
	playerID, status, computer, team, color, race, ai, handicap int
}

// metadata is everything before the game data records.
type metadata struct {
	host     playerRecord
	players  []playerRecord
	reforged map[int]reforgedPlayer
	skins    map[int]int
	slots    []slotRecord
	post202  bool
}

// readMetadata reads the game info, player records, Reforged player
// metadata and slot table, filling r and returning the cursor at the first
// game data record.
func readMetadata(c *cursor, r *dto.Replay, warn func(string, ...any)) (metadata, error) {
	m := metadata{reforged: map[int]reforgedPlayer{}, skins: map[int]int{}}
	c.skip(4) // unknown, 0x00000110
	if id := c.u8(); id != 0x00 && c.err == nil {
		return m, fmt.Errorf("host record id is 0x%02X, want 0x00", id)
	}
	m.host = readPlayerRecord(c)
	r.HostID = m.host.id
	r.GameName = c.cstr()
	c.cstr() // private string, always empty
	readMapSettings(decodeMeta(c.cbytes()), r)
	c.u32() // player count (slots)
	r.GameType = hex.EncodeToString(c.take(4))
	r.LanguageID = hex.EncodeToString(c.take(4))
	if c.err != nil {
		return m, fmt.Errorf("game info: %w", c.err)
	}
	m.players = append(m.players, m.host)

	for {
		id, ok := c.peek()
		if !ok {
			return m, fmt.Errorf("player list: %w", errShort)
		}
		switch id {
		case recPlayer:
			c.skip(1)
			m.players = append(m.players, readPlayerRecord(c))
			c.skip(4)
		case recReforgedPost202, recReforgedPre202:
			c.skip(1)
			if id == recReforgedPost202 {
				m.post202 = true
			}
			sub := c.u8()
			body := c.take(int(c.u32()))
			if c.err != nil {
				return m, fmt.Errorf("reforged record: %w", c.err)
			}
			readReforged(sub, body, &m, warn)
		case recSlots:
			c.skip(1)
			if err := readSlots(c, &m, r); err != nil {
				return m, err
			}
			return m, nil
		default:
			return m, fmt.Errorf("unknown record 0x%02X before the slot table", id)
		}
		if c.err != nil {
			return m, fmt.Errorf("player list: %w", c.err)
		}
	}
}

func readPlayerRecord(c *cursor) playerRecord {
	p := playerRecord{id: int(c.u8()), name: c.cstr()}
	c.skip(int(c.u8())) // extra data: 1 byte in custom games, 8 in ladder
	return p
}

func readReforged(sub byte, body []byte, m *metadata, warn func(string, ...any)) {
	switch sub {
	case reforgedSubPlayer:
		players, err := decodeReforgedPlayers(body)
		if err != nil {
			warn("reforged player record: %v", err)
			return
		}
		for _, p := range players {
			m.reforged[p.id] = p
		}
	case reforgedSubSkins:
		id, n, err := decodeReforgedSkins(body)
		if err != nil {
			warn("reforged skin record: %v", err)
			return
		}
		m.skins[id] = n
	}
}

func readSlots(c *cursor, m *metadata, r *dto.Replay) error {
	size := int(c.u16())
	count := int(c.u8())
	if c.err != nil || count == 0 {
		return fmt.Errorf("slot table is empty or cut short")
	}
	// size = count byte + slots + seed (4) + select mode (1) + start spots (1).
	// Slots grew from 7 to 9 bytes over the years; derive it from the size.
	slotLen := (size - 7) / count
	if slotLen < 7 || (size-7)%count != 0 {
		return fmt.Errorf("slot table size %d does not fit %d slots", size, count)
	}
	for range count {
		s := c.take(slotLen)
		if c.err != nil {
			return fmt.Errorf("slot table: %w", c.err)
		}
		rec := slotRecord{playerID: int(s[0]), status: int(s[2]), computer: int(s[3]), team: int(s[4]), color: int(s[5]), race: int(s[6]), handicap: 100}
		if slotLen >= 8 {
			rec.ai = int(s[7])
		}
		if slotLen >= 9 {
			rec.handicap = int(s[8])
		}
		m.slots = append(m.slots, rec)
	}
	r.RandomSeed = c.u32()
	r.SelectMode = int(c.u8())
	r.StartSpots = int(c.u8())
	if c.err != nil {
		return fmt.Errorf("slot table: %w", c.err)
	}
	return nil
}

// decodeMeta undoes the map settings string's encoding: a mask byte before
// every seven bytes says which were stored as they are; the others were
// stored plus one so that the string holds no zero.
func decodeMeta(enc []byte) []byte {
	out := make([]byte, 0, len(enc))
	var mask byte
	for i, b := range enc {
		if i%8 == 0 {
			mask = b
			continue
		}
		if mask&(1<<(i%8)) == 0 {
			b--
		}
		out = append(out, b)
	}
	return out
}

func readMapSettings(meta []byte, r *dto.Replay) {
	c := cursor{b: meta}
	speed := c.u8()
	b2, b3, b4 := c.u8(), c.u8(), c.u8()
	c.skip(5)
	sum := c.take(4)
	path := c.cstr()
	creator := c.cstr()
	c.skip(1)
	sha := c.take(20)

	s := &r.Settings
	s.Speed = int(speed)
	s.HideTerrain = b2&0x01 != 0
	s.MapExplored = b2&0x02 != 0
	s.AlwaysVisible = b2&0x04 != 0
	s.TeamsTogether = b2&0x40 != 0
	s.FixedTeams = b3&0x06 != 0
	s.SharedControl = b4&0x01 != 0
	s.RandomHero = b4&0x02 != 0
	s.RandomRaces = b4&0x04 != 0
	referees := b4&0x40 != 0
	switch mode := (b2 >> 4) & 0x03; {
	case referees && (mode == 3 || mode == 0):
		s.Observers = "referees"
	case mode == 2:
		s.Observers = "on defeat"
	case mode == 3:
		s.Observers = "full"
	default:
		s.Observers = "none"
	}
	r.Map = dto.ReplayMap{Path: path, File: mapFile(path), Creator: creator, Checksum: hex.EncodeToString(sum)}
	if sha != nil {
		r.Map.SHA1 = hex.EncodeToString(sha)
	}
}

func mapFile(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}

func raceName(flag int) string {
	switch flag & 0x3F {
	case 0x01:
		return "human"
	case 0x02:
		return "orc"
	case 0x04:
		return "nightelf"
	case 0x08:
		return "undead"
	case 0x20:
		return "random"
	}
	return ""
}

// buildPlayers joins slots with player records and Reforged metadata.
func buildPlayers(m metadata, version uint32) []dto.ReplayPlayer {
	names := map[int]string{}
	for _, p := range m.players {
		names[p.id] = p.name
	}
	obsTeam := observerTeam
	if version < observerTeamVersion {
		obsTeam = observerTeamLegacy
	}
	var out []dto.ReplayPlayer
	for i, s := range m.slots {
		if s.status != slotStatusUsed {
			continue
		}
		p := dto.ReplayPlayer{
			ID: s.playerID, Slot: i, Team: s.team, Color: s.color,
			Race: raceName(s.race), RaceFlag: s.race, Handicap: s.handicap,
			Computer: s.computer != 0, Observer: s.team == obsTeam,
		}
		if p.Computer {
			p.Name = "Computer"
			p.AIStrength = s.ai
		} else {
			p.Name = names[s.playerID]
			if rp, ok := m.reforged[s.playerID]; ok {
				p.BattleTag, p.Clan, p.Portrait = rp.battleTag, rp.clan, rp.portrait
			}
			p.Skins = m.skins[s.playerID]
		}
		out = append(out, p)
	}
	return out
}
