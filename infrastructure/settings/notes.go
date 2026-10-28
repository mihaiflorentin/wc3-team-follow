package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// maxNotesFileSize is far above any likely number of notes; a larger file
// is not ours.
const maxNotesFileSize = 4 * 1024 * 1024

// PlayerNotes keeps the player notes in <dir>/settings/notes.json, next to
// friends.json.
type PlayerNotes struct {
	file string
}

// NewPlayerNotes returns a store that keeps its file under dir.
func NewPlayerNotes(dir string) contract.PlayerNotes {
	return &PlayerNotes{file: filepath.Join(dir, "settings", "notes.json")}
}

type notesFile struct {
	Notes map[string]dto.PlayerNote `json:"notes"`
}

// Load returns the saved notes. A missing, unreadable or unrecognized file
// loads as no notes: the next save replaces it. Keys are lower-cased, text
// trimmed and cut to dto.MaxNoteLength characters, and empty notes dropped.
func (s *PlayerNotes) Load() (map[string]dto.PlayerNote, error) {
	notes := map[string]dto.PlayerNote{}
	info, err := os.Stat(s.file)
	if err != nil || info.Size() > maxNotesFileSize {
		return notes, nil
	}
	content, err := os.ReadFile(s.file)
	if err != nil {
		return notes, nil
	}
	var saved notesFile
	if err := json.Unmarshal(content, &saved); err != nil {
		return notes, nil
	}
	for tag, note := range saved.Notes {
		key := strings.ToLower(strings.TrimSpace(tag))
		text := strings.TrimSpace(note.Text)
		if key == "" || text == "" {
			continue
		}
		if r := []rune(text); len(r) > dto.MaxNoteLength {
			text = strings.TrimSpace(string(r[:dto.MaxNoteLength]))
		}
		notes[key] = dto.PlayerNote{Text: text, Updated: note.Updated}
	}
	return notes, nil
}

// Save replaces the file atomically.
func (s *PlayerNotes) Save(notes map[string]dto.PlayerNote) error {
	if notes == nil {
		notes = map[string]dto.PlayerNote{}
	}
	content, err := json.MarshalIndent(notesFile{Notes: notes}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode player notes: %w", err)
	}
	return writeAtomic(s.file, content)
}
