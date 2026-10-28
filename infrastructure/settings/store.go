// Package settings saves the friend selection in the per-user data folder
// (config.DataDir).
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const maxFileSize = 16 * 1024

// ErrUnreadable reports a saved file that exists but cannot be used. The
// file is left untouched.
var ErrUnreadable = errors.New("saved friends could not be loaded")

// Store keeps the selection in <dir>/settings/friends.json.
type Store struct {
	file string
}

// New returns a store that keeps its file under dir.
func New(dir string) contract.PreferenceStore {
	return &Store{file: filepath.Join(dir, "settings", "friends.json")}
}

type fileShape struct {
	Tags        []string `json:"tags"`
	IncludeTeam *bool    `json:"includeTeam"`
	AskPassword bool     `json:"askPassword,omitempty"`
	// JoinHostTeam is written only when off: a file without it is on.
	JoinHostTeam *bool             `json:"joinHostTeam,omitempty"`
	MapsFolder   string            `json:"mapsFolder,omitempty"`
	Host         *dto.HostSettings `json:"host,omitempty"`
}

// Load returns the saved selection, or every tag empty and the team
// included when nothing has been saved.
func (s *Store) Load() (dto.Preferences, error) {
	info, err := os.Stat(s.file)
	if errors.Is(err, fs.ErrNotExist) {
		return dto.Preferences{Tags: []string{}, IncludeTeam: true, JoinHostTeam: true}, nil
	}
	if err != nil {
		return dto.Preferences{}, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	if info.Size() > maxFileSize {
		return dto.Preferences{}, fmt.Errorf("%w: file is too large", ErrUnreadable)
	}
	content, err := os.ReadFile(s.file)
	if err != nil {
		return dto.Preferences{}, fmt.Errorf("%w: %v", ErrUnreadable, err)
	}
	var saved fileShape
	decoder := json.NewDecoder(bytes.NewReader(content))
	if err := decoder.Decode(&saved); err != nil || saved.Tags == nil || saved.IncludeTeam == nil {
		return dto.Preferences{}, fmt.Errorf("%w: unrecognized content", ErrUnreadable)
	}
	return dto.Preferences{Tags: saved.Tags, IncludeTeam: *saved.IncludeTeam, AskPassword: saved.AskPassword,
		JoinHostTeam: saved.JoinHostTeam == nil || *saved.JoinHostTeam, MapsFolder: saved.MapsFolder, Host: saved.Host}, nil
}

// Save replaces the file atomically. Only the selection, the Maps folder and
// the Auto-host setup are written.
func (s *Store) Save(prefs dto.Preferences) error {
	tags := prefs.Tags
	if tags == nil {
		tags = []string{}
	}
	var joinHostTeam *bool
	if !prefs.JoinHostTeam {
		joinHostTeam = &prefs.JoinHostTeam
	}
	content, err := json.MarshalIndent(fileShape{Tags: tags, IncludeTeam: &prefs.IncludeTeam, AskPassword: prefs.AskPassword, JoinHostTeam: joinHostTeam,
		MapsFolder: prefs.MapsFolder, Host: prefs.Host}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode friends: %w", err)
	}
	dir := filepath.Dir(s.file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create settings folder: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "friends-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	// After a successful rename the temporary name no longer exists.
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(content, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write friends: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write friends: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return fmt.Errorf("protect friends file: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.file); err != nil {
		return fmt.Errorf("replace friends file: %w", err)
	}
	return nil
}
