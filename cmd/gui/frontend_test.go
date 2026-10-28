package gui_test

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// The page's Game mode delay field must use the engine's limit; app.js
// reads it from the field's max.
func TestGameModeDelayFieldUsesTheSameLimit(t *testing.T) {
	page, err := os.ReadFile("frontend/index.html")
	if err != nil {
		t.Fatal(err)
	}
	field := regexp.MustCompile(`<input id="game-mode-delay"[^>]*\bmax="(\d+)"`).FindSubmatch(page)
	if field == nil {
		t.Fatal("no game-mode-delay field with a max")
	}
	if got, want := string(field[1]), strconv.Itoa(dto.MaxGameModeDelaySeconds); got != want {
		t.Fatalf("game-mode-delay max = %s, want %s", got, want)
	}
}

// The note box must use the Go limit; app.js reads it from data-max.
func TestNoteFieldUsesTheSameLimit(t *testing.T) {
	page, err := os.ReadFile("frontend/index.html")
	if err != nil {
		t.Fatal(err)
	}
	field := regexp.MustCompile(`<textarea id="note-text"[^>]*\bdata-max="(\d+)"`).FindSubmatch(page)
	if field == nil {
		t.Fatal("no note-text field with a data-max")
	}
	if got, want := string(field[1]), strconv.Itoa(dto.MaxNoteLength); got != want {
		t.Fatalf("note-text data-max = %s, want %s", got, want)
	}
}
