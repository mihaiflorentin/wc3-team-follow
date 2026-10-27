package follow

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func chatEngine(sent *[]string, open bool) *Engine {
	return NewEngine(func(message string, payload map[string]any) bool {
		line, _ := payload["content"].(string)
		*sent = append(*sent, message+"|"+line)
		return open
	}, func() time.Time { return time.Unix(1000, 0) }, nil)
}

func TestSendChatSendsTheTrimmedLine(t *testing.T) {
	var sent []string
	e := chatEngine(&sent, true)
	if err := e.sendChat("  !swap 1 2 \t"); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0] != "SendGameChatMessage|!swap 1 2" {
		t.Fatalf("sent = %q", sent)
	}
}

func TestSendChatAllowsExactlyTheLimit(t *testing.T) {
	var sent []string
	e := chatEngine(&sent, true)
	if err := e.sendChat(strings.Repeat("é", MaxChatLine)); err != nil {
		t.Fatalf("100 characters: %v", err)
	}
	if err := e.sendChat(strings.Repeat("a", MaxChatLine+1)); !errors.Is(err, ErrChatLine) {
		t.Fatalf("101 characters: err = %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("sent = %d lines, want 1", len(sent))
	}
}

func TestSendChatRejectsEmptyAndMultiLineText(t *testing.T) {
	var sent []string
	e := chatEngine(&sent, true)
	for _, text := range []string{"", "   ", "\n", "one\ntwo", "one\r\ntwo", "one\rtwo", "bell\x07"} {
		if err := e.sendChat(text); !errors.Is(err, ErrChatLine) {
			t.Errorf("%q: err = %v", text, err)
		}
	}
	if len(sent) != 0 {
		t.Fatalf("sent = %q", sent)
	}
}

func TestSendChatReportsAClosedConnection(t *testing.T) {
	var sent []string
	e := chatEngine(&sent, false)
	if err := e.sendChat("hi"); err == nil || errors.Is(err, ErrChatLine) {
		t.Fatalf("err = %v", err)
	}
}
