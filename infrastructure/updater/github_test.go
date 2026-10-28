package updater_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/updater"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

const (
	repo  = "mihaiflorentin/wc3-team-follow"
	asset = "wc3-team-follow-macos-arm64.zip"
	dl    = "https://github.com/" + repo + "/releases/download/v0.8.0/"
)

// fakeGitHub answers the two requests the adapter makes, as GitHub's REST
// API does.
type fakeGitHub struct {
	mu        sync.Mutex
	latest    map[string]any
	status    int
	header    http.Header
	changelog string
	// changelogStatus fails the changelog request when set.
	changelogStatus int
	requests        []*http.Request
}

func newRelease() map[string]any {
	return map[string]any{
		"tag_name": "v0.8.0", "name": "v0.8.0", "body": "## What's Changed\n* Update check",
		"html_url": "https://github.com/" + repo + "/releases/tag/v0.8.0", "draft": false, "prerelease": false,
		"published_at": "2026-11-02T10:00:00Z",
		"assets": []any{
			map[string]any{"name": "checksums.txt", "browser_download_url": dl + "checksums.txt"},
			map[string]any{"name": "wc3-team-follow-linux-amd64.tar.gz", "browser_download_url": dl + "wc3-team-follow-linux-amd64.tar.gz"},
			map[string]any{"name": asset, "browser_download_url": dl + asset},
			map[string]any{"name": "wc3-team-follow-windows-amd64.zip", "browser_download_url": dl + "wc3-team-follow-windows-amd64.zip"},
		},
	}
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	switch r.URL.Path {
	case "/repos/" + repo + "/releases/latest":
		for k, v := range f.header {
			w.Header()[k] = v
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(`{"message":"error"}`))
			return
		}
		if f.latest == nil {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(f.latest)
	case "/repos/" + repo + "/contents/CHANGELOG.md":
		if f.changelogStatus != 0 {
			w.WriteHeader(f.changelogStatus)
			return
		}
		if r.Header.Get("Accept") != "application/vnd.github.raw+json" || r.URL.Query().Get("ref") != "v0.8.0" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(f.changelog))
	default:
		http.NotFound(w, r)
	}
}

func serve(t *testing.T, f *fakeGitHub) contract.Updater {
	t.Helper()
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)
	return updater.New(srv.Client(), updater.Options{APIURL: srv.URL, Repository: repo, Asset: asset}, slog.New(slog.DiscardHandler))
}

func TestLatestReturnsTheStableReleaseWithItsFileAndChangelog(t *testing.T) {
	f := &fakeGitHub{latest: newRelease(), changelog: "# Changelog\n## [0.8.0] - 2026-11-02\n"}
	rel, err := serve(t, f).Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v0.8.0" || rel.PageURL != "https://github.com/"+repo+"/releases/tag/v0.8.0" || rel.Notes != "## What's Changed\n* Update check" {
		t.Fatalf("release = %+v", rel)
	}
	if rel.AssetName != asset || rel.AssetURL != dl+asset || rel.ChecksumURL != dl+"checksums.txt" {
		t.Fatalf("files = %+v", rel)
	}
	if rel.Changelog != f.changelog || !rel.Published.Equal(time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("changelog or date = %+v", rel)
	}
	for _, r := range f.requests {
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "wc3-team-follow") || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
			t.Fatalf("request headers = %v", r.Header)
		}
	}
}

func TestLatestSkipsPreReleasesAndDrafts(t *testing.T) {
	for _, field := range []string{"prerelease", "draft"} {
		rel := newRelease()
		rel[field] = true
		got, err := serve(t, &fakeGitHub{latest: rel}).Latest(context.Background())
		if err != nil || got.Tag != "" {
			t.Errorf("%s: release = %+v, %v", field, got, err)
		}
	}
}

func TestLatestWithNoReleaseIsEmpty(t *testing.T) {
	got, err := serve(t, &fakeGitHub{}).Latest(context.Background())
	if err != nil || got.Tag != "" {
		t.Fatalf("release = %+v, %v", got, err)
	}
}

func TestLatestWithoutAFileForThisSystemLeavesTheAssetEmpty(t *testing.T) {
	rel := newRelease()
	rel["assets"] = []any{map[string]any{"name": "checksums.txt", "browser_download_url": dl + "checksums.txt"}}
	got, err := serve(t, &fakeGitHub{latest: rel}).Latest(context.Background())
	if err != nil || got.Tag != "v0.8.0" || got.AssetURL != "" || got.AssetName != "" || got.ChecksumURL == "" {
		t.Fatalf("release = %+v, %v", got, err)
	}
}

