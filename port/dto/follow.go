package dto

import "time"

// FollowConfig changes the companion's selection or switches Follow on or
// off. Nil fields are left unchanged.
type FollowConfig struct {
	// Tags replaces the manually added friend BattleTags; an empty list
	// clears them.
	Tags        *[]string `json:"tags,omitempty"`
	IncludeTeam *bool     `json:"includeTeam,omitempty"`
	// AskPassword lets a lobby that wants a password ask the player for it;
	// off, such a lobby is skipped.
	AskPassword *bool `json:"askPassword,omitempty"`
	// Diagnostic selects preview mode: watch friends but never join.
	Diagnostic *bool `json:"diagnostic,omitempty"`
	Enabled    *bool `json:"enabled,omitempty"`
	// FromMenus confirms the player is in the game menus, outside a lobby,
	// match or queue. Enabling Follow requires it.
	FromMenus bool `json:"fromMenus,omitempty"`
}

// FollowState is a snapshot of the companion for the interface.
type FollowState struct {
	Connected bool   `json:"connected"`
	Version   string `json:"version"`
	// VerifiedBuild reports whether live joining is allowed on Version.
	VerifiedBuild bool     `json:"verifiedBuild"`
	Enabled       bool     `json:"enabled"`
	Diagnostic    bool     `json:"diagnostic"`
	ManualTags    []string `json:"manualTags"`
	IncludeTeam   bool     `json:"includeTeam"`
	AskPassword   bool     `json:"askPassword"`
	Status        string   `json:"status"`
	Screen        string   `json:"screen"`
	Busy          bool     `json:"busy"`
	Queued        bool     `json:"queued"`
	InMatch       bool     `json:"inMatch"`
	Paused        bool     `json:"paused"`
	// InMenus reports that the game itself said, since connecting, that the
	// player is in a menu screen outside a lobby, match or queue.
	InMenus         bool             `json:"inMenus"`
	JoinPending     bool             `json:"joinPending"`
	PasswordRequest *PasswordRequest `json:"passwordRequest"`
	Team            []TeamMember     `json:"team"`
	Watched         []WatchedMember  `json:"watched"`
	// Friends is the live presence of each friend in the follow group.
	Friends []WatchedMember `json:"friends"`
	// Roster is the player's whole Battle.net friends list, to choose from.
	Roster           []Friend    `json:"roster"`
	LastJoin         *JoinRecord `json:"lastJoin"`
	Log              []LogEntry  `json:"log"`
	PreferencesError string      `json:"preferencesError"`
	// ConnectionHint explains why the game is not connected yet, when
	// discovery knows.
	ConnectionHint string `json:"connectionHint"`
	// GameCount is how many copies of Warcraft III discovery found while
	// not connected, when it found more than one; 0 otherwise.
	GameCount int `json:"gameCount"`
	// Host is Auto-host's state.
	Host HostState `json:"host"`
	// MapsFolder is the folder the map browser opens.
	MapsFolder string `json:"mapsFolder"`
}

// TeamMember is one player of the game's current team.
type TeamMember struct {
	BattleTag string `json:"battleTag"`
	IsSelf    bool   `json:"isSelf"`
	IsFriend  bool   `json:"isFriend"`
	IsOwner   bool   `json:"isOwner"`
	Status    string `json:"status"`
}

// WatchedMember is one player Follow remembered when it was switched on.
type WatchedMember struct {
	BattleTag string `json:"battleTag"`
	Label     string `json:"label"`
	Open      bool   `json:"open"`
	Lobby     string `json:"lobby"`
}

// Friend is one Battle.net friend, for choosing who to follow.
type Friend struct {
	BattleTag string `json:"battleTag"`
	Label     string `json:"label"`
	Lobby     string `json:"lobby"`
	Online    bool   `json:"online"`
	Away      bool   `json:"away"`
	// InGame is true while the friend is in Warcraft III.
	InGame bool `json:"inGame"`
	// Open is true while the friend's lobby can be joined.
	Open bool `json:"open"`
	// Selected is true when the friend is in the follow group.
	Selected bool `json:"selected"`
}

// PasswordRequest names the lobby the game asked a password for.
type PasswordRequest struct {
	Lobby  string `json:"lobby"`
	Friend string `json:"friend"`
}

// JoinRecord describes the latest join attempt.
type JoinRecord struct {
	Friend    string    `json:"friend"`
	Lobby     string    `json:"lobby"`
	Time      time.Time `json:"time"`
	Confirmed bool      `json:"confirmed"`
}

// LogEntry is one line of recent activity.
type LogEntry struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
}

