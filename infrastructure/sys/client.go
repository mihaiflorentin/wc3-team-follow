package sys

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

type client struct{}

// New constructs a filesystem and environment adapter backed by os/filepath.
func New() contract.Sys {
	return &client{}
}

func (c *client) EnsureDir(path string, perm os.FileMode) error {
	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("ensure dir %q: %w", path, err)
	}
	return nil
}

func (c *client) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("stat %q: %w", path, err)
}

func (c *client) Abs(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("abs %q: %w", path, err)
	}
	return absPath, nil
}

func (c *client) Getwd() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	return wd, nil
}

func (c *client) Open(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	return file, nil
}

func (c *client) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	file, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("open file %q: %w", path, err)
	}
	return file, nil
}

func (c *client) ReadFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", path, err)
	}
	return data, nil
}

func (c *client) WriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("write file %q: %w", path, err)
	}
	return nil
}

func (c *client) Remove(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove %q: %w", path, err)
	}
	return nil
}

func (c *client) RemoveAll(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove all %q: %w", path, err)
	}
	return nil
}

func (c *client) Rename(oldPath, newPath string) error {
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, err)
	}
	return nil
}

func (c *client) Environ() []string {
	return append([]string(nil), os.Environ()...)
}

func (c *client) Getenv(key string) string {
	return os.Getenv(key)
}

func (c *client) LookupEnv(key string) (string, bool) {
	return os.LookupEnv(key)
}
