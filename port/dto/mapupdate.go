package dto

import "time"

// TrackedMap is a custom map that publishes official GitHub Releases, which
// Team Follow keeps current in the Maps folder (ADR 0012). The list comes
// from config.toml ([maps] tracked).
type TrackedMap struct {
	// Name is the map's name for the window, like "Legion TD".
	Name string `json:"name"`
	// Repository is the GitHub repository that publishes it, owner/name.
	Repository string `json:"repository"`
	// Pattern is a regular expression the whole file name must match, with
	// a group named version: the map's version in that file name.
	Pattern string `json:"-"`
	// Folder is the Maps subfolder downloads go into, like "Download".
	Folder string `json:"-"`
}

// MapRelease is a tracked map's newest release, as contract.MapReleases
// found it.
type MapRelease struct {
	Map TrackedMap
	// Tag is the release's git tag, like v11.4c-hf9.
	Tag string
	// Version is the version in the asset's file name, like 11.4c-hf9;
	// installed files are compared with it.
	Version string
	// AssetName is the map file's name, like Legion_TD_11.4c-hf9_Team_OZE.w3x.
	AssetName string
	// Size is the file's size in bytes as GitHub lists it.
	Size int64
	// SHA256 is the file's hex SHA-256, from the digest GitHub publishes.
	SHA256 string
	// URL is the file's download address on GitHub.
	URL string
	// PageURL is the release's page on github.com.
	PageURL   string
	Published time.Time
}

// InstalledMap is one file of a tracked map found in the Maps folder.
type InstalledMap struct {
	// Version is the version in its file name.
	Version  string
	Path     string
	Modified time.Time
}

// MapProgress is how far a map's download is, in bytes; Total is 0 when
// the size isn't known.
type MapProgress struct {
	Done  int64 `json:"done"`
	Total int64 `json:"total"`
}

// Map update phases (MapUpdate.Phase).
const (
	MapIdle        = "idle"
	MapChecking    = "checking"
	MapUpToDate    = "upToDate"
	MapMissing     = "missing"
	MapDownloading = "downloading"
	MapDone        = "done"
	MapFailed      = "failed"
)

// MapUpdatesState is the Maps section's part of FollowState (ADR 0012).
type MapUpdatesState struct {
	// Enabled is "Keep maps up to date": check at start-up and every few
	// hours, and download the newest version when it's missing.
	Enabled bool `json:"enabled"`
	// Checking reports that a check (and any download it started) runs.
	Checking bool `json:"checking"`
	// Message is the answer to the last check the player asked for when it
	// failed as a whole, such as no Maps folder; "" otherwise.
	Message string `json:"message"`
	// LastChecked is when the last check finished.
	LastChecked *time.Time `json:"lastChecked,omitempty"`
	// Maps has one row per tracked map, in config order. Never nil.
	Maps []MapUpdate `json:"maps"`
}

// MapUpdate is one tracked map's row in the Maps section.
type MapUpdate struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
	// Installed is the version of the newest matching file in the Maps
	// folder (the newest version when it's there, otherwise the most
	// recently changed file's), or "" when none was found.
	Installed string `json:"installed"`
	// Latest is the newest release's version, or "" before a check found it.
	Latest string `json:"latest"`
	// Phase is idle, checking, upToDate, missing, downloading, done or
	// failed.
	Phase string `json:"phase"`
	// Progress is the download's progress while downloading, or nil.
	Progress *MapProgress `json:"progress"`
	// Message is a sentence for the row: ready, out but switched off, or
	// why it failed.
	Message string `json:"message"`
	// PageURL is the newest release's page on github.com, or "".
	PageURL string `json:"pageUrl"`
}
