package mocksys

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

var _ contract.Sys = (*Client)(nil)

// Client is a lightweight test double for the sys contract.
type Client struct {
	EnsureDirFunc func(path string, perm os.FileMode) error
	ExistsFunc    func(path string) (bool, error)
	AbsFunc       func(path string) (string, error)
	GetwdFunc     func() (string, error)
	OpenFunc      func(path string) (*os.File, error)
	OpenFileFunc  func(path string, flag int, perm os.FileMode) (*os.File, error)
	ReadFileFunc  func(path string) ([]byte, error)
	WriteFileFunc func(path string, data []byte, perm os.FileMode) error
	RemoveFunc    func(path string) error
	RemoveAllFunc func(path string) error
	RenameFunc    func(oldPath, newPath string) error

	Dirs      map[string]os.FileMode
	Files     map[string][]byte
	Env       map[string]string
	OpenFiles map[string]*os.File

	WorkingDir string
	AbsBase    string

	backingFiles map[string]string
}

func (c *Client) EnsureDir(path string, perm os.FileMode) error {
	if c.EnsureDirFunc != nil {
		return c.EnsureDirFunc(path, perm)
	}
	c.ensureMaps()
	for _, dir := range directoryChain(cleanPath(path), true) {
		if c.hasFilePath(dir) {
			return fmt.Errorf("ensure dir %q: %w", path, os.ErrInvalid)
		}
		c.Dirs[dir] = perm
	}
	return nil
}

func (c *Client) Exists(path string) (bool, error) {
	if c.ExistsFunc != nil {
		return c.ExistsFunc(path)
	}
	clean := cleanPath(path)
	if _, ok := c.Files[clean]; ok {
		return true, nil
	}
	if _, ok := c.Dirs[clean]; ok {
		return true, nil
	}
	return false, nil
}

func (c *Client) Abs(path string) (string, error) {
	if c.AbsFunc != nil {
		return c.AbsFunc(path)
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	base := c.AbsBase
	if base == "" {
		base = c.WorkingDir
	}
	if base == "" {
		base = string(filepath.Separator)
	}
	return filepath.Clean(filepath.Join(base, path)), nil
}

func (c *Client) Getwd() (string, error) {
	if c.GetwdFunc != nil {
		return c.GetwdFunc()
	}
	if c.WorkingDir == "" {
		return string(filepath.Separator), nil
	}
	return filepath.Clean(c.WorkingDir), nil
}

func (c *Client) Open(path string) (*os.File, error) {
	if c.OpenFunc != nil {
		return c.OpenFunc(path)
	}
	c.ensureMaps()
	clean := cleanPath(path)

	file, ok := c.OpenFiles[clean]
	if ok {
		return file, nil
	}

	backingPath, err := c.ensureBackingFile(clean)
	if err == nil {
		file, err := os.Open(backingPath)
		if err != nil {
			return nil, fmt.Errorf("open %q: %w", path, err)
		}
		return file, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}

	return nil, fmt.Errorf("open %q: %w", path, os.ErrNotExist)
}

func (c *Client) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	if c.OpenFileFunc != nil {
		return c.OpenFileFunc(path, flag, perm)
	}
	c.ensureMaps()
	clean := cleanPath(path)

	file, ok := c.OpenFiles[clean]
	if ok {
		return file, nil
	}

	backingPath, existed, err := c.prepareOpenFile(clean, flag)
	if err == nil {
		openFlag := flag
		if !existed && flag&os.O_CREATE != 0 {
			openFlag &^= os.O_EXCL
		}

		file, err := os.OpenFile(backingPath, openFlag, perm)
		if err != nil {
			return nil, fmt.Errorf("open file %q: %w", path, err)
		}
		return file, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("open file %q: %w", path, err)
	}

	return nil, fmt.Errorf("open file %q: %w", path, os.ErrNotExist)
}

func (c *Client) ReadFile(path string) ([]byte, error) {
	if c.ReadFileFunc != nil {
		return c.ReadFileFunc(path)
	}
	c.ensureMaps()
	clean := cleanPath(path)

	if backingPath, ok := c.backingFiles[clean]; ok {
		content, err := os.ReadFile(backingPath)
		if err != nil {
			return nil, fmt.Errorf("read file %q: %w", path, err)
		}
		c.Files[clean] = append([]byte(nil), content...)
		return append([]byte(nil), content...), nil
	}

	content, ok := c.Files[clean]
	if !ok {
		return nil, fmt.Errorf("read file %q: %w", path, os.ErrNotExist)
	}
	return append([]byte(nil), content...), nil
}

