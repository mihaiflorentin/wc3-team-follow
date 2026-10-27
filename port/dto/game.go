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
