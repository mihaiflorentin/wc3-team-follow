package dto

import "time"

// Release is Team Follow's newest stable release on GitHub, as
// contract.Updater found it.
type Release struct {
	// Tag is the release's git tag, such as v0.8.0.
	Tag string
	// Notes is the release's description on GitHub, as Markdown text.
	Notes string
	// Changelog is CHANGELOG.md at the release's tag, as Markdown text, or
	// "" when it couldn't be read.
	Changelog string
	// PageURL is the release's page on github.com.
	PageURL   string
	Published time.Time
	// AssetName and AssetURL are the release file for this system, or ""
	// when the release has none.
	AssetName string
	AssetURL  string
	// ChecksumURL is the release's checksums.txt, or "" without one.
	ChecksumURL string
}

// Update phases (UpdateState.Phase). The in-app update (issue 23) adds
// downloading, verifying, installing, restarting and failed.
const (
	UpdateIdle      = "idle"
	UpdateChecking  = "checking"
	UpdateAvailable = "available"
)

// UpdateState is the update check's part of FollowState (ADR 0009).
type UpdateState struct {
	// Supported is false for a build that isn't a clean release version,
	// such as a development build; it never checks or updates.
	Supported bool `json:"supported"`
	// Phase is idle, checking or available.
	Phase string `json:"phase"`
	// CurrentVersion is the running version, without the leading v of a
	// release build.
	CurrentVersion string `json:"currentVersion"`
	// CheckAutomatically checks at start-up and every few hours.
	CheckAutomatically bool `json:"checkAutomatically"`
	// Release is the newer release found, or nil.
	Release *UpdateRelease `json:"release"`
	// Prompt shows the banner: a newer release the player neither skipped
	// nor put off with Later.
	Prompt bool `json:"prompt"`
	// Skipped reports that Release is the version the player skipped.
	Skipped bool `json:"skipped"`
	// Warning says that updating restarts Team Follow and turns Follow off,
	// while the player is in a lobby or match or Auto-join or Auto-host is
	// on; "" otherwise.
	Warning string `json:"warning"`
	// Message is the result of the last check the player asked for, or why
	// updates are off; "" otherwise.
	Message string `json:"message"`
	// LastChecked is when the last check finished.
	LastChecked *time.Time `json:"lastChecked,omitempty"`
	// ReleasesURL is the releases page, for updating by hand.
	ReleasesURL string `json:"releasesUrl"`
}

// UpdateRelease is the newer release shown in the banner.
type UpdateRelease struct {
	// Version is the release's version without the leading v, like 0.8.0.
	Version string `json:"version"`
	// URL is the release's page on github.com.
	URL       string    `json:"url"`
	Published time.Time `json:"published"`
	// Changes lists the changelog sections newer than the running version,
	// up to and including Version, newest first. Never nil.
	Changes []ChangelogSection `json:"changes"`
	// Notes is the release's description as plain text, shown when Changes
	// is empty.
	Notes string `json:"notes"`
}

// ChangelogSection is one version's entries in CHANGELOG.md.
type ChangelogSection struct {
	Version string `json:"version"`
	// Date is the release date as written, like 2026-10-28, or "".
	Date   string           `json:"date"`
	Groups []ChangelogGroup `json:"groups"`
}

// ChangelogGroup is one group of entries, such as Added or Fixed.
type ChangelogGroup struct {
	Name    string           `json:"name"`
	Entries []ChangelogEntry `json:"entries"`
}

// ChangelogEntry is one changelog line as plain-text spans, so the window
// never renders Markdown or HTML that came from the network.
type ChangelogEntry struct {
	Spans []TextSpan `json:"spans"`
}

// TextSpan is a run of plain text, bold or code as the changelog marks it.
type TextSpan struct {
	Text string `json:"text"`
	Bold bool   `json:"bold,omitempty"`
	Code bool   `json:"code,omitempty"`
}
