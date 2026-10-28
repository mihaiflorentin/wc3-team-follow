package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	// maxAsset and maxChecksums bound the downloads.
	maxAsset     = 512 << 20
	maxChecksums = 64 << 10
	// progressEvery spaces out progress reports.
	progressEvery = 100 * time.Millisecond
	// defaultStall is how long a download may receive nothing when the
	// client has no timeout.
	defaultStall = 30 * time.Second
)

// errChecksum is a download whose SHA-256 differs from checksums.txt.
var errChecksum = problem{sentence: "The download didn't match its published checksum, so nothing was installed."}

// Download fetches checksums.txt and this system's release file into a
// temporary folder beside the installed app, then checks the file's
// SHA-256. Nothing is unpacked or run before the checksum matches; on any
// error the folder is removed.
func (g *GitHub) Download(ctx context.Context, rel dto.Release, progress func(dto.UpdateProgress)) (path string, err error) {
	if rel.AssetName == "" || rel.AssetName != g.opts.Asset || filepath.Base(rel.AssetName) != rel.AssetName {
		return "", fmt.Errorf("release %s has no file for this system", rel.Tag)
	}
	if !onGitHub(rel.AssetURL) || !onGitHub(rel.ChecksumURL) {
		return "", fmt.Errorf("release %s's files aren't on GitHub over HTTPS", rel.Tag)
	}
	t, err := g.locate()
	if err != nil {
		return "", err
	}
	staging, err := makeStaging(t, system)
	if err != nil {
		return "", err
	}
	defer func() {
		if err != nil {
			_ = os.RemoveAll(staging)
		}
	}()

	want, err := g.checksum(ctx, rel)
	if err != nil {
		return "", err
	}
	path = filepath.Join(staging, rel.AssetName)
	got, err := g.fetch(ctx, rel.AssetURL, path, progress)
	if err != nil {
		return "", err
	}
	progress(dto.UpdateProgress{Done: got.size, Total: got.size, Verifying: true})
	if got.sum != want {
		g.logger.WarnContext(ctx, "update.checksum_mismatch", slog.String("tag", rel.Tag), slog.String("asset", rel.AssetName))
		return "", errChecksum
	}
	g.logger.InfoContext(ctx, "update.downloaded", slog.String("tag", rel.Tag), slog.Int64("bytes", got.size))
	return path, nil
}

// Install unpacks a file Download verified and replaces the installed app.
// It only installs from its own temporary folder beside the app. On error
// the folder is removed and the installed app is as it was.
func (g *GitHub) Install(ctx context.Context, path string) error {
	t, err := g.locate()
	if err != nil {
		return err
	}
	staging := filepath.Dir(path)
	if filepath.Dir(staging) != t.dir || !strings.HasPrefix(filepath.Base(staging), stagingPrefix) {
		return fmt.Errorf("%s wasn't downloaded by the updater", filepath.Base(path))
	}
	defer func() { _ = os.RemoveAll(staging) }()
	unpacked := filepath.Join(staging, "new")
	if err := os.Mkdir(unpacked, 0o755); err != nil {
		return fmt.Errorf("create unpack folder: %w", err)
	}
	if err := extract(path, unpacked); err != nil {
		return err
	}
	if err := replace(t, unpacked, staging); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(t.path), err)
	}
	g.logger.InfoContext(ctx, "update.replaced", slog.String("path", t.path))
	return nil
}

// Cleanup removes the replaced program and temporary folders an earlier
// update left beside the installed app.
func (g *GitHub) Cleanup(context.Context) error {
	t, err := g.locate()
	if err != nil {
		// Nowhere to update means nothing was left behind.
		return nil
	}
	return cleanup(t)
}

// locate finds what an update replaces.
func (g *GitHub) locate() (target, error) {
	if g.opts.Executable == "" {
		return target{}, errors.New("the program's location is unknown")
	}
	return locate(g.opts.Executable)
}

// checksum reads checksums.txt and returns the SHA-256 it lists for the
// release file.
func (g *GitHub) checksum(ctx context.Context, rel dto.Release) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.ChecksumURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download checksums.txt: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download checksums.txt: GitHub answered %s", resp.Status)
	}
	sum, ok := parseChecksums(io.LimitReader(resp.Body, maxChecksums), rel.AssetName)
	if !ok {
		return "", problem{sentence: "The release's checksums.txt has no entry for this system's file, so nothing was installed."}
	}
	return sum, nil
}

// parseChecksums finds name's SHA-256 in sha256sum's output: one
// "<hex>  <name>" (or "<hex> *<name>") per line.
func parseChecksums(r io.Reader, name string) (string, bool) {
	scan := bufio.NewScanner(r)
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != name {
			continue
		}
		sum := strings.ToLower(fields[0])
		if b, err := hex.DecodeString(sum); err == nil && len(b) == sha256.Size {
			return sum, true
		}
	}
	return "", false
}

type fetched struct {
	sum  string
	size int64
}

// fetch downloads link into path, hashing it on the way and reporting
// progress. The download has no overall time limit, but gives up when
// nothing arrives for the client's timeout.
func (g *GitHub) fetch(ctx context.Context, link, path string, progress func(dto.UpdateProgress)) (fetched, error) {
	stall := g.client.Timeout
	if stall <= 0 {
		stall = defaultStall
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stalled := errors.New("the download stalled")
	watchdog := time.AfterFunc(stall, func() { cancel(stalled) })
	defer watchdog.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
	if err != nil {
		return fetched{}, err
	}
	req.Header.Set("User-Agent", userAgent)
	client := *g.client
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return fetched{}, fmt.Errorf("download %s: %w", filepath.Base(path), causeOf(ctx, err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fetched{}, fmt.Errorf("download %s: GitHub answered %s", filepath.Base(path), resp.Status)
	}
	total := max(resp.ContentLength, 0)
	if total > maxAsset {
		return fetched{}, fmt.Errorf("download %s: the file is too large", filepath.Base(path))
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fetched{}, fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	hash := sha256.New()
	var done int64
	last := time.Time{}
	buf := make([]byte, 64<<10)
	progress(dto.UpdateProgress{Total: total})
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			watchdog.Reset(stall)
			done += int64(n)
			if done > maxAsset {
				rerr = errors.New("the file is too large")
			} else if _, werr := f.Write(buf[:n]); werr != nil {
				rerr = werr
			} else {
				hash.Write(buf[:n])
			}
			if now := time.Now(); now.Sub(last) >= progressEvery {
				last = now
				progress(dto.UpdateProgress{Done: done, Total: total})
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			_ = f.Close()
			return fetched{}, fmt.Errorf("download %s: %w", filepath.Base(path), causeOf(ctx, rerr))
		}
	}
	if err := f.Close(); err != nil {
		return fetched{}, fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	if total > 0 && done != total {
		return fetched{}, fmt.Errorf("download %s: got %d of %d bytes", filepath.Base(path), done, total)
	}
	return fetched{sum: hex.EncodeToString(hash.Sum(nil)), size: done}, nil
}

// causeOf prefers why ctx ended (such as a stalled download) over the
// error it caused.
func causeOf(ctx context.Context, err error) error {
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	return err
}

// isReadOnly reports a read-only file system.
func isReadOnly(err error) bool { return errors.Is(err, syscall.EROFS) }
