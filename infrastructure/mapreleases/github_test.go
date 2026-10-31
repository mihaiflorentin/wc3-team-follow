package mapreleases_test

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/mapreleases"
	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/updater"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

const (
	repo  = "team-oze/Legion-TD"
	asset = "Legion_TD_11.4c-hf9_Team_OZE.w3x"
	base  = "/" + repo + "/releases/download/v11.4c-hf9/"
)

var legionTD = dto.TrackedMap{Name: "Legion TD", Repository: repo,
	Pattern: `(?i)^Legion_TD_(?P<version>\d[^_/\\ ]*)_Team_?OZE(?: \(\d+\))?\.w3x$`, Folder: "Download"}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// fakeGitHub answers as api.github.com and github.com would, from a local
// TLS server that the client reaches for every host.
type fakeGitHub struct {
	mu       sync.Mutex
	latest   map[string]any
	status   int
	header   http.Header
	files    map[string][]byte
	requests []*http.Request
}

func release(file []byte) map[string]any {
	return map[string]any{
		"tag_name": "v11.4c-hf9", "draft": false, "prerelease": false, "published_at": "2026-10-19T10:00:00Z",
		"html_url": "https://github.com/" + repo + "/releases/tag/v11.4c-hf9",
		"assets": []any{
			map[string]any{"name": "README.txt", "size": 10, "digest": "sha256:" + sha([]byte("x")), "browser_download_url": "https://github.com" + base + "README.txt"},
			map[string]any{"name": asset, "size": len(file), "digest": "sha256:" + sha(file), "browser_download_url": "https://github.com" + base + asset},
		},
	}
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	if r.Host == "api.github.com" && r.URL.Path == "/repos/"+repo+"/releases/latest" {
		for k, v := range f.header {
			w.Header()[k] = v
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
			return
		}
		_ = json.NewEncoder(w).Encode(f.latest)
		return
	}
	if body, ok := f.files[r.URL.Path]; ok && r.Host == "github.com" {
		_, _ = w.Write(body)
		return
	}
	http.NotFound(w, r)
}

func (f *fakeGitHub) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func serve(t *testing.T, f *fakeGitHub, maxSize int64) contract.MapReleases {
	t.Helper()
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)
	transport := srv.Client().Transport.(*http.Transport).Clone()
	addr := srv.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	// The test certificate is for example.com; GitHub's hosts reach it.
	transport.TLSClientConfig = &tls.Config{RootCAs: transport.TLSClientConfig.RootCAs, ServerName: "example.com"}
	client := updater.NewClient(0)
	client.Transport = transport
	return mapreleases.New(client, mapreleases.Options{APIURL: "https://api.github.com", MaxSize: maxSize}, slog.New(slog.DiscardHandler))
}

func TestLatestReturnsTheMatchingFileWithItsDigest(t *testing.T) {
	file := []byte("map bytes")
	f := &fakeGitHub{latest: release(file)}
	rel, err := serve(t, f, 0).Latest(context.Background(), legionTD)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Tag != "v11.4c-hf9" || rel.Version != "11.4c-hf9" || rel.AssetName != asset || rel.Size != int64(len(file)) ||
		rel.SHA256 != sha(file) || rel.URL != "https://github.com"+base+asset || rel.PageURL == "" || rel.Published.IsZero() ||
		rel.Map.Repository != repo {
		t.Fatalf("release = %+v", rel)
	}
	r := f.requests[0]
	if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" || r.Header.Get("User-Agent") != updater.UserAgent {
		t.Fatalf("headers = %v", r.Header)
	}
}

func TestLatestRefusesAReleaseWithoutADigest(t *testing.T) {
	for _, digest := range []any{nil, "", "md5:abc", "sha256:not-hex", "sha256:" + strings.Repeat("a", 63)} {
		latest := release([]byte("map"))
		a := latest["assets"].([]any)[1].(map[string]any)
		if digest == nil {
			delete(a, "digest")
		} else {
			a["digest"] = digest
		}
		_, err := serve(t, &fakeGitHub{latest: latest}, 0).Latest(context.Background(), legionTD)
		var problem contract.UpdateProblem
		if err == nil || !errors.As(err, &problem) || !strings.Contains(problem.Problem(), "checksum") {
			t.Fatalf("digest %v: err = %v", digest, err)
		}
	}
}

func TestLatestFindsNothingInPreReleasesOrWithoutAMatchingFile(t *testing.T) {
	pre := release([]byte("map"))
	pre["prerelease"] = true
	draft := release([]byte("map"))
	draft["draft"] = true
	other := release([]byte("map"))
	other["assets"] = other["assets"].([]any)[:1]
	offsite := release([]byte("map"))
	offsite["assets"].([]any)[1].(map[string]any)["browser_download_url"] = "https://example.com" + base + asset
	plain := release([]byte("map"))
	plain["assets"].([]any)[1].(map[string]any)["browser_download_url"] = "http://github.com" + base + asset
	for name, f := range map[string]*fakeGitHub{
		"pre-release": {latest: pre}, "draft": {latest: draft}, "no match": {latest: other},
		"off GitHub": {latest: offsite}, "plain HTTP": {latest: plain}, "none": {status: http.StatusNotFound},
	} {
		rel, err := serve(t, f, 0).Latest(context.Background(), legionTD)
		if err != nil || rel.Version != "" {
			t.Fatalf("%s: Latest = %+v, %v", name, rel, err)
		}
	}
}

