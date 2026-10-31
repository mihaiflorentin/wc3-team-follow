package dto

import "time"

// ReplayFile is a replay file the game wrote: where it is, when it last
// changed and how big it is. Path holds the Battle.net account folder, so
// it is never logged, shown or saved.
type ReplayFile struct {
	Path     string    `json:"-"`
	Modified time.Time `json:"modified"`
	Size     int64     `json:"size"`
}

// Replay is what a Warcraft III replay file (.w3g) says about one game. It
// holds only what the file records: the lobby (map, slots, players), who
// left when and with which result, chat lines, how many actions each player
// sent and the units, upgrades and research they ordered. Kills, damage, gold
// and the score screen are not in replays: the game works them out again
// from the orders when a replay is watched.
type Replay struct {
	// VersionNumber is the header's game version, such as 10200; Version is
	// it formatted the way the community does ("3.00" for 10200).
	VersionNumber uint32 `json:"versionNumber"`
	Version       string `json:"version"`
	// Build is the game's build number, such as 7003.
	Build uint16 `json:"build"`
	// Expansion is true for The Frozen Throne (every Reforged replay).
	Expansion bool `json:"expansion"`
	// Multiplayer is false for single-player games.
	Multiplayer bool `json:"multiplayer"`
	// Duration is the replay length the header records.
	Duration time.Duration `json:"duration"`
	// GameTime is the sum of the time slots: how long the recorded game ran.
	GameTime time.Duration `json:"gameTime"`
	// Ended is when the match ended by the game clock (action 0x62): the
	// time its last value was first sent, once that value was sent again
	// with a rising repeat counter, as it is after a king dies or an end
	// vote passes. Zero when the clock never stopped in the replay, as
	// when the saver left before the end.
	Ended time.Duration `json:"ended,omitempty"`
	// Post202 is true for replays from game 2.0.2 on, whose Reforged player
	// records start with 0x38 and whose action ids above 0x77 shift by one.
	Post202 bool `json:"post202"`

	GameName string `json:"gameName"`
	// HostID is the player id of the player who created the game.
	HostID int `json:"hostId"`
	// SaverID is the player id of whoever saved the replay (the last to leave).
	SaverID int `json:"saverId"`

	Map      ReplayMap      `json:"map"`
	Settings ReplaySettings `json:"settings"`

	// Players holds every used slot: humans, computers and observers, in
	// slot order.
	Players []ReplayPlayer `json:"players"`
	// Leaves are the leave records in the order they happened.
	Leaves []ReplayLeave `json:"leaves"`
	// Chat holds the chat lines typed during the game.
	Chat []ReplayChat `json:"chat"`
	// Orders are the orders that name an object type (training or building
	// a unit, buying, researching), in the order they were given.
	Orders []ReplayOrder `json:"orders"`
	Winner ReplayWinner  `json:"winner"`

	// RandomSeed, SelectMode and StartSpots come from the slot table.
	RandomSeed uint32 `json:"randomSeed"`
	SelectMode int    `json:"selectMode"`
	StartSpots int    `json:"startSpots"`
	// GameType and LanguageID are raw header fields, as hex.
	GameType   string `json:"gameType"`
	LanguageID string `json:"languageId"`

	// Warnings lists parts of the file that could not be read; the rest of
	// the replay is still filled in.
	Warnings []string `json:"warnings,omitempty"`
}

// ReplayMap is the map the game was played on.
type ReplayMap struct {
	// Path is the map's path as the host's game saw it.
	Path string `json:"path"`
	// File is the file name taken from Path.
	File    string `json:"file"`
	Creator string `json:"creator"`
	// Checksum (4 bytes) and SHA1 identify the map file, as hex; SHA1 is
	// empty in old replays.
	Checksum string `json:"checksum"`
	SHA1     string `json:"sha1,omitempty"`
}

// ReplaySettings are the lobby's game settings.
type ReplaySettings struct {
	// Speed is 0 slow, 1 normal, 2 fast.
	Speed int `json:"speed"`
	// Observers is "none", "on defeat", "full" or "referees".
	Observers     string `json:"observers"`
	HideTerrain   bool   `json:"hideTerrain"`
	MapExplored   bool   `json:"mapExplored"`
	AlwaysVisible bool   `json:"alwaysVisible"`
	TeamsTogether bool   `json:"teamsTogether"`
	FixedTeams    bool   `json:"fixedTeams"`
	SharedControl bool   `json:"sharedControl"`
	RandomHero    bool   `json:"randomHero"`
	RandomRaces   bool   `json:"randomRaces"`
}

