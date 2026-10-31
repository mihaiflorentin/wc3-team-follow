// Package mapreleases holds a test double for contract.MapReleases. It
// never touches the network or the Maps folder.
package mapreleases

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

var _ contract.MapReleases = (*Releases)(nil)

// Releases answers Latest with Release and Err, and Download with the
// scripted progress, then DownloadErr or the path the file would have in
// dir. When Hold is set, Download waits until it is closed or ctx ends.
type Releases struct {
	mu          sync.Mutex
	Release     dto.MapRelease
	Err         error
	Progress    []dto.MapProgress
	Hold        chan struct{}
	DownloadErr error
	// OnDownload runs after a successful Download, as the file appearing
	// in the Maps folder would.
	OnDownload func(path string)

	latest     int
	downloaded []string
	dirs       []string
}

// Set changes the answer to Latest.
func (r *Releases) Set(rel dto.MapRelease, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Release, r.Err = rel, err
}

// Script changes the answer to Download.
func (r *Releases) Script(progress []dto.MapProgress, hold chan struct{}, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Progress, r.Hold, r.DownloadErr = progress, hold, err
}

func (r *Releases) Latest(ctx context.Context, tracked dto.TrackedMap) (dto.MapRelease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latest++
	rel := r.Release
	if rel.Version != "" {
		rel.Map = tracked
	}
	return rel, r.Err
}

func (r *Releases) Download(ctx context.Context, rel dto.MapRelease, dir string, progress func(dto.MapProgress)) (string, error) {
	r.mu.Lock()
	r.downloaded = append(r.downloaded, rel.Version)
	r.dirs = append(r.dirs, dir)
	steps, hold, err, after := r.Progress, r.Hold, r.DownloadErr, r.OnDownload
	r.mu.Unlock()
	for _, p := range steps {
		progress(p)
	}
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, rel.AssetName)
	if after != nil {
		after(path)
	}
	return path, nil
}

// LatestCalls is how many times Latest was called.
func (r *Releases) LatestCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.latest
}

// Downloaded lists the versions Download was asked for.
func (r *Releases) Downloaded() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.downloaded...)
}

// Dirs lists the folders Download was asked to write into.
func (r *Releases) Dirs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.dirs...)
}
