package dto

import "time"

// MaxNoteLength is how many characters a player note may hold.
const MaxNoteLength = 500

// PlayerNote is the player's private note about another player, kept on
// this computer only. Notes are keyed by the lower-cased BattleTag.
type PlayerNote struct {
	// Text is the note, trimmed, at most MaxNoteLength characters.
	Text string `json:"text"`
	// Updated is when the note was last saved.
	Updated time.Time `json:"updated"`
}