func TestLatestReportsGitHubsRateLimit(t *testing.T) {
	f := &fakeGitHub{status: http.StatusForbidden, header: http.Header{"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {"2000000000"}}}
	_, err := serve(t, f, 0).Latest(context.Background(), legionTD)
	var retry contract.RetryLater
	if !errors.As(err, &retry) || retry.RetryAt().Unix() != 2000000000 {
		t.Fatalf("err = %v", err)
	}
}

// downloadable is a release of file served at its GitHub address.
func downloadable(t *testing.T, file []byte, served []byte, maxSize int64) (contract.MapReleases, dto.MapRelease, *fakeGitHub) {
	t.Helper()
	f := &fakeGitHub{latest: release(file), files: map[string][]byte{base + asset: served}}
	m := serve(t, f, maxSize)
	rel, err := m.Latest(context.Background(), legionTD)
	if err != nil {
		t.Fatal(err)
	}
	return m, rel, f
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range list {
		out = append(out, e.Name())
	}
	return out
}

func TestDownloadPlacesTheVerifiedFileAndReportsProgress(t *testing.T) {
	file := []byte(strings.Repeat("legion", 50000))
	m, rel, _ := downloadable(t, file, file, 0)
	dir := filepath.Join(t.TempDir(), "Download")
	var steps []dto.MapProgress
	path, err := m.Download(context.Background(), rel, dir, func(p dto.MapProgress) { steps = append(steps, p) })
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, asset) {
		t.Fatalf("path = %s", path)
	}
	if got, _ := os.ReadFile(path); string(got) != string(file) {
		t.Fatal("the placed file differs from the download")
	}
	if names := entries(t, dir); len(names) != 1 || names[0] != asset {
		t.Fatalf("folder holds %v", names)
	}
	if len(steps) < 2 || steps[0].Done != 0 || steps[len(steps)-1].Done != int64(len(file)) || steps[len(steps)-1].Total != int64(len(file)) {
		t.Fatalf("progress = %v", steps)
	}
	for i := 1; i < len(steps); i++ {
		if steps[i].Done < steps[i-1].Done {
			t.Fatalf("progress went back: %v", steps)
		}
	}
}

func TestDownloadWithADigestMismatchLeavesNoFile(t *testing.T) {
	m, rel, _ := downloadable(t, []byte("published map"), []byte("tampered map!"), 0)
	dir := t.TempDir()
	_, err := m.Download(context.Background(), rel, dir, func(dto.MapProgress) {})
	var problem contract.UpdateProblem
	if !errors.As(err, &problem) || !strings.Contains(problem.Problem(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if names := entries(t, dir); len(names) != 0 {
		t.Fatalf("folder holds %v", names)
	}
}

func TestDownloadStopsAtTheSizeCap(t *testing.T) {
	file := []byte(strings.Repeat("x", 4096))
	// GitHub lists a small size but serves more than the cap.
	m, rel, _ := downloadable(t, file[:100], file, 1024)
	dir := t.TempDir()
	if _, err := m.Download(context.Background(), rel, dir, func(dto.MapProgress) {}); err == nil {
		t.Fatal("no error past the size cap")
	}
	if names := entries(t, dir); len(names) != 0 {
		t.Fatalf("folder holds %v", names)
	}
	// A listed size over the cap is refused before anything is requested.
	m, rel, f := downloadable(t, file, file, 1024)
	before := f.count()
	if _, err := m.Download(context.Background(), rel, dir, func(dto.MapProgress) {}); err == nil || f.count() != before {
		t.Fatalf("err = %v, requests %d -> %d", err, before, f.count())
	}
}

func TestDownloadRefusesABadAssetName(t *testing.T) {
	file := []byte("map")
	m, good, f := downloadable(t, file, file, 0)
	dir := t.TempDir()
	for _, name := range []string{"Footmen_Frenzy.w3x", "../" + asset, "Download/" + asset, `..\` + asset, ""} {
		rel := good
		rel.AssetName = name
		before := f.count()
		if _, err := m.Download(context.Background(), rel, dir, func(dto.MapProgress) {}); err == nil || f.count() != before {
			t.Fatalf("%q: err = %v, requests %d -> %d", name, err, before, f.count())
		}
	}
	bad := good
	bad.URL = "https://example.com" + base + asset
	if _, err := m.Download(context.Background(), bad, dir, func(dto.MapProgress) {}); err == nil {
		t.Fatal("downloaded from outside GitHub")
	}
	if names := entries(t, filepath.Dir(dir)); len(names) != 1 {
		t.Fatalf("parent holds %v", names)
	}
	if names := entries(t, dir); len(names) != 0 {
		t.Fatalf("folder holds %v", names)
	}
}

func TestDownloadNeverOverwritesAFile(t *testing.T) {
	file := []byte("new map")
	m, rel, _ := downloadable(t, file, file, 0)
	dir := t.TempDir()
	existing := filepath.Join(dir, asset)
	if err := os.WriteFile(existing, []byte("player's own copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Download(context.Background(), rel, dir, func(dto.MapProgress) {}); err == nil {
		t.Fatal("no error for an existing file")
	}
	if got, _ := os.ReadFile(existing); string(got) != "player's own copy" {
		t.Fatalf("existing file = %q", got)
	}
	if names := entries(t, dir); len(names) != 1 {
		t.Fatalf("folder holds %v", names)
	}
}

func TestCancelledDownloadLeavesNoFile(t *testing.T) {
	file := []byte(strings.Repeat("x", 1<<20))
	m, rel, _ := downloadable(t, file, file, 0)
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	_, err := m.Download(ctx, rel, dir, func(p dto.MapProgress) {
		if p.Done > 0 {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("no error after cancelling")
	}
	if names := entries(t, dir); len(names) != 0 {
		t.Fatalf("folder holds %v", names)
	}
}