func (c *Client) WriteFile(path string, data []byte, perm os.FileMode) error {
	if c.WriteFileFunc != nil {
		return c.WriteFileFunc(path, data, perm)
	}
	c.ensureMaps()
	clean := cleanPath(path)
	if c.isDirectoryPath(clean) {
		return fmt.Errorf("write file %q: %w", path, os.ErrInvalid)
	}
	if err := c.validateParentDirs(clean, "write file"); err != nil {
		return err
	}
	c.Files[clean] = append([]byte(nil), data...)
	if backingPath, ok := c.backingFiles[clean]; ok {
		if err := os.WriteFile(backingPath, data, perm); err != nil {
			return fmt.Errorf("write file %q: %w", path, err)
		}
	}
	return nil
}

func (c *Client) Remove(path string) error {
	if c.RemoveFunc != nil {
		return c.RemoveFunc(path)
	}
	clean := cleanPath(path)

	if _, ok := c.Files[clean]; ok {
		c.cleanupBackingFile(clean)
		delete(c.Files, clean)
		delete(c.OpenFiles, clean)
		return nil
	}

	if _, ok := c.Dirs[clean]; ok {
		if c.hasChildren(clean) {
			return fmt.Errorf("remove %q: directory not empty", path)
		}
		delete(c.Dirs, clean)
		return nil
	}

	return fmt.Errorf("remove %q: %w", path, os.ErrNotExist)
}

func (c *Client) RemoveAll(path string) error {
	if c.RemoveAllFunc != nil {
		return c.RemoveAllFunc(path)
	}
	clean := cleanPath(path)

	c.cleanupBackingTree(clean)
	delete(c.Files, clean)
	delete(c.Dirs, clean)
	delete(c.OpenFiles, clean)

	for filePath := range c.Files {
		if hasDescendant(filePath, clean) {
			delete(c.Files, filePath)
			delete(c.OpenFiles, filePath)
		}
	}
	for dirPath := range c.Dirs {
		if hasDescendant(dirPath, clean) {
			delete(c.Dirs, dirPath)
		}
	}
	for openPath := range c.OpenFiles {
		if hasDescendant(openPath, clean) {
			delete(c.OpenFiles, openPath)
		}
	}
	return nil
}

func (c *Client) Rename(oldPath, newPath string) error {
	if c.RenameFunc != nil {
		return c.RenameFunc(oldPath, newPath)
	}
	c.ensureMaps()

	oldClean := cleanPath(oldPath)
	newClean := cleanPath(newPath)
	if oldClean == newClean {
		if _, ok := c.Files[oldClean]; ok {
			return nil
		}
		if _, ok := c.Dirs[oldClean]; ok {
			return nil
		}
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, os.ErrNotExist)
	}

	if _, ok := c.Files[oldClean]; ok {
		if err := c.validateRenameFileDestination(oldPath, newPath, newClean); err != nil {
			return err
		}
		c.syncBackedFile(oldClean)
		c.cleanupBackingFile(oldClean)
		content := append([]byte(nil), c.Files[oldClean]...)
		c.clearFileState(newClean)
		c.Files[newClean] = append([]byte(nil), content...)
		delete(c.Files, oldClean)
		if file, ok := c.OpenFiles[oldClean]; ok {
			c.OpenFiles[newClean] = file
			delete(c.OpenFiles, oldClean)
		}
		return nil
	}

	perm, ok := c.Dirs[oldClean]
	if !ok {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, os.ErrNotExist)
	}

	if err := c.validateRenameDirectoryDestination(oldPath, oldClean, newPath, newClean); err != nil {
		return err
	}

	c.syncBackedTree(oldClean)
	c.cleanupBackingTree(oldClean)
	c.clearEmptyDirectoryState(newClean)

	c.Dirs[newClean] = perm
	delete(c.Dirs, oldClean)

	for dirPath, dirPerm := range copyDirs(c.Dirs) {
		if !hasDescendant(dirPath, oldClean) {
			continue
		}
		targetPath := joinRenamedPath(oldClean, newClean, dirPath)
		c.Dirs[targetPath] = dirPerm
		delete(c.Dirs, dirPath)
	}

	for filePath, content := range copyFiles(c.Files) {
		if !hasDescendant(filePath, oldClean) {
			continue
		}
		targetPath := joinRenamedPath(oldClean, newClean, filePath)
		c.Files[targetPath] = append([]byte(nil), content...)
		delete(c.Files, filePath)
	}

	for filePath, file := range copyOpenFiles(c.OpenFiles) {
		if !hasDescendant(filePath, oldClean) {
			continue
		}
		targetPath := joinRenamedPath(oldClean, newClean, filePath)
		c.OpenFiles[targetPath] = file
		delete(c.OpenFiles, filePath)
	}

	return nil
}

