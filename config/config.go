package config

import (
	"bytes"
	"embed"
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

//go:embed config.toml
var configFS embed.FS

type Config struct {
	App     AppConfig      `mapstructure:"app"`
	Logging *LoggingConfig `mapstructure:"logging"`
	Follow  *FollowConfig  `mapstructure:"follow"`
	Storage *StorageConfig `mapstructure:"storage"`
	Window  *WindowConfig  `mapstructure:"window"`
	Updates *UpdatesConfig `mapstructure:"updates"`
	Maps    *MapsConfig    `mapstructure:"maps"`
}

type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
	// Version is the build's version, set by main from the build info,
	// never read from config.toml.
	Version string `mapstructure:"-"`
	// Executable is this program's path, which an update replaces, and
	// Updated marks the first start after an update; main sets both.
	Executable string `mapstructure:"-"`
	Updated    bool   `mapstructure:"-"`
	// Changelog is CHANGELOG.md, embedded by main, never read from
	// config.toml.
	Changelog string `mapstructure:"-"`
}

type LoggingConfig struct {
	Default string `mapstructure:"default"`
	Level   string `mapstructure:"level"`
	// File writes a JSON-lines log into <data folder>/logs for bug reports.
	File      bool   `mapstructure:"file"`
	FileLevel string `mapstructure:"file_level"`
	// FileMaxMB rotates the file at this size; FileKeep old files are kept.
	FileMaxMB int `mapstructure:"file_max_mb"`
	FileKeep  int `mapstructure:"file_keep"`
}

// FollowConfig tunes the companion that watches the game and joins friends.
type FollowConfig struct {
	// VerifiedBuilds lists the game versions live joining is allowed on.
	VerifiedBuilds    []string `mapstructure:"verified_builds"`
	RefreshInterval   string   `mapstructure:"refresh_interval"`
	DiscoveryInterval string   `mapstructure:"discovery_interval"`
}

// StorageConfig locates the per-user settings folder.
type StorageConfig struct {
	DirName string `mapstructure:"dir_name"`
}

// UpdatesConfig locates Team Follow's releases on GitHub (docs/adr/0009).
type UpdatesConfig struct {
	// APIURL is GitHub's API; it must use HTTPS.
	APIURL     string `mapstructure:"api_url"`
	Repository string `mapstructure:"repository"`
	// CheckInterval is the time between automatic checks, RetryInterval the
	// wait after a failed one and Timeout the limit on one request.
	CheckInterval string `mapstructure:"check_interval"`
	RetryInterval string `mapstructure:"retry_interval"`
	Timeout       string `mapstructure:"timeout"`
}

// MapsConfig is Keep maps up to date: the tracked maps and how their GitHub
// Releases are checked (docs/adr/0012).
type MapsConfig struct {
	// APIURL is GitHub's API; it must use HTTPS.
	APIURL string `mapstructure:"api_url"`
	// CheckInterval is the time between automatic checks, RetryInterval the
	// wait after a failed one and Timeout the limit on one request (and how
	// long a download may receive nothing).
	CheckInterval string `mapstructure:"check_interval"`
	RetryInterval string `mapstructure:"retry_interval"`
	Timeout       string `mapstructure:"timeout"`
	// MaxSizeMB caps one map download.
	MaxSizeMB int                `mapstructure:"max_size_mb"`
	Tracked   []TrackedMapConfig `mapstructure:"tracked"`
}

// TrackedMapConfig is one tracked map.
type TrackedMapConfig struct {
	Name       string `mapstructure:"name"`
	Repository string `mapstructure:"repository"`
	// Pattern must match the whole file name, with a group named version.
	Pattern string `mapstructure:"pattern"`
	// Folder is the Maps subfolder downloads go into.
	Folder string `mapstructure:"folder"`
}

// WindowConfig sizes the desktop window.
type WindowConfig struct {
	Title     string `mapstructure:"title"`
	Width     int    `mapstructure:"width"`
	Height    int    `mapstructure:"height"`
	MinWidth  int    `mapstructure:"min_width"`
	MinHeight int    `mapstructure:"min_height"`
}

func NewConfig() (*Config, error) {
	content, err := configFS.ReadFile("config.toml")
	if err != nil {
		return nil, fmt.Errorf("read embedded config: %w", err)
	}

	v := viper.New()
	v.SetConfigType("toml")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadConfig(bytes.NewBuffer(content)); err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}
