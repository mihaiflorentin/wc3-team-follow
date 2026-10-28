package contract

import (
	"context"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// Companion is the inbound port the desktop interface drives: it watches
// the game, keeps the follow selection and joins friends' lobbies.
type Companion interface {
	// Run connects to the game and keeps the companion running until ctx
	// is cancelled.
	Run(ctx context.Context) error
	State() dto.FollowState
	Configure(cfg dto.FollowConfig) (dto.FollowState, error)
	// SubmitPassword answers the game's lobby password request once. The
	// password is never stored.
	SubmitPassword(password string) (dto.FollowState, error)
	Diagnostics() dto.Diagnostics
	// ConfigureHost changes the Auto-host setup, or turns it on or off.
	ConfigureHost(ctx context.Context, cfg dto.HostConfig) (dto.FollowState, error)
	// MapsFolders lists the chosen Maps folder, then the detected ones.
	MapsFolders(ctx context.Context) []dto.MapsFolder
	// SetMapsFolder chooses the Maps folder; "" goes back to detecting it.
	SetMapsFolder(path string) (dto.FollowState, error)
	// BrowseMaps lists one folder of the Maps folder.
	BrowseMaps(ctx context.Context, dir string) (dto.MapListing, error)
	// SearchMaps finds maps by name anywhere in the Maps folder.
	SearchMaps(ctx context.Context, query string) (dto.MapListing, error)
	// CloseGames closes every copy of Warcraft III this user runs, when the
	// player asked for it and confirmed, then looks for the game again.
	CloseGames(ctx context.Context) (dto.FollowState, error)
	// RequestKeyboardPermission asks the system for the permission Game
	// mode's typing needs, which may show a system prompt. Call it only when
	// the player asks.
	RequestKeyboardPermission(ctx context.Context) (dto.FollowState, error)
	// ClearRecentPlayers forgets every recent player, on this computer too.
	// The follow group stays as it is.
	ClearRecentPlayers() (dto.FollowState, error)
	// CheckForUpdates starts an update check the player asked for; the
	// answer arrives with a later state (ADR 0009).
	CheckForUpdates() (dto.FollowState, error)
	// UpdateLater hides the update banner until a newer release is found or
	// the player checks again.
	UpdateLater() dto.FollowState
	// SkipUpdate silences the release on offer for good; the next release is
	// offered again.
	SkipUpdate() (dto.FollowState, error)
	// SetNote saves the player's private note about the player with tag;
	// empty text deletes it. Notes hold at most dto.MaxNoteLength characters
	// and stay on this computer.
	SetNote(tag, text string) (dto.FollowState, error)
	// Subscribe calls fn with every new state until the returned function
	// is called.
	Subscribe(fn func(dto.FollowState)) (unsubscribe func())
}