func (c *Client) Environ() []string {
	if len(c.Env) == 0 {
		return []string{}
	}

	keys := make([]string, 0, len(c.Env))
	for key := range c.Env {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	values := make([]string, 0, len(keys))
	for _, key := range keys {
		values = append(values, key+"="+c.Env[key])
	}
	return values
}

func (c *Client) Getenv(key string) string {
	value, _ := c.LookupEnv(key)
	return value
}

func (c *Client) LookupEnv(key string) (string, bool) {
	value, ok := c.Env[key]
	return value, ok
}

func (c *Client) ensureMaps() {
	if c.Dirs == nil {
		c.Dirs = make(map[string]os.FileMode)
	}
	if c.Files == nil {
		c.Files = make(map[string][]byte)
	}
	if c.Env == nil {
		c.Env = make(map[string]string)
	}
	if c.OpenFiles == nil {
		c.OpenFiles = make(map[string]*os.File)
	}
	if c.backingFiles == nil {
		c.backingFiles = make(map[string]string)
	}
}

func (c *Client) hasChildren(path string) bool {
	for filePath := range c.Files {
		if hasDescendant(filePath, path) {
			return true
		}
	}
	for dirPath := range c.Dirs {
		if hasDescendant(dirPath, path) {
			return true
		}
	}
	return false
}

func cleanPath(path string) string {
	if path == "" {
		return "."
	}
	return filepath.Clean(path)
}

func hasDescendant(path, parent string) bool {
	parent = cleanPath(parent)
	path = cleanPath(path)
	if path == parent {
		return false
	}
	return strings.HasPrefix(path, parent+string(filepath.Separator))
}

func joinRenamedPath(oldBase, newBase, path string) string {
	relPath, err := filepath.Rel(oldBase, path)
	if err != nil {
		return cleanPath(path)
	}
	return cleanPath(filepath.Join(newBase, relPath))
}

func copyDirs(dirs map[string]os.FileMode) map[string]os.FileMode {
	cloned := make(map[string]os.FileMode, len(dirs))
	for path, perm := range dirs {
		cloned[path] = perm
	}
	return cloned
}

func copyFiles(files map[string][]byte) map[string][]byte {
	cloned := make(map[string][]byte, len(files))
	for path, content := range files {
		cloned[path] = append([]byte(nil), content...)
	}
	return cloned
}

func copyOpenFiles(files map[string]*os.File) map[string]*os.File {
	cloned := make(map[string]*os.File, len(files))
	for path, file := range files {
		cloned[path] = file
	}
	return cloned
}

func (c *Client) prepareOpenFile(path string, flag int) (string, bool, error) {
	c.ensureMaps()

	backingPath, err := c.ensureBackingFile(path)
	if err == nil {
		return backingPath, true, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	if flag&os.O_CREATE == 0 {
		return "", false, os.ErrNotExist
	}
	if c.isDirectoryPath(path) {
		return "", false, os.ErrExist
	}
	if err := c.validateParentDirs(path, "open file"); err != nil {
		return "", false, err
	}

	c.Files[path] = []byte{}

	backingPath, err = c.ensureBackingFile(path)
	if err != nil {
		return "", false, err
	}
	return backingPath, false, nil
}

func (c *Client) ensureBackingFile(path string) (string, error) {
	c.ensureMaps()

	if _, ok := c.Files[path]; !ok {
		return "", os.ErrNotExist
	}

	if backingPath, ok := c.backingFiles[path]; ok {
		if _, err := os.Stat(backingPath); err == nil {
			return backingPath, nil
		}
		delete(c.backingFiles, path)
	}

	file, err := os.CreateTemp("", "mocksys-*")
	if err != nil {
		return "", err
	}

	if _, err := file.Write(c.Files[path]); err != nil {
		name := file.Name()
		_ = file.Close()
		_ = os.Remove(name)
		return "", err
	}

	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}

	c.backingFiles[path] = name
	return name, nil
}

func (c *Client) syncBackedFile(path string) {
	backingPath, ok := c.backingFiles[path]
	if !ok {
		return
	}

	content, err := os.ReadFile(backingPath)
	if err != nil {
		return
	}
	c.Files[path] = append([]byte(nil), content...)
}

func (c *Client) syncBackedTree(path string) {
	for filePath := range c.backingFiles {
		if filePath == path || hasDescendant(filePath, path) {
			c.syncBackedFile(filePath)
		}
	}
}

func (c *Client) cleanupBackingFile(path string) {
	backingPath, ok := c.backingFiles[path]
	if !ok {
		return
	}
	delete(c.backingFiles, path)
	_ = os.Remove(backingPath)
}

func (c *Client) cleanupBackingTree(path string) {
	for filePath := range copyBackingFiles(c.backingFiles) {
		if filePath == path || hasDescendant(filePath, path) {
			c.cleanupBackingFile(filePath)
		}
	}
}

func (c *Client) hasFilePath(path string) bool {
	if _, ok := c.Files[path]; ok {
		return true
	}
	if _, ok := c.OpenFiles[path]; ok {
		return true
	}
	if _, ok := c.backingFiles[path]; ok {
		return true
	}
	return false
}

func (c *Client) hasDirPath(path string) bool {
	_, ok := c.Dirs[path]
	return ok
}

func (c *Client) hasAnyDescendants(path string) bool {
	for filePath := range c.Files {
		if hasDescendant(filePath, path) {
			return true
		}
	}
	for dirPath := range c.Dirs {
		if hasDescendant(dirPath, path) {
			return true
		}
	}
	for filePath := range c.OpenFiles {
		if hasDescendant(filePath, path) {
			return true
		}
	}
	for filePath := range c.backingFiles {
		if hasDescendant(filePath, path) {
			return true
		}
	}
	return false
}

func (c *Client) hasFileAncestor(path string) bool {
	for _, ancestor := range pathAncestors(path) {
		if c.hasFilePath(ancestor) {
			return true
		}
	}
	return false
}

func (c *Client) isDirectoryPath(path string) bool {
	clean := cleanPath(path)
	if clean == "." || clean == string(filepath.Separator) {
		return true
	}
	return c.hasDirPath(clean) || c.hasAnyDescendants(clean)
}

func (c *Client) validateParentDirs(path, op string) error {
	for _, dir := range directoryChain(path, false) {
		if c.hasFilePath(dir) {
			return fmt.Errorf("%s %q: %w", op, path, os.ErrInvalid)
		}
		if !c.isDirectoryPath(dir) {
			return fmt.Errorf("%s %q: %w", op, path, os.ErrNotExist)
		}
	}
	return nil
}

func (c *Client) validateRenameFileDestination(oldPath, newPath, newClean string) error {
	if err := c.validateParentDirs(newClean, "rename"); err != nil {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, unwrapErr(err))
	}
	if c.isDirectoryPath(newClean) {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, os.ErrInvalid)
	}
	if c.hasAnyDescendants(newClean) {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, os.ErrInvalid)
	}
	return nil
}