// Diagnostics is the shareable report. It never contains BattleTags,
// account identifiers, lobby names or session details.
type Diagnostics struct {
	Version      string         `json:"version"`
	Connected    bool           `json:"connected"`
	Screen       string         `json:"screen"`
	TeamCount    int            `json:"teamCount"`
	WatchedCount int            `json:"watchedCount"`
	FriendCount  int            `json:"friendCount"`
	EventCounts  map[string]int `json:"eventCounts"`
	Diagnostic   bool           `json:"diagnostic"`
	Enabled      bool           `json:"enabled"`
	Busy         bool           `json:"busy"`
	Queued       bool           `json:"queued"`
	Platform     string         `json:"platform"`
	// LogFolder is where the log files are, for attaching to bug reports.
	LogFolder  string `json:"logFolder,omitempty"`
	AppVersion string `json:"appVersion"`
}

// Preferences is what is saved on disk between runs: the friend BattleTags,
// whether to include the current team and to ask for lobby passwords.
type Preferences struct {
	Tags        []string `json:"tags"`
	IncludeTeam bool     `json:"includeTeam"`
	AskPassword bool     `json:"askPassword,omitempty"`
	// MapsFolder is the Maps folder the player chose; "" detects it.
	MapsFolder string `json:"mapsFolder,omitempty"`
	// Host is the saved Auto-host setup.
	Host *HostSettings `json:"host,omitempty"`
}

// HostSettings is how Auto-host creates lobbies. Everything else uses the
// game's own defaults.
type HostSettings struct {
	// MapPath is the map file on this computer.
	MapPath string `json:"mapPath"`
	// MapName is shown in the window and status lines.
	MapName  string `json:"mapName"`
	GameName string `json:"gameName"`
	// Private hides the lobby from the Custom Games list (the game's
	// [PUBLIC]/[PRIVATE] toggle).
	Private bool `json:"private"`
	// Visibility is the map visibility: 0 default, 1 hide terrain, 2 map
	// explored, 4 always visible (the game's VisibilityEnum).
	Visibility int `json:"visibility"`
	// Observers is who may watch: 0 none, 32 observers on defeat, 4194304
	// referees, 16 full observers (the game's ObserverEnum).
	Observers int `json:"observers"`
	// DelaySeconds is how long to wait before creating a lobby: after
	// Auto-host is turned on, and after each match.
	DelaySeconds int `json:"delaySeconds"`
	// StartWhenFull starts the game once every playable slot is taken and
	// everyone in the follow group is in the lobby.
	StartWhenFull bool `json:"startWhenFull"`
	// SameTeam moves everyone in the follow group onto the host's team.
	SameTeam bool `json:"sameTeam"`
	// InviteGroup invites everyone in the follow group who is in
	// Warcraft III but not in the lobby, for friends without Team Follow.
	InviteGroup bool `json:"inviteGroup"`
}

// HostConfig changes Auto-host. Nil fields are left unchanged.
type HostConfig struct {
	Settings *HostSettings `json:"settings,omitempty"`
	Enabled  *bool         `json:"enabled,omitempty"`
	// FromMenus confirms the player is in the game's menus.
	FromMenus bool `json:"fromMenus,omitempty"`
	// GameMapPath is MapPath as the game sees it (a Windows path under
	// Wine). The companion fills it in.
	GameMapPath string `json:"-"`
}

// HostState is Auto-host's part of the snapshot.
type HostState struct {
	Enabled  bool         `json:"enabled"`
	Settings HostSettings `json:"settings"`
	// Phase is off, waiting, creating, lobby or match.
	Phase string `json:"phase"`
	// CreateAt is when the next lobby will be created, while waiting.
	CreateAt *time.Time `json:"createAt,omitempty"`
	// Players and Slots describe the open lobby.
	Players int  `json:"players"`
	Slots   int  `json:"slots"`
	Full    bool `json:"full"`
	// Missing lists group members not in the lobby yet.
	Missing []string `json:"missing"`
	// OffTeam lists group members not on the host's team, with SameTeam.
	OffTeam []string `json:"offTeam"`
}

// MapEntry is a map file or a folder in the map browser.
type MapEntry struct {
	Name     string `json:"name"`
	IsFolder bool   `json:"isFolder"`
	// Path is the full path on this computer.
	Path string `json:"path"`
	// Dir is the containing folder relative to the Maps folder ("" for the
	// top), for search results and breadcrumbs.
	Dir      string    `json:"dir"`
	Modified time.Time `json:"modified"`
	Size     int64     `json:"size"`
}

// MapListing is one folder's contents, or search results.
type MapListing struct {
	Root    string     `json:"root"`
	Dir     string     `json:"dir"`
	Query   string     `json:"query,omitempty"`
	Entries []MapEntry `json:"entries"`
	// Truncated reports that a search stopped at its result limit.
	Truncated bool `json:"truncated"`
}
