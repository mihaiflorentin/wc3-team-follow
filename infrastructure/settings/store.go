// Package settings saves the friend selection in the per-user data folder
// (config.DataDir).
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// maxFileSize leaves room for a full blacklist (dto.MaxBlacklist).
const maxFileSize = 256 * 1024

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
	JoinHostTeam *bool      `json:"joinHostTeam,omitempty"`
	MapsFolder   string     `json:"mapsFolder,omitempty"`
	Host         *savedHost `json:"host,omitempty"`
	// Blacklist is missing in files saved before it existed.
	Blacklist []dto.BlacklistEntry `json:"blacklist,omitempty"`
	// AutoBan is written only when off: a file without it is on.
	AutoBan *bool `json:"autoBan,omitempty"`
	// BlacklistManualBans is written only when off: a file without it is on.
	BlacklistManualBans *bool `json:"blacklistManualBans,omitempty"`
	// CheckUpdates is written only when off: a file without it is on.
	CheckUpdates   *bool  `json:"checkUpdates,omitempty"`
	SkippedVersion string `json:"skippedVersion,omitempty"`
	// MapUpdates is written only when off: a file without it is on.
	MapUpdates *bool `json:"mapUpdates,omitempty"`
	// LastVersion is missing in files saved before What's new existed.
	LastVersion string `json:"lastVersion,omitempty"`
}

// savedHost is the Auto-host setup as saved. Settings saved before Game
// mode existed get its default delay, and those saved before Use the
// newest installed version existed have it on.
type savedHost dto.HostSettings

func (h *savedHost) UnmarshalJSON(data []byte) error {
	type plain dto.HostSettings
	v := plain{GameModeDelaySeconds: dto.DefaultGameModeDelaySeconds, UseNewest: true}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	*h = savedHost(v)
	return nil
}

// Load returns the saved selection, or every tag empty and the team
// included when nothing has been saved.
func (s *Store) Load() (dto.Preferences, error) {
	info, err := os.Stat(s.file)
	if errors.Is(err, fs.ErrNotExist) {
		return dto.Preferences{Tags: []string{}, IncludeTeam: true, JoinHostTeam: true, Blacklist: []dto.BlacklistEntry{}, AutoBan: true,
			BlacklistManualBans: true, CheckUpdates: true, MapUpdates: true}, nil
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
	if err := json.Unmarshal(content, &saved); err != nil || saved.Tags == nil || saved.IncludeTeam == nil {
		return dto.Preferences{}, fmt.Errorf("%w: unrecognized content", ErrUnreadable)
	}
	blacklist := saved.Blacklist
	if blacklist == nil {
		blacklist = []dto.BlacklistEntry{}
	}
	return dto.Preferences{Tags: saved.Tags, IncludeTeam: *saved.IncludeTeam, AskPassword: saved.AskPassword,
		JoinHostTeam: saved.JoinHostTeam == nil || *saved.JoinHostTeam, MapsFolder: saved.MapsFolder,
		Host: (*dto.HostSettings)(saved.Host), Blacklist: blacklist, AutoBan: saved.AutoBan == nil || *saved.AutoBan,
		BlacklistManualBans: saved.BlacklistManualBans == nil || *saved.BlacklistManualBans,
		CheckUpdates:        saved.CheckUpdates == nil || *saved.CheckUpdates, SkippedVersion: saved.SkippedVersion,
		MapUpdates:  saved.MapUpdates == nil || *saved.MapUpdates,
		LastVersion: saved.LastVersion, Stored: true}, nil
}

// Save replaces the file atomically. Only the selection, the Maps folder,
// the Auto-host setup and the blacklist are written.
func (s *Store) Save(prefs dto.Preferences) error {
	tags := prefs.Tags
	if tags == nil {
		tags = []string{}
	}
	var joinHostTeam *bool
	if !prefs.JoinHostTeam {
		joinHostTeam = &prefs.JoinHostTeam
	}
	var autoBan, manualBans *bool
	if !prefs.AutoBan {
		autoBan = &prefs.AutoBan
	}
	if !prefs.BlacklistManualBans {
		manualBans = &prefs.BlacklistManualBans
	}
	var checkUpdates *bool
	if !prefs.CheckUpdates {
		checkUpdates = &prefs.CheckUpdates
	}
	var mapUpdates *bool
	if !prefs.MapUpdates {
		mapUpdates = &prefs.MapUpdates
	}
	content, err := json.MarshalIndent(fileShape{Tags: tags, IncludeTeam: &prefs.IncludeTeam, AskPassword: prefs.AskPassword, JoinHostTeam: joinHostTeam,
		MapsFolder: prefs.MapsFolder, Host: (*savedHost)(prefs.Host), Blacklist: prefs.Blacklist, AutoBan: autoBan,
		BlacklistManualBans: manualBans, CheckUpdates: checkUpdates, SkippedVersion: prefs.SkippedVersion,
		MapUpdates: mapUpdates, LastVersion: prefs.LastVersion}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode friends: %w", err)
	}
	return writeAtomic(s.file, content)
}

// writeAtomic replaces file with content through a temporary file in the
// same folder, readable only by this user.
func writeAtomic(file string, content []byte) error {
	dir, name := filepath.Dir(file), filepath.Base(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create settings folder: %w", err)
	}
	tmp, err := os.CreateTemp(dir, strings.TrimSuffix(name, filepath.Ext(name))+"-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	// After a successful rename the temporary name no longer exists.
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(append(content, '\n')); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return fmt.Errorf("protect %s: %w", name, err)
	}
	if err := os.Rename(tmp.Name(), file); err != nil {
		return fmt.Errorf("replace %s: %w", name, err)
	}
	return nil
}
