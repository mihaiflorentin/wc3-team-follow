package dto

// GameConnection locates the running game's local menu service. Session is
// private: it is held in memory only and never logged, shown or saved.
type GameConnection struct {
	Port    int
	Session string
	Version string
	PID     int
}

// GameEvent is one message from the game's menu service. Payload keeps the
// game's loose JSON shape; numbers are json.Number so account identifiers
// keep their precision.
type GameEvent struct {
	MessageType string
	Payload     map[string]any
	// Malformed marks a message that could not be read as an event.
	Malformed bool
}

// MapsFolder is a folder of Warcraft III maps found on this computer.
type MapsFolder struct {
	Path string `json:"path"`
	// Source says how it was found, for the player.
	Source string `json:"source"`
}

// KeyboardStatus says whether Team Follow can type in the match chat on
// this computer (ADR 0007).
type KeyboardStatus struct {
	// Supported is false on systems without a keystroke method yet.
	Supported bool `json:"supported"`
	// Permitted is true when the system permission typing needs is granted.
	Permitted bool `json:"permitted"`
	// Reason explains, in a sentence for the player, why typing can't
	// happen; empty when Supported and Permitted.
	Reason string `json:"reason"`
}
