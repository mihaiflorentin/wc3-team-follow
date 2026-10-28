package updater

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The install steps for each system, written portably so the tests run
// them in temporary folders on every system; platform_<os>.go picks the
// one for the running system.

const (
	// stagingPrefix names the temporary folders beside the installed app.
	stagingPrefix = ".wc3-team-follow-update-"
	// programName is the program's name in the Linux release.
	programName = "wc3-team-follow"
)

// target is what an update replaces.
type target struct {
	// path is the program (Windows, Linux) or the app bundle (macOS).
	path string
	// dir is its folder, which holds the temporary folder, so the final
	// rename stays on one volume.
	dir string
}

// problem is an error the player can act on (contract.UpdateProblem).
type problem struct {
	sentence string
	err      error
}

func (p problem) Error() string {
	if p.err != nil {
		return p.sentence + " (" + p.err.Error() + ")"
	}
	return p.sentence
}
func (p problem) Unwrap() error   { return p.err }
func (p problem) Problem() string { return p.sentence }

const moveToApplications = "Move Warcraft Team Follow to the Applications folder, open it from there and try again."

// locateProgram is the target for a program replaced on its own.
func locateProgram(exe string) (target, error) {
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return target{path: exe, dir: filepath.Dir(exe)}, nil
}

// locateBundle is the .app bundle around exe. An app macOS translocated
// (opened straight from Downloads, run from a read-only copy) can't update.
func locateBundle(exe string) (target, error) {
	if strings.Contains(exe, "/AppTranslocation/") {
		return target{}, problem{sentence: "macOS is running Team Follow from a temporary copy, so it can't update itself. " + moveToApplications}
	}
	macOS := filepath.Dir(exe)
	contents := filepath.Dir(macOS)
	bundle := filepath.Dir(contents)
	if filepath.Base(macOS) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(bundle, ".app") {
		return target{}, problem{sentence: "Team Follow isn't running from its app, so it can't update itself. " + moveToApplications}
	}
	return target{path: bundle, dir: filepath.Dir(bundle)}, nil
}

// makeStaging creates the temporary folder beside the target, which also
// shows whether Team Follow may write there.
func makeStaging(t target, system string) (string, error) {
	dir, err := os.MkdirTemp(t.dir, stagingPrefix)
	if err == nil {
		return dir, nil
	}
	if errors.Is(err, fs.ErrPermission) || isReadOnly(err) {
		if system == "darwin" {
			return "", problem{sentence: "Team Follow can't write to the folder it is in, so it can't update itself. " + moveToApplications, err: err}
		}
		return "", problem{sentence: "Team Follow can't write to its folder, " + t.dir + ", so it can't update itself.", err: err}
	}
	return "", fmt.Errorf("create temporary folder: %w", err)
}

// replaceExe installs the new .exe on Windows, where the running program
// can be renamed but not overwritten: the old one becomes <exe>.old (removed
// at the next start) and the new one takes its name. If the new one can't
// be moved in, the old one is put back.
func replaceExe(exe, unpacked string) error {
	fresh, err := single(unpacked, func(e fs.DirEntry) bool {
		return !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".exe")
	})
	if err != nil {
		return err
	}
	old := exe + ".old"
	if err := os.Remove(old); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove the previous %s: %w", filepath.Base(old), err)
	}
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("move the running program aside: %w", err)
	}
	if err := os.Rename(fresh, exe); err != nil {
		if back := os.Rename(old, exe); back != nil {
			return fmt.Errorf("move the new program in: %w (and putting the old one back failed: %v)", err, back)
		}
		return fmt.Errorf("move the new program in: %w", err)
	}
	return nil
}

// swapBundle installs the new .app on macOS: the old bundle moves into the
// temporary folder (removed with it) and the new one takes its place, both
// by rename. If the new one can't be moved in, the old one is put back.
func swapBundle(bundle, unpacked, staging string) error {
	fresh, err := single(unpacked, func(e fs.DirEntry) bool { return e.IsDir() && strings.HasSuffix(e.Name(), ".app") })
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(fresh, "Contents", "MacOS")); err != nil {
		return fmt.Errorf("the new app has no program: %w", err)
	}
	old := filepath.Join(staging, "old.app")
	if err := os.Rename(bundle, old); err != nil {
		return fmt.Errorf("move the running app aside: %w", err)
	}
	if err := os.Rename(fresh, bundle); err != nil {
		if back := os.Rename(old, bundle); back != nil {
			return fmt.Errorf("move the new app in: %w (and putting the old one back failed: %v)", err, back)
		}
		return fmt.Errorf("move the new app in: %w", err)
	}
	return nil
}

// replaceBinary installs the new program on Linux with one atomic rename
// over the old one, keeping the old one's mode. File capabilities (setcap)
// don't carry over to the new file.
func replaceBinary(exe, unpacked string) error {
	fresh := filepath.Join(unpacked, programName)
	if info, err := os.Lstat(fresh); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("the release has no %s program", programName)
	}
	info, err := os.Stat(exe)
	if err != nil {
		return fmt.Errorf("read the running program: %w", err)
	}
	if err := os.Chmod(fresh, info.Mode().Perm()); err != nil {
		return fmt.Errorf("set the new program's mode: %w", err)
	}
	if err := os.Rename(fresh, exe); err != nil {
		return fmt.Errorf("move the new program in: %w", err)
	}
	return nil
}

// cleanup removes what an earlier update left beside the target: the
// replaced Windows program (<exe>.old) and temporary folders.
func cleanup(t target) error {
	var errs []error
	if err := os.Remove(t.path + ".old"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		errs = append(errs, err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(t.dir, stagingPrefix+"*"))
	for _, dir := range leftovers {
		if err := os.RemoveAll(dir); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// single finds the one entry at the top of dir that match accepts.
func single(dir string, match func(fs.DirEntry) bool) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	found := ""
	for _, e := range entries {
		if match(e) {
			if found != "" {
				return "", errors.New("the release holds more than one program")
			}
			found = filepath.Join(dir, e.Name())
		}
	}
	if found == "" {
		return "", errors.New("the release holds no program for this system")
	}
	return found, nil
}
