package dto

import "time"

// FollowConfig changes the companion's selection or switches Follow on or
// off. Nil fields are left unchanged.
type FollowConfig struct {
	// Tags replaces the manually added friend BattleTags; an empty list
	// clears them.
	Tags        *[]string `json:"tags,omitempty"`
	IncludeTeam *bool     `json:"includeTeam,omitempty"`
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
	// Friends is the live presence of each manually added friend.
	Friends          []WatchedMember `json:"friends"`
	LastJoin         *JoinRecord     `json:"lastJoin"`
	Log              []LogEntry      `json:"log"`
	PreferencesError string          `json:"preferencesError"`
	// ConnectionHint explains why the game is not connected yet, when
	// discovery knows.
	ConnectionHint string `json:"connectionHint"`
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
	AppVersion   string         `json:"appVersion"`
}

// Preferences is what is saved on disk between runs: only the friend
// BattleTags and whether to include the current team.
type Preferences struct {
	Tags        []string `json:"tags"`
	IncludeTeam bool     `json:"includeTeam"`
}
