package mocksys

import (
	"errors"
	"io"
	"os"
	"testing"
)

func TestClientOpenFallbackUsesStoredFiles(t *testing.T) {
	tests := []struct {
		name string
		open func(*Client) (*os.File, error)
	}{
		{
			name: "Open",
			open: func(client *Client) (*os.File, error) {
				return client.Open("config/app.toml")
			},
		},
		{
			name: "OpenFile",
			open: func(client *Client) (*os.File, error) {
				return client.OpenFile("config/app.toml", os.O_RDONLY, 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &Client{
				Files: map[string][]byte{
					"config/app.toml": []byte("name = \"sys\""),
				},
			}

			file, err := tt.open(client)
			if err != nil {
				t.Fatalf("open file: %v", err)
			}
			defer file.Close()

			got, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("read file handle: %v", err)
			}
			if string(got) != "name = \"sys\"" {
				t.Fatalf("unexpected file content: %q", string(got))
			}
		})
	}
}

func TestClientOpenFileWithCreateCreatesUsableState(t *testing.T) {
	client := &Client{}
	if err := client.EnsureDir("tmp", 0o755); err != nil {
		t.Fatalf("ensure dir: %v", err)
	}

	file, err := client.OpenFile("tmp/new.txt", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("open file with create: %v", err)
	}

	if _, err := file.Write([]byte("hello sys")); err != nil {
		_ = file.Close()
		t.Fatalf("write file handle: %v", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = file.Close()
		t.Fatalf("seek file handle: %v", err)
	}

	got, err := io.ReadAll(file)
	if err != nil {
		_ = file.Close()
		t.Fatalf("read file handle: %v", err)
	}
	if string(got) != "hello sys" {
		_ = file.Close()
		t.Fatalf("unexpected file content from handle: %q", string(got))
	}

	if err := file.Close(); err != nil {
		t.Fatalf("close file handle: %v", err)
	}

	exists, err := client.Exists("tmp/new.txt")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatalf("expected file to exist after OpenFile with os.O_CREATE")
	}

	stored, err := client.ReadFile("tmp/new.txt")
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}
	if string(stored) != "hello sys" {
		t.Fatalf("unexpected stored file content: %q", string(stored))
	}
}

func TestClientOpenFileWithCreateFailsWhenParentDirectoryMissing(t *testing.T) {
	client := &Client{}

	_, err := client.OpenFile("tmp/new.txt", os.O_CREATE|os.O_RDWR, 0o600)
	if err == nil {
		t.Fatalf("expected open file with missing parent to fail")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func TestClientEnsureDirFailsWhenPathSegmentIsFile(t *testing.T) {
	client := &Client{
		Files: map[string][]byte{
			"tmp": []byte("not a directory"),
		},
	}

	err := client.EnsureDir("tmp/nested", 0o755)
	if err == nil {
		t.Fatalf("expected EnsureDir to fail when a path segment is a file")
	}
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expected os.ErrInvalid, got %v", err)
	}
}

func TestClientWriteFileRejectsImpossibleFilesystemState(t *testing.T) {
	tests := []struct {
		name   string
		client *Client
		path   string
		errIs  error
	}{
		{
			name:   "missing parent directory",
			client: &Client{},
			path:   "tmp/file.txt",
			errIs:  os.ErrNotExist,
		},
		{
			name: "parent segment is a file",
			client: &Client{
				Files: map[string][]byte{
					"tmp": []byte("not a directory"),
				},
			},
			path:  "tmp/file.txt",
			errIs: os.ErrInvalid,
		},
		{
			name: "target path is a directory",
			client: &Client{
				Dirs: map[string]os.FileMode{
					"tmp": 0o755,
				},
			},
			path:  "tmp",
			errIs: os.ErrInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.client.WriteFile(tt.path, []byte("content"), 0o600)
			if err == nil {
				t.Fatalf("expected WriteFile to fail")
			}
			if !errors.Is(err, tt.errIs) {
				t.Fatalf("expected %v, got %v", tt.errIs, err)
			}
		})
	}
}

func TestClientRenameAllowsReplacingExistingFile(t *testing.T) {
	client := &Client{
		Files: map[string][]byte{
			"from.txt": []byte("from"),
			"to.txt":   []byte("to"),
		},
	}

	if err := client.Rename("from.txt", "to.txt"); err != nil {
		t.Fatalf("rename file: %v", err)
	}

	if exists, err := client.Exists("from.txt"); err != nil {
		t.Fatalf("exists old path: %v", err)
	} else if exists {
		t.Fatalf("expected old path to be removed")
	}

	got, err := client.ReadFile("to.txt")
	if err != nil {
		t.Fatalf("read renamed file: %v", err)
	}
	if string(got) != "from" {
		t.Fatalf("unexpected renamed file content: %q", string(got))
	}
}

func TestClientRenameAllowsReplacingEmptyDirectory(t *testing.T) {
	client := &Client{
		Dirs: map[string]os.FileMode{
			"src": 0o755,
			"dst": 0o755,
		},
		Files: map[string][]byte{
			"src/file.txt": []byte("from"),
		},
	}

	if err := client.Rename("src", "dst"); err != nil {
		t.Fatalf("rename directory: %v", err)
	}

	if exists, err := client.Exists("src"); err != nil {
		t.Fatalf("exists old dir: %v", err)
	} else if exists {
		t.Fatalf("expected old directory to be removed")
	}

	got, err := client.ReadFile("dst/file.txt")
	if err != nil {
		t.Fatalf("read moved file: %v", err)
	}
	if string(got) != "from" {
		t.Fatalf("unexpected moved file content: %q", string(got))
	}
}

func TestClientRenameRejectsNonEmptyDestinationDirectory(t *testing.T) {
	client := &Client{
		Dirs: map[string]os.FileMode{
			"src": 0o755,
			"dst": 0o755,
		},
		Files: map[string][]byte{
			"src/file.txt":     []byte("from"),
			"dst/existing.txt": []byte("already here"),
		},
	}

	err := client.Rename("src", "dst")
	if err == nil {
		t.Fatalf("expected rename into non-empty directory to fail")
	}
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected os.ErrExist, got %v", err)
	}
}