func (c *Client) validateRenameDirectoryDestination(oldPath, oldClean, newPath, newClean string) error {
	if hasDescendant(newClean, oldClean) {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, os.ErrInvalid)
	}
	if err := c.validateParentDirs(newClean, "rename"); err != nil {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, unwrapErr(err))
	}
	if c.hasFilePath(newClean) {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, os.ErrInvalid)
	}
	if c.isDirectoryPath(newClean) && !c.isDirectoryEmpty(newClean) {
		return fmt.Errorf("rename %q to %q: %w", oldPath, newPath, os.ErrExist)
	}
	return nil
}

func (c *Client) isDirectoryEmpty(path string) bool {
	clean := cleanPath(path)
	for filePath := range c.Files {
		if hasDescendant(filePath, clean) {
			return false
		}
	}
	for dirPath := range c.Dirs {
		if hasDescendant(dirPath, clean) {
			return false
		}
	}
	for filePath := range c.OpenFiles {
		if hasDescendant(filePath, clean) {
			return false
		}
	}
	for filePath := range c.backingFiles {
		if hasDescendant(filePath, clean) {
			return false
		}
	}
	return true
}

func (c *Client) clearFileState(path string) {
	c.cleanupBackingFile(path)
	delete(c.Files, path)
	delete(c.OpenFiles, path)
}

func (c *Client) clearEmptyDirectoryState(path string) {
	if c.isDirectoryEmpty(path) {
		delete(c.Dirs, path)
	}
}

func directoryChain(path string, includeSelf bool) []string {
	clean := cleanPath(path)
	if clean == "." || clean == string(filepath.Separator) {
		return nil
	}

	ancestors := pathAncestors(clean)
	chain := make([]string, 0, len(ancestors)+1)
	for i := len(ancestors) - 1; i >= 0; i-- {
		dir := ancestors[i]
		if dir == "." || dir == string(filepath.Separator) {
			continue
		}
		chain = append(chain, dir)
	}
	if includeSelf {
		chain = append(chain, clean)
	}
	return chain
}

func unwrapErr(err error) error {
	var target *os.PathError
	if errors.As(err, &target) {
		return target.Err
	}
	return err
}

func pathAncestors(path string) []string {
	clean := cleanPath(path)
	ancestors := []string{}
	parent := filepath.Dir(clean)
	for parent != clean {
		ancestors = append(ancestors, parent)
		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		clean = parent
		parent = next
	}
	return ancestors
}

func copyBackingFiles(files map[string]string) map[string]string {
	cloned := make(map[string]string, len(files))
	for path, file := range files {
		cloned[path] = file
	}
	return cloned
}
