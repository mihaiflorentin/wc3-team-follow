package contract

import "os"

// Sys defines a small filesystem and environment contract.
type Sys interface {
	EnsureDir(path string, perm os.FileMode) error
	Exists(path string) (bool, error)
	Abs(path string) (string, error)
	Getwd() (string, error)
	Open(path string) (*os.File, error)
	OpenFile(path string, flag int, perm os.FileMode) (*os.File, error)
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	Remove(path string) error
	RemoveAll(path string) error
	Rename(oldPath, newPath string) error
	Environ() []string
	Getenv(key string) string
	LookupEnv(key string) (string, bool)
}
