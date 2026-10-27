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
}

type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

type LoggingConfig struct {
	Default string `mapstructure:"default"`
	Level   string `mapstructure:"level"`
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