func TestLatestIgnoresLinksOffGitHubOrWithoutHTTPS(t *testing.T) {
	rel := newRelease()
	rel["assets"] = []any{
		map[string]any{"name": "checksums.txt", "browser_download_url": "http://github.com/" + repo + "/checksums.txt"},
		map[string]any{"name": asset, "browser_download_url": "https://evil.example.com/" + asset},
	}
	got, err := serve(t, &fakeGitHub{latest: rel}).Latest(context.Background())
	if err != nil || got.AssetURL != "" || got.ChecksumURL != "" {
		t.Fatalf("release = %+v, %v", got, err)
	}
	rel["html_url"] = "https://github.com.evil.example.com/release"
	if _, err := serve(t, &fakeGitHub{latest: rel}).Latest(context.Background()); err == nil {
		t.Fatal("accepted a release page off GitHub")
	}
}

func TestLatestFallsBackToTheNotesWhenTheChangelogCantBeRead(t *testing.T) {
	got, err := serve(t, &fakeGitHub{latest: newRelease(), changelogStatus: http.StatusNotFound}).Latest(context.Background())
	if err != nil || got.Tag != "v0.8.0" || got.Changelog != "" || got.Notes == "" {
		t.Fatalf("release = %+v, %v", got, err)
	}
}

func TestLatestReportsGitHubsRateLimit(t *testing.T) {
	reset := time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC)
	f := &fakeGitHub{status: http.StatusForbidden, header: http.Header{
		"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {strconv.FormatInt(reset.Unix(), 10)}}}
	_, err := serve(t, f).Latest(context.Background())
	var retry contract.RetryLater
	if !errors.As(err, &retry) || !retry.RetryAt().Equal(reset) {
		t.Fatalf("err = %v", err)
	}

	f = &fakeGitHub{status: http.StatusTooManyRequests, header: http.Header{"Retry-After": {"120"}}}
	before := time.Now()
	_, err = serve(t, f).Latest(context.Background())
	if !errors.As(err, &retry) || retry.RetryAt().Before(before.Add(119*time.Second)) {
		t.Fatalf("err = %v", err)
	}
}

func TestLatestReportsServerErrors(t *testing.T) {
	_, err := serve(t, &fakeGitHub{status: http.StatusBadGateway}).Latest(context.Background())
	var retry contract.RetryLater
	if err == nil || errors.As(err, &retry) {
		t.Fatalf("err = %v", err)
	}
}

func TestLatestRefusesAnAPIWithoutHTTPS(t *testing.T) {
	srv := httptest.NewServer(&fakeGitHub{latest: newRelease()})
	t.Cleanup(srv.Close)
	u := updater.New(srv.Client(), updater.Options{APIURL: srv.URL, Repository: repo, Asset: asset}, slog.New(slog.DiscardHandler))
	if _, err := u.Latest(context.Background()); err == nil {
		t.Fatal("called an API over plain HTTP")
	}
}

func TestTheClientFollowsRedirectsOnlyToGitHubOverHTTPS(t *testing.T) {
	client := updater.NewClient(10 * time.Second)
	for _, tc := range []struct {
		to string
		ok bool
	}{
		{"https://objects.githubusercontent.com/file", true},
		{"https://api.github.com/repos/x", true},
		{"http://objects.githubusercontent.com/file", false},
		{"https://evil.example.com/file", false},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.to, nil)
		if err := client.CheckRedirect(req, nil); (err == nil) != tc.ok {
			t.Errorf("redirect to %s: %v", tc.to, err)
		}
	}
	if client.Timeout != 10*time.Second {
		t.Fatalf("timeout = %v", client.Timeout)
	}
}

func TestAssetNamePerSystem(t *testing.T) {
	for _, tc := range []struct{ goos, goarch, want string }{
		{"windows", "amd64", "wc3-team-follow-windows-amd64.zip"},
		{"darwin", "arm64", "wc3-team-follow-macos-arm64.zip"},
		{"linux", "amd64", "wc3-team-follow-linux-amd64.tar.gz"},
		{"linux", "arm64", ""},
		{"darwin", "amd64", ""},
	} {
		if got := updater.AssetName(tc.goos, tc.goarch); got != tc.want {
			t.Errorf("AssetName(%s, %s) = %q", tc.goos, tc.goarch, got)
		}
	}
}
