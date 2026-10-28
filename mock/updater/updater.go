// Package updater holds a test double for contract.Updater. It never
// touches the network.
package updater

import (
	"context"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.Updater = (*Updater)(nil)

// Updater answers Latest with Release and Err and counts the calls. When
// Hold is set, Latest waits until it is closed, as a slow network would.
type Updater struct {
	mu      sync.Mutex
	Release dto.Release
	Err     error
	Hold    chan struct{}
	calls   int

	install    Install
	downloaded []string
	installed  []string
	cleanups   int
}

// Latest returns the configured release or error.
func (u *Updater) Latest(ctx context.Context) (dto.Release, error) {
	u.mu.Lock()
	u.calls++
	hold := u.Hold
	u.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return dto.Release{}, ctx.Err()
		}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.Release, u.Err
}

// Set changes the answer.
func (u *Updater) Set(rel dto.Release, err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.Release, u.Err = rel, err
}

// Calls is how many times Latest was called.
func (u *Updater) Calls() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls
}

// Install is the scripted answer to Download, Install and Cleanup. It
// never touches the network or the installed app.
type Install struct {
	// Progress is reported by Download, in order, before it waits on Hold.
	Progress []dto.UpdateProgress
	// Hold, when set, keeps Download waiting until it is closed or ctx ends.
	Hold chan struct{}
	// Path is the verified file Download returns.
	Path        string
	DownloadErr error
	InstallErr  error
	CleanupErr  error
}

// Script sets the answers for the in-app update.
func (u *Updater) Script(i Install) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.install = i
}

// Download reports the scripted progress and returns Path or DownloadErr.
func (u *Updater) Download(ctx context.Context, rel dto.Release, progress func(dto.UpdateProgress)) (string, error) {
	u.mu.Lock()
	u.downloaded = append(u.downloaded, rel.Tag)
	script := u.install
	u.mu.Unlock()
	for _, p := range script.Progress {
		progress(p)
	}
	if script.Hold != nil {
		select {
		case <-script.Hold:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if script.DownloadErr != nil {
		return "", script.DownloadErr
	}
	return script.Path, nil
}

// Install records path and returns InstallErr.
func (u *Updater) Install(_ context.Context, path string) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.installed = append(u.installed, path)
	return u.install.InstallErr
}

// Cleanup counts the call and returns CleanupErr.
func (u *Updater) Cleanup(context.Context) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.cleanups++
	return u.install.CleanupErr
}

// Downloaded lists the tags Download was asked for.
func (u *Updater) Downloaded() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.downloaded...)
}

// Installed lists the paths Install was asked to install.
func (u *Updater) Installed() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.installed...)
}

// Cleanups is how many times Cleanup was called.
func (u *Updater) Cleanups() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.cleanups
}
