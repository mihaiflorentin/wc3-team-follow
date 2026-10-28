// Package updater finds Team Follow's newest stable release through
// GitHub's public REST API (docs/adr/0009). It sends only GitHub's standard
// requests: no token, cookie or anything about the player.
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	userAgent = "wc3-team-follow"
	// maxRelease and maxChangelog bound what is read from the network.
	maxRelease   = 1 << 20
	maxChangelog = 512 << 10
)

// downloadHosts are the hosts a release's page and files may live on.
var downloadHosts = map[string]bool{
	"github.com":                           true,
	"objects.githubusercontent.com":        true,
	"release-assets.githubusercontent.com": true,
}

// Options locate the releases.
type Options struct {
	// APIURL is GitHub's API, https://api.github.com. It must use HTTPS.
	APIURL string
	// Repository is owner/name.
	Repository string
	// Asset is this system's release file name (AssetName).
	Asset string
}

// GitHub implements contract.Updater with GitHub Releases.
type GitHub struct {
	client *http.Client
	opts   Options
	logger *slog.Logger
}

// New returns the updater. client should have a timeout.
func New(client *http.Client, opts Options, logger *slog.Logger) contract.Updater {
	opts.APIURL = strings.TrimSuffix(opts.APIURL, "/")
	return &GitHub{client: client, opts: opts, logger: logger}
}

// NewClient returns the HTTP client for GitHub: it gives up after timeout
// and follows redirects only to GitHub's own hosts over HTTPS.
func NewClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" || (req.URL.Host != "api.github.com" && !downloadHosts[req.URL.Host]) {
				return fmt.Errorf("redirect to %s isn't to GitHub over HTTPS", req.URL.Redacted())
			}
			return nil
		},
	}
}

// AssetName is the release file for a system, or "" when there is none.
func AssetName(goos, goarch string) string {
	switch goos + "/" + goarch {
	case "windows/amd64":
		return "wc3-team-follow-windows-amd64.zip"
	case "darwin/arm64":
		return "wc3-team-follow-macos-arm64.zip"
	case "linux/amd64":
		return "wc3-team-follow-linux-amd64.tar.gz"
	}
	return ""
}

// rateLimited is GitHub's rate limit; it implements contract.RetryLater.
type rateLimited struct{ at time.Time }

func (e rateLimited) Error() string {
	return "github rate limit until " + e.at.Format(time.RFC3339)
}
func (e rateLimited) RetryAt() time.Time { return e.at }

type releaseJSON struct {
	TagName     string    `json:"tag_name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// Latest reads releases/latest, which GitHub answers with the newest
// release that is neither a draft nor a pre-release; both are checked again
// here. CHANGELOG.md is read at the release's tag; when that fails the
// release still counts, with an empty Changelog.
func (g *GitHub) Latest(ctx context.Context) (dto.Release, error) {
	base, err := url.Parse(g.opts.APIURL)
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return dto.Release{}, fmt.Errorf("updates API %q isn't an HTTPS address", g.opts.APIURL)
	}
	resp, err := g.get(ctx, "/repos/"+g.opts.Repository+"/releases/latest", "application/vnd.github+json")
	if err != nil {
		return dto.Release{}, fmt.Errorf("read latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return dto.Release{}, nil
	case resp.StatusCode != http.StatusOK:
		if at, ok := retryAt(resp); ok {
			return dto.Release{}, fmt.Errorf("read latest release: %w", rateLimited{at: at})
		}
		return dto.Release{}, fmt.Errorf("read latest release: GitHub answered %s", resp.Status)
	}
	var raw releaseJSON
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRelease)).Decode(&raw); err != nil {
		return dto.Release{}, fmt.Errorf("read latest release: %w", err)
	}
	if raw.Draft || raw.Prerelease || raw.TagName == "" {
		return dto.Release{}, nil
	}
	if !onGitHub(raw.HTMLURL) {
		return dto.Release{}, fmt.Errorf("read latest release: page %q isn't on github.com", raw.HTMLURL)
	}
	rel := dto.Release{Tag: raw.TagName, Notes: raw.Body, PageURL: raw.HTMLURL, Published: raw.PublishedAt}
	for _, a := range raw.Assets {
		if !onGitHub(a.URL) {
			continue
		}
		switch a.Name {
		case "checksums.txt":
			rel.ChecksumURL = a.URL
		case g.opts.Asset:
			if g.opts.Asset != "" {
				rel.AssetName, rel.AssetURL = a.Name, a.URL
			}
		}
	}
	changelog, err := g.changelog(ctx, raw.TagName)
	if err != nil {
		// The release notes stand in for it.
		g.logger.InfoContext(ctx, "update.changelog", slog.String("tag", raw.TagName), slog.Any("error", err))
	}
	rel.Changelog = changelog
	return rel, nil
}

// changelog reads CHANGELOG.md at tag through the contents API, as raw text.
func (g *GitHub) changelog(ctx context.Context, tag string) (string, error) {
	resp, err := g.get(ctx, "/repos/"+g.opts.Repository+"/contents/CHANGELOG.md?ref="+url.QueryEscape(tag), "application/vnd.github.raw+json")
	if err != nil {
		return "", fmt.Errorf("read changelog: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("read changelog: GitHub answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxChangelog+1))
	if err != nil {
		return "", fmt.Errorf("read changelog: %w", err)
	}
	if len(body) > maxChangelog {
		return "", errors.New("read changelog: file is too large")
	}
	return string(body), nil
}

// get sends one plain GET to the API, with GitHub's recommended headers
// and nothing else.
func (g *GitHub) get(ctx context.Context, path, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.opts.APIURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", userAgent)
	return g.client.Do(req)
}

// retryAt reads when GitHub allows the next request after a rate limit:
// Retry-After seconds, or X-RateLimit-Reset once none remain.
func retryAt(resp *http.Response) (time.Time, bool) {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return time.Time{}, false
	}
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
		return time.Now().Add(time.Duration(s) * time.Second), true
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return time.Unix(reset, 0), true
		}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return time.Now().Add(time.Minute), true
	}
	return time.Time{}, false
}

// onGitHub reports whether link is an HTTPS address on GitHub's own hosts.
func onGitHub(link string) bool {
	u, err := url.Parse(link)
	return err == nil && u.Scheme == "https" && u.User == nil && downloadHosts[u.Host]
}