// ReplayPlayer is one used slot.
type ReplayPlayer struct {
	// ID is the player id the game data refers to; Slot is the lobby slot.
	ID   int `json:"id"`
	Slot int `json:"slot"`
	// Name is the name in the player record ("Computer" for computers).
	Name string `json:"name"`
	// BattleTag and Clan come from Reforged's player metadata records.
	BattleTag string `json:"battleTag,omitempty"`
	Clan      string `json:"clan,omitempty"`
	// Portrait is the Reforged portrait id, such as "p001".
	Portrait string `json:"portrait,omitempty"`
	// Skins is how many unit skins Reforged recorded for the player.
	Skins int `json:"skins,omitempty"`

	Team  int `json:"team"`
	Color int `json:"color"`
	// Race is "human", "orc", "nightelf", "undead", "random" or "" if
	// unknown; RaceFlag is the raw slot value.
	Race     string `json:"race"`
	RaceFlag int    `json:"raceFlag"`
	Handicap int    `json:"handicap"`
	Computer bool   `json:"computer"`
	// AIStrength is 0 easy, 1 normal, 2 insane, for computers.
	AIStrength int  `json:"aiStrength"`
	Observer   bool `json:"observer"`

	// Actions counts every action the player sent; APMActions only those
	// counted for APM (orders, selections, hotkeys), as w3gjs counts them.
	Actions    int `json:"actions"`
	APMActions int `json:"apmActions"`
	// APM is APMActions per minute up to the player's last action.
	APM        int           `json:"apm"`
	LastAction time.Duration `json:"lastAction"`
	// Left is when the player's leave record came, zero if none.
	Left time.Duration `json:"left"`
	// Result is the player's own leave result ("won", "lost", ...) or "".
	Result string `json:"result"`
}

// ReplayLeave is one leave record.
type ReplayLeave struct {
	Time     time.Duration `json:"time"`
	PlayerID int           `json:"playerId"`
	// Reason is "remote" (0x01), "local" (0x0C, the saver's own game) or the
	// raw value as hex; Result is the decoded result ("left", "lost", "won",
	// "draw", "observer left") or the raw value as hex.
	Reason    string `json:"reason"`
	Result    string `json:"result"`
	RawReason uint32 `json:"rawReason"`
	RawResult uint32 `json:"rawResult"`
	// Counter is the record's last field; it is incremented on the saver's
	// last leave record when the saver won.
	Counter uint32 `json:"counter"`
}

// ReplayChat is one chat line.
type ReplayChat struct {
	Time     time.Duration `json:"time"`
	PlayerID int           `json:"playerId"`
	// Mode is "all", "allies", "observers" or "private" for a chat record,
	// which holds only what the saver could see. It is "unseen" for a line
	// read from the chat action the map's script receives, which every
	// player's game records: another team's allies chat, whose channel the
	// action doesn't say.
	Mode    string `json:"mode"`
	Message string `json:"message"`
}

// ReplayOrder is one order naming an object type: a unit trained or built,
// an item or unit bought, an upgrade or research.
type ReplayOrder struct {
	Time     time.Duration `json:"time"`
	PlayerID int           `json:"playerId"`
	// Code is the object's four-character type id, such as "h04P".
	Code string `json:"code"`
	// Name is the object's name in the map, when the map was read.
	Name string `json:"name,omitempty"`
}

// ReplayWinner is the parser's best judgement of who won.
type ReplayWinner struct {
	// Team is the winning team, -1 when unknown or a draw.
	Team int `json:"team"`
	// Draw is true when a leave record says the game was a draw.
	Draw bool `json:"draw"`
	// Confidence is "certain" (a won or lost result for the team decides
	// it), "likely" (inferred from the saver's last leave record), "guess"
	// (the first team to leave lost) or "unknown".
	Confidence string `json:"confidence"`
	// Basis says in a few words which records decided it.
	Basis string `json:"basis"`
}
