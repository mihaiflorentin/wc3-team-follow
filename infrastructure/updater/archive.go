package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// maxUnpacked and maxEntries bound what a release file may unpack to.
	maxUnpacked = 1 << 30
	maxEntries  = 10000
)

// extract unpacks a release file (.zip or .tar.gz) into dest, which must
// exist. It refuses the whole file when an entry would land outside dest:
// absolute paths, .. elements, backslashes, writes through a link, links
// that point outside dest, hard links and devices. macOS resource forks
// (__MACOSX) are skipped.
func extract(archive, dest string) error {
	u := &unpacker{root: dest}
	var err error
	switch {
	case strings.HasSuffix(archive, ".zip"):
		err = u.zip(archive)
	case strings.HasSuffix(archive, ".tar.gz"):
		err = u.tarGz(archive)
	default:
		err = fmt.Errorf("unknown archive type %q", filepath.Base(archive))
	}
	if err == nil {
		err = u.checkLinks()
	}
	if err != nil {
		return fmt.Errorf("unpack %s: %w", filepath.Base(archive), err)
	}
	return nil
}

type unpacker struct {
	root    string
	total   int64
	entries int
	links   []string
}

func (u *unpacker) zip(archive string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer func() { _ = zr.Close() }()
	for _, f := range zr.File {
		mode := f.Mode()
		switch {
		case mode.IsDir():
			err = u.dir(f.Name)
		case mode&fs.ModeSymlink != 0:
			err = u.zipLink(f)
		case mode.IsRegular():
			err = u.zipFile(f)
		default:
			err = fmt.Errorf("%q isn't a file, folder or link", f.Name)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (u *unpacker) zipFile(f *zip.File) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	return u.file(f.Name, f.Mode(), r)
}

func (u *unpacker) zipLink(f *zip.File) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	target, err := io.ReadAll(io.LimitReader(r, 4096))
	if err != nil {
		return err
	}
	return u.link(f.Name, string(target))
}

func (u *unpacker) tarGz(archive string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = u.dir(h.Name)
		case tar.TypeReg:
			err = u.file(h.Name, fs.FileMode(h.Mode).Perm(), tr)
		case tar.TypeSymlink:
			err = u.link(h.Name, h.Linkname)
		default:
			err = fmt.Errorf("%q isn't a file, folder or link", h.Name)
		}
		if err != nil {
			return err
		}
	}
}

// local turns an entry name into a path inside root, or reports that it
// would escape. skip is set for entries that aren't unpacked.
func (u *unpacker) local(name string) (dest string, skip bool, err error) {
	u.entries++
	if u.entries > maxEntries {
		return "", false, errors.New("too many entries")
	}
	if strings.Contains(name, `\`) || strings.HasPrefix(name, "/") {
		return "", false, fmt.Errorf("entry %q isn't a relative path", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", false, fmt.Errorf("entry %q leaves the folder", name)
		}
	}
	clean := path.Clean(name)
	if clean == "." {
		return "", true, nil
	}
	if !filepath.IsLocal(filepath.FromSlash(clean)) {
		return "", false, fmt.Errorf("entry %q leaves the folder", name)
	}
	if first, _, _ := strings.Cut(clean, "/"); first == "__MACOSX" {
		return "", true, nil
	}
	dest = filepath.Join(u.root, filepath.FromSlash(clean))
	// Nothing is written through a link: every folder on the way must be
	// a real folder (or not exist yet).
	parent := u.root
	parts := strings.Split(clean, "/")
	for _, part := range parts[:len(parts)-1] {
		parent = filepath.Join(parent, part)
		info, err := os.Lstat(parent)
		if errors.Is(err, fs.ErrNotExist) {
			break
		}
		if err != nil {
			return "", false, err
		}
		if !info.IsDir() {
			return "", false, fmt.Errorf("entry %q goes through %q, which isn't a folder", name, part)
		}
	}
	return dest, false, nil
}

func (u *unpacker) dir(name string) error {
	dest, skip, err := u.local(name)
	if err != nil || skip {
		return err
	}
	return os.MkdirAll(dest, 0o755)
}

func (u *unpacker) file(name string, mode fs.FileMode, r io.Reader) error {
	dest, skip, err := u.local(name)
	if err != nil || skip {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	// O_EXCL: an entry never replaces another one, or a link.
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode.Perm()|0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(r, maxUnpacked-u.total+1))
	u.total += n
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && u.total > maxUnpacked {
		err = errors.New("the archive unpacks to too much data")
	}
	if err == nil {
		// Keep the mode as packed (the umask may have cut it).
		err = os.Chmod(dest, mode.Perm()|0o600)
	}
	return err
}

func (u *unpacker) link(name, target string) error {
	if runtime.GOOS == "windows" {
		return fmt.Errorf("entry %q is a link", name)
	}
	dest, skip, err := u.local(name)
	if err != nil || skip {
		return err
	}
	if target == "" || strings.HasPrefix(target, "/") || strings.Contains(target, `\`) ||
		!filepath.IsLocal(filepath.FromSlash(path.Join(path.Dir(path.Clean(name)), target))) {
		return fmt.Errorf("link %q points outside the folder", name)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(target, dest); err != nil {
		return err
	}
	u.links = append(u.links, dest)
	return nil
}

// checkLinks resolves every unpacked link and refuses one that ends up
// outside root, through other links, or nowhere.
func (u *unpacker) checkLinks() error {
	if len(u.links) == 0 {
		return nil
	}
	root, err := filepath.EvalSymlinks(u.root)
	if err != nil {
		return err
	}
	for _, link := range u.links {
		real, err := filepath.EvalSymlinks(link)
		if err != nil {
			return fmt.Errorf("link %q points nowhere", filepath.Base(link))
		}
		if real != root && !strings.HasPrefix(real, root+string(filepath.Separator)) {
			return fmt.Errorf("link %q points outside the folder", filepath.Base(link))
		}
	}
	return nil
}
