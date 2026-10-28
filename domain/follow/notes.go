package follow

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// NoteKey is the key a player's note is kept under: the lower-cased,
// trimmed BattleTag.
func NoteKey(tag string) string { return tagKey(tag) }

// NoteText checks a note the player wrote and returns it trimmed. Empty
// text means the note is deleted. Notes hold at most dto.MaxNoteLength
// characters; a longer one is refused whole, never cut short.
func NoteText(tag, text string) (string, error) {
	if NoteKey(tag) == "" {
		return "", errors.New("Choose a player to write a note about.")
	}
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) > dto.MaxNoteLength {
		return "", fmt.Errorf("A note can be at most %d characters.", dto.MaxNoteLength)
	}
	return text, nil
}
