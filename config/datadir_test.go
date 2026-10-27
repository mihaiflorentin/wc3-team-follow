package config_test

import (
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/config"
)

func TestDataDirHonoursTheOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(config.DataDirEnv, dir)
	got, err := config.DataDir(nil)
	if err != nil || got != dir {
		t.Fatalf("DataDir = %q, %v", got, err)
	}
}

func TestDataDirNeedsAFolderName(t *testing.T) {
	t.Setenv(config.DataDirEnv, "")
	if _, err := config.DataDir(&config.StorageConfig{}); err == nil {
		t.Fatal("accepted an empty dir_name")
	}
}
