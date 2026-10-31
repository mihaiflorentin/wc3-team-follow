// Package mapreleases keeps tracked custom maps current from their GitHub
// Releases (docs/adr/0012). It follows the app updater's network rule: plain
// GETs to GitHub's own hosts over HTTPS, with no token, cookie or anything
// about the player.
package mapreleases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/maps"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/updater"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	// DefaultMaxSize caps a map download when Options.MaxSize is 0.
	DefaultMaxSize = 256 << 20
	// maxRelease bounds the release JSON read from the API.
	maxRelease = 1 << 20
	// progressEvery spaces out progress reports.
	progressEvery = 100 * time.Millisecond
	// defaultStall is how long a download may receive nothing when the
	// client has no timeout.
	defaultStall = 30 * time.Second
	// tempPrefix starts the temporary file's name; the dot hides it from
	// the map browser, and the .part suffix from the game.
	tempPrefix = ".wc3-team-follow-map-"
)

// Options locate GitHub and bound downloads.
type Options struct {
	// APIURL is GitHub's API, https://api.github.com. It must use HTTPS.
	APIURL string
	// MaxSize caps a download in bytes; 0 uses DefaultMaxSize.
	MaxSize int64
}

// GitHub implements contract.MapReleases with GitHub Releases.
type GitHub struct {
	client *http.Client
	opts   Options
	logger *slog.Logger
}

// New returns the map releases client. client should come from
// updater.NewClient, which follows redirects only to GitHub's hosts over
// HTTPS.
func New(client *http.Client, opts Options, logger *slog.Logger) contract.MapReleases {
	opts.APIURL = strings.TrimSuffix(opts.APIURL, "/")
	if opts.MaxSize <= 0 {
		opts.MaxSize = DefaultMaxSize
	}
	return &GitHub{client: client, opts: opts, logger: logger}
}

// problem is an error with a sentence for the player
// (contract.UpdateProblem).
type problem struct {
	sentence string
	cause    error
}

func (p problem) Error() string {
	if p.cause != nil {
		return p.sentence + " (" + p.cause.Error() + ")"
	}
	return p.sentence
}
func (p problem) Problem() string { return p.sentence }
func (p problem) Unwrap() error   { return p.cause }

