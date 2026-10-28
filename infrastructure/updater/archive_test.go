package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// entry is one file in a test archive: a regular file, a folder (name
// ending in /) or a symbolic link (link set).
type entry struct {
	name, body, link string
	mode             fs.FileMode
}

func zipOf(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		mode := e.mode
		switch {
		case e.link != "":
			mode = fs.ModeSymlink | 0o777
		case strings.HasSuffix(e.name, "/"):
			mode = fs.ModeDir | 0o755
		case mode == 0:
			mode = 0o644
		}
		h.SetMode(mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		body := e.body
		if e.link != "" {
			body = e.link
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tarOf(t *testing.T, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: int64(e.mode.Perm()), Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		switch {
		case e.link != "":
			h.Typeflag, h.Linkname, h.Size = tar.TypeSymlink, e.link, 0
		case strings.HasSuffix(e.name, "/"):
			h.Typeflag, h.Mode, h.Size = tar.TypeDir, 0o755, 0
		case e.mode == 0:
			h.Mode = 0o644
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		_, _ = tw.Write([]byte(e.body))
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	_ = gz.Close()
	return buf.Bytes()
}

func writeArchive(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtractUnpacksFoldersFilesModesAndInnerLinks(t *testing.T) {
	entries := []entry{
		{name: "Warcraft Team Follow.app/"},
		{name: "Warcraft Team Follow.app/Contents/MacOS/wc3-team-follow", body: "new program", mode: 0o755},
		{name: "Warcraft Team Follow.app/Contents/Info.plist", body: "<plist/>"},
		{name: "__MACOSX/._Warcraft Team Follow.app", body: "resource fork"},
	}
	if runtime.GOOS != "windows" {
		// Windows unpacks no links: its release has none.
		entries = append(entries, entry{name: "Warcraft Team Follow.app/Contents/Current", link: "MacOS"})
	}
	for name, data := range map[string][]byte{"a.zip": zipOf(t, entries...), "a.tar.gz": tarOf(t, entries...)} {
		t.Run(name, func(t *testing.T) {
			dest := t.TempDir()
			if err := extract(writeArchive(t, name, data), dest); err != nil {
				t.Fatal(err)
			}
			program := filepath.Join(dest, "Warcraft Team Follow.app/Contents/MacOS/wc3-team-follow")
			if b, err := os.ReadFile(program); err != nil || string(b) != "new program" {
				t.Fatalf("program = %q, %v", b, err)
			}
			if info, _ := os.Stat(program); runtime.GOOS != "windows" && info.Mode().Perm()&0o100 == 0 {
				t.Fatalf("program lost its exec bit: %v", info.Mode())
			}
			if runtime.GOOS != "windows" {
				if link, err := os.Readlink(filepath.Join(dest, "Warcraft Team Follow.app/Contents/Current")); err != nil || link != "MacOS" {
					t.Fatalf("link = %q, %v", link, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dest, "__MACOSX")); !os.IsNotExist(err) {
				t.Fatal("unpacked macOS resource forks")
			}
		})
	}
}

func TestExtractRejectsPathsThatEscape(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []entry
	}{
		{"parent", []entry{{name: "../evil", body: "x"}}},
		{"nested parent", []entry{{name: "app/../../evil", body: "x"}}},
		{"absolute", []entry{{name: "/tmp/evil", body: "x"}}},
		{"backslash", []entry{{name: `..\evil`, body: "x"}}},
		{"link out", []entry{{name: "app/link", link: "../../outside"}}},
		{"absolute link", []entry{{name: "app/link", link: "/etc/passwd"}}},
		{"write through link", []entry{{name: "app/", body: ""}, {name: "app/link", link: "."}, {name: "app/link/evil", body: "x"}}},
		{"link chain out", []entry{{name: "a/", body: ""}, {name: "a/up", link: ".."}, {name: "b", link: "a/up/.."}}},
		{"duplicate", []entry{{name: "app", body: "one"}, {name: "app", body: "two"}}},
	} {
		for kind, data := range map[string][]byte{"zip": zipOf(t, tc.entries...), "tar.gz": tarOf(t, tc.entries...)} {
			t.Run(tc.name+" "+kind, func(t *testing.T) {
				root := t.TempDir()
				dest := filepath.Join(root, "new")
				if err := os.Mkdir(dest, 0o755); err != nil {
					t.Fatal(err)
				}
				err := extract(writeArchive(t, "a."+kind, data), dest)
				if err == nil {
					t.Fatal("unpacked an archive that escapes its folder")
				}
				t.Log(err)
				if _, err := os.Lstat(filepath.Join(root, "evil")); !os.IsNotExist(err) {
					t.Fatal("wrote outside the folder")
				}
			})
		}
	}
}

func TestExtractRejectsHardLinksAndDevices(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "hard", Typeflag: tar.TypeLink, Linkname: "/etc/passwd"})
	_ = tw.Close()
	_ = gz.Close()
	if err := extract(writeArchive(t, "a.tar.gz", buf.Bytes()), t.TempDir()); err == nil {
		t.Fatal("unpacked a hard link")
	}
}