type releaseJSON struct {
	TagName     string    `json:"tag_name"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
		URL    string `json:"browser_download_url"`
	} `json:"assets"`
}

// Latest reads releases/latest, which GitHub answers with the newest
// release that is neither a draft nor a pre-release; both are checked again
// here. The first file whose name matches the map's pattern, on GitHub over
// HTTPS, is the map; its digest must be a SHA-256.
func (g *GitHub) Latest(ctx context.Context, tracked dto.TrackedMap) (dto.MapRelease, error) {
	pattern, err := maps.Pattern(tracked.Pattern)
	if err != nil {
		return dto.MapRelease{}, err
	}
	base, err := url.Parse(g.opts.APIURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return dto.MapRelease{}, fmt.Errorf("maps API %q isn't an HTTPS address", g.opts.APIURL)
	}
	if !validRepository(tracked.Repository) {
		return dto.MapRelease{}, fmt.Errorf("map repository %q isn't owner/name", tracked.Repository)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.opts.APIURL+"/repos/"+tracked.Repository+"/releases/latest", nil)
	if err != nil {
		return dto.MapRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", updater.UserAgent)
	resp, err := g.client.Do(req)
	if err != nil {
		return dto.MapRelease{}, fmt.Errorf("read %s's latest release: %w", tracked.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return dto.MapRelease{}, nil
	case resp.StatusCode != http.StatusOK:
		if limit := updater.RateLimitError(resp); limit != nil {
			return dto.MapRelease{}, fmt.Errorf("read %s's latest release: %w", tracked.Name, limit)
		}
		return dto.MapRelease{}, fmt.Errorf("read %s's latest release: GitHub answered %s", tracked.Name, resp.Status)
	}
	var raw releaseJSON
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRelease)).Decode(&raw); err != nil {
		return dto.MapRelease{}, fmt.Errorf("read %s's latest release: %w", tracked.Name, err)
	}
	if raw.Draft || raw.Prerelease || raw.TagName == "" {
		return dto.MapRelease{}, nil
	}
	page := raw.HTMLURL
	if !updater.OnGitHub(page) {
		page = ""
	}
	for _, a := range raw.Assets {
		version := matchName(pattern, a.Name)
		if version == "" || !updater.OnGitHub(a.URL) {
			continue
		}
		sum, ok := digest(a.Digest)
		if !ok {
			return dto.MapRelease{}, problem{sentence: fmt.Sprintf("%s %s has no published checksum on GitHub, so it isn't downloaded.", tracked.Name, version)}
		}
		return dto.MapRelease{Map: tracked, Tag: raw.TagName, Version: version, AssetName: a.Name, Size: a.Size, SHA256: sum,
			URL: a.URL, PageURL: page, Published: raw.PublishedAt}, nil
	}
	return dto.MapRelease{}, nil
}

// validRepository reports an owner/name with no other path in it.
func validRepository(repo string) bool {
	owner, name, ok := strings.Cut(repo, "/")
	clean := func(s string) bool {
		return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\?#% ")
	}
	return ok && clean(owner) && clean(name)
}

// matchName returns the version in a map file's name, or "".
func matchName(pattern *regexp.Regexp, name string) string {
	if name == "" || name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || !maps.IsMap(name) {
		return ""
	}
	m := pattern.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return m[pattern.SubexpIndex("version")]
}

// digest reads GitHub's "sha256:<hex>" digest.
func digest(s string) (string, bool) {
	sum, ok := strings.CutPrefix(s, "sha256:")
	if !ok {
		return "", false
	}
	sum = strings.ToLower(sum)
	if b, err := hex.DecodeString(sum); err != nil || len(b) != sha256.Size {
		return "", false
	}
	return sum, true
}

// errChecksum is a download whose SHA-256 differs from GitHub's digest.
func errChecksum(rel dto.MapRelease) error {
	return problem{sentence: fmt.Sprintf("The download of %s %s didn't match its published checksum, so it wasn't kept.", rel.Map.Name, rel.Version)}
}

// Download fetches the map into a temporary file in dir, hashing it as it
// streams, and links it into place under its own name only when the
// SHA-256 matches. An existing file is never replaced. On any error the
// temporary file is removed.
func (g *GitHub) Download(ctx context.Context, rel dto.MapRelease, dir string, progress func(dto.MapProgress)) (string, error) {
	pattern, err := maps.Pattern(rel.Map.Pattern)
	if err != nil {
		return "", err
	}
	if version := matchName(pattern, rel.AssetName); version == "" || version != rel.Version {
		return "", fmt.Errorf("release file %q isn't %s's map file", rel.AssetName, rel.Map.Name)
	}
	if !updater.OnGitHub(rel.URL) {
		return "", fmt.Errorf("release file %s isn't on GitHub over HTTPS", rel.AssetName)
	}
	want, ok := digest("sha256:" + rel.SHA256)
	if !ok {
		return "", problem{sentence: fmt.Sprintf("%s %s has no published checksum on GitHub, so it isn't downloaded.", rel.Map.Name, rel.Version)}
	}
	if rel.Size > g.opts.MaxSize {
		return "", problem{sentence: fmt.Sprintf("%s %s is larger than %d MB, so it isn't downloaded.", rel.Map.Name, rel.Version, g.opts.MaxSize>>20)}
	}
	final := filepath.Join(dir, rel.AssetName)
	if _, err := os.Lstat(final); err == nil {
		return "", problem{sentence: fmt.Sprintf("%s is already in your Maps folder, so it was left as it is.", rel.AssetName)}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", problem{sentence: "The Maps folder couldn't be written to.", cause: err}
	}
	removeLeftovers(dir)
	tmp, err := os.CreateTemp(dir, tempPrefix+"*.part")
	if err != nil {
		return "", problem{sentence: "The Maps folder couldn't be written to.", cause: err}
	}
	tmpPath := tmp.Name()
	// After a successful link the temporary name is removed too.
	defer func() { _ = os.Remove(tmpPath) }()

	got, size, err := g.fetch(ctx, rel, tmp, progress)
	if cerr := tmp.Close(); err == nil && cerr != nil {
		err = fmt.Errorf("write %s: %w", rel.AssetName, cerr)
	}
	if err != nil {
		return "", err
	}
	if got != want {
		g.logger.WarnContext(ctx, "maps.checksum_mismatch", slog.String("map", rel.Map.Name), slog.String("version", rel.Version))
		return "", errChecksum(rel)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return "", fmt.Errorf("prepare %s: %w", rel.AssetName, err)
	}
	if err := placeNew(tmpPath, final); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", problem{sentence: fmt.Sprintf("%s is already in your Maps folder, so it was left as it is.", rel.AssetName)}
		}
		return "", fmt.Errorf("place %s: %w", rel.AssetName, err)
	}
	g.logger.InfoContext(ctx, "maps.downloaded", slog.String("map", rel.Map.Name), slog.String("version", rel.Version), slog.Int64("bytes", size))
	return final, nil
}

// placeNew moves tmp to final without ever replacing a file there: a hard
// link fails when final exists. Where links aren't supported, it checks
// final is free and renames.
func placeNew(tmp, final string) error {
	err := os.Link(tmp, final)
	if err == nil || errors.Is(err, fs.ErrExist) {
		return err
	}
	if _, statErr := os.Lstat(final); statErr == nil {
		return fs.ErrExist
	}
	return os.Rename(tmp, final)
}

// removeLeftovers deletes temporary files an interrupted download left in
// dir; they are only ever this package's own.
func removeLeftovers(dir string) {
	found, _ := filepath.Glob(filepath.Join(dir, tempPrefix+"*.part"))
	for _, f := range found {
		_ = os.Remove(f)
	}
}

// fetch downloads rel.URL into f, hashing it and reporting progress. It has
// no overall time limit but gives up when nothing arrives for the client's
// timeout, and stops at the size cap.
func (g *GitHub) fetch(ctx context.Context, rel dto.MapRelease, f *os.File, progress func(dto.MapProgress)) (string, int64, error) {
	stall := g.client.Timeout
	if stall <= 0 {
		stall = defaultStall
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	stalled := errors.New("the download stalled")
	watchdog := time.AfterFunc(stall, func() { cancel(stalled) })
	defer watchdog.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rel.URL, nil)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("User-Agent", updater.UserAgent)
	client := *g.client
	client.Timeout = 0
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("download %s: %w", rel.AssetName, causeOf(ctx, err))
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("download %s: GitHub answered %s", rel.AssetName, resp.Status)
	}
	total := rel.Size
	if total <= 0 {
		total = max(resp.ContentLength, 0)
	}
	tooLarge := problem{sentence: fmt.Sprintf("%s %s is larger than %d MB, so it isn't downloaded.", rel.Map.Name, rel.Version, g.opts.MaxSize>>20)}
	if resp.ContentLength > g.opts.MaxSize {
		return "", 0, tooLarge
	}
	hash := sha256.New()
	var done int64
	var last time.Time
	buf := make([]byte, 64<<10)
	progress(dto.MapProgress{Total: total})
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			watchdog.Reset(stall)
			done += int64(n)
			if done > g.opts.MaxSize {
				return "", 0, tooLarge
			}
			if _, werr := f.Write(buf[:n]); werr != nil {
				return "", 0, problem{sentence: "The Maps folder couldn't be written to.", cause: werr}
			}
			hash.Write(buf[:n])
			if now := time.Now(); now.Sub(last) >= progressEvery {
				last = now
				progress(dto.MapProgress{Done: done, Total: max(total, done)})
			}
		}
		if errors.Is(rerr, io.EOF) {
			break
		}
		if rerr != nil {
			return "", 0, fmt.Errorf("download %s: %w", rel.AssetName, causeOf(ctx, rerr))
		}
	}
	if ctx.Err() != nil {
		return "", 0, fmt.Errorf("download %s: %w", rel.AssetName, causeOf(ctx, ctx.Err()))
	}
	progress(dto.MapProgress{Done: done, Total: max(total, done)})
	return hex.EncodeToString(hash.Sum(nil)), done, nil
}

// causeOf prefers why ctx ended (such as a stalled download) over the
// error it caused.
func causeOf(ctx context.Context, err error) error {
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	return err
}
