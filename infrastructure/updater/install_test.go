package updater_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/infrastructure/updater"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// installed is a fake install of Team Follow for the running system in a
// temporary folder: the program, or on macOS the app bundle around it.
type installed struct {
	dir, exe, asset string
	release         []byte
}

func fakeInstall(t *testing.T) installed {
	t.Helper()
	dir := t.TempDir()
	var in installed
	switch runtime.GOOS {
	case "darwin":
		in = installed{exe: filepath.Join(dir, "Warcraft Team Follow.app/Contents/MacOS/wc3-team-follow"), asset: "wc3-team-follow-macos-arm64.zip"}
		in.release = zipFiles(t, map[string]string{
			"Warcraft Team Follow.app/Contents/MacOS/wc3-team-follow": "new program",
			"Warcraft Team Follow.app/Contents/Info.plist":            "0.8.0",
		})
	case "windows":
		in = installed{exe: filepath.Join(dir, "wc3-team-follow.exe"), asset: "wc3-team-follow-windows-amd64.zip"}
		in.release = zipFiles(t, map[string]string{"wc3-team-follow-windows-amd64.exe": "new program"})
	default:
		in = installed{exe: filepath.Join(dir, "wc3-team-follow"), asset: "wc3-team-follow-linux-amd64.tar.gz"}
		in.release = tarFiles(t, map[string]string{"./wc3-team-follow": "new program", "./wc3-team-follow.desktop": "[Desktop Entry]"})
	}
	in.dir = dir
	if err := os.MkdirAll(filepath.Dir(in.exe), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in.exe, []byte("old program"), 0o755); err != nil {
		t.Fatal(err)
	}
	return in
}

func zipFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(0o755)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func tarFiles(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

// releaseHost serves a release's files as github.com would, from a local
// TLS server that the client reaches for every host.
type releaseHost struct {
	mu        sync.Mutex
	files     map[string][]byte
	requested []string
}

func (h *releaseHost) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requested = append(h.requested, r.Host+r.URL.Path)
	body, ok := h.files[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	_, _ = w.Write(body)
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// serveRelease returns an updater for in whose release file is file, with
// sums as checksums.txt ("" lists file's own SHA-256).
func serveRelease(t *testing.T, in installed, file []byte, sums string) (contract.Updater, dto.Release, *releaseHost) {
	t.Helper()
	base := "/" + repo + "/releases/download/v0.8.0/"
	if sums == "" {
		sums = sha([]byte("other")) + "  wc3-team-follow-other.zip\n" + sha(file) + "  " + in.asset + "\n"
	}
	host := &releaseHost{files: map[string][]byte{base + in.asset: file, base + "checksums.txt": []byte(sums)}}
	srv := httptest.NewTLSServer(host)
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
	u := updater.New(client, updater.Options{APIURL: "https://api.github.com", Repository: repo, Asset: in.asset, Executable: in.exe},
		slog.New(slog.DiscardHandler))
	rel := dto.Release{Tag: "v0.8.0", AssetName: in.asset, AssetURL: "https://github.com" + base + in.asset, ChecksumURL: "https://github.com" + base + "checksums.txt"}
	return u, rel, host
}

func installedProgram(t *testing.T, in installed) string {
	t.Helper()
	b, err := os.ReadFile(in.exe)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// leftovers lists the temporary folders beside the installed app.
func leftovers(t *testing.T, in installed) []string {
	t.Helper()
	dir := in.dir
	found, _ := filepath.Glob(filepath.Join(dir, ".wc3-team-follow-update-*"))
	return found
}

func TestDownloadVerifiesAndInstallReplacesTheApp(t *testing.T) {
	in := fakeInstall(t)
	u, rel, _ := serveRelease(t, in, in.release, "")
	var steps []dto.UpdateProgress
	path, err := u.Download(context.Background(), rel, func(p dto.UpdateProgress) { steps = append(steps, p) })
	if err != nil {
		t.Fatal(err)
	}
	if real, _ := filepath.EvalSymlinks(in.dir); filepath.Dir(filepath.Dir(path)) != in.dir && filepath.Dir(filepath.Dir(path)) != real {
		t.Fatalf("downloaded to %s, not beside the program", path)
	}
	if last := steps[len(steps)-1]; !last.Verifying || last.Done != int64(len(in.release)) {
		t.Fatalf("progress = %+v", steps)
	}
	if installedProgram(t, in) != "old program" {
		t.Fatal("Download changed the app")
	}
	if err := u.Install(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	if got := installedProgram(t, in); got != "new program" {
		t.Fatalf("program = %q", got)
	}
	if left := leftovers(t, in); len(left) != 0 {
		t.Fatalf("left %v", left)
	}
	if err := u.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(in.exe + ".old"); !os.IsNotExist(err) {
		t.Fatal("Cleanup left the replaced program")
	}
}

func TestAChecksumMismatchInstallsNothing(t *testing.T) {
	in := fakeInstall(t)
	tampered := append(append([]byte(nil), in.release...), 0)
	u, rel, _ := serveRelease(t, in, tampered, sha(in.release)+"  "+in.asset+"\n")
	_, err := u.Download(context.Background(), rel, func(dto.UpdateProgress) {})
	var p contract.UpdateProblem
	if !errors.As(err, &p) || !strings.Contains(p.Problem(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if installedProgram(t, in) != "old program" || len(leftovers(t, in)) != 0 {
		t.Fatal("a mismatching download left something behind")
	}
}

func TestDownloadNeedsTheFileInChecksums(t *testing.T) {
	in := fakeInstall(t)
	u, rel, _ := serveRelease(t, in, in.release, sha(in.release)+"  wc3-team-follow-other.zip\n")
	if _, err := u.Download(context.Background(), rel, func(dto.UpdateProgress) {}); err == nil {
		t.Fatal("downloaded a file checksums.txt doesn't list")
	}
	if len(leftovers(t, in)) != 0 {
		t.Fatal("left a temporary folder")
	}
}

func TestDownloadOnlyFetchesFromGitHub(t *testing.T) {
	in := fakeInstall(t)
	u, rel, host := serveRelease(t, in, in.release, "")
	for _, bad := range []dto.Release{
		{Tag: rel.Tag, AssetName: rel.AssetName, AssetURL: "https://evil.example/" + in.asset, ChecksumURL: rel.ChecksumURL},
		{Tag: rel.Tag, AssetName: rel.AssetName, AssetURL: strings.Replace(rel.AssetURL, "https", "http", 1), ChecksumURL: rel.ChecksumURL},
		{Tag: rel.Tag, AssetName: "../" + rel.AssetName, AssetURL: rel.AssetURL, ChecksumURL: rel.ChecksumURL},
	} {
		if _, err := u.Download(context.Background(), bad, func(dto.UpdateProgress) {}); err == nil {
			t.Errorf("downloaded %+v", bad)
		}
	}
	if len(host.requested) != 0 {
		t.Fatalf("requested %v", host.requested)
	}
}

func TestACancelledDownloadLeavesNothing(t *testing.T) {
	in := fakeInstall(t)
	u, rel, _ := serveRelease(t, in, in.release, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := u.Download(ctx, rel, func(dto.UpdateProgress) {}); err == nil {
		t.Fatal("a cancelled download succeeded")
	}
	if len(leftovers(t, in)) != 0 {
		t.Fatal("left a temporary folder")
	}
}

// otherRelease is a well-formed release file for the running system whose
// program isn't the published one.
func otherRelease(t *testing.T) []byte {
	t.Helper()
	switch runtime.GOOS {
	case "darwin":
		return zipFiles(t, map[string]string{"Warcraft Team Follow.app/Contents/MacOS/wc3-team-follow": "other program"})
	case "windows":
		return zipFiles(t, map[string]string{"wc3-team-follow-windows-amd64.exe": "other program"})
	default:
		return tarFiles(t, map[string]string{"./wc3-team-follow": "other program"})
	}
}

func TestInstallRefusesAFileSwappedAfterTheDownload(t *testing.T) {
	in := fakeInstall(t)
	u, rel, _ := serveRelease(t, in, in.release, "")
	path, err := u.Download(context.Background(), rel, func(dto.UpdateProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, otherRelease(t), 0o600); err != nil {
		t.Fatal(err)
	}
	err = u.Install(context.Background(), path)
	var p contract.UpdateProblem
	if !errors.As(err, &p) || !strings.Contains(p.Problem(), "checksum") || installedProgram(t, in) != "old program" {
		t.Fatalf("installed a swapped file: %v", err)
	}
	if len(leftovers(t, in)) != 0 {
		t.Fatal("left a temporary folder")
	}
}

func TestInstallRefusesAFilePlacedInAnUpdateFolder(t *testing.T) {
	in := fakeInstall(t)
	u, rel, _ := serveRelease(t, in, in.release, "")
	// A real download shows where update folders go.
	path, err := u.Download(context.Background(), rel, func(dto.UpdateProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	planted, err := os.MkdirTemp(filepath.Dir(filepath.Dir(path)), ".wc3-team-follow-update-")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(planted, in.asset)
	if err := os.WriteFile(file, otherRelease(t), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := u.Install(context.Background(), file); err == nil || installedProgram(t, in) != "old program" {
		t.Fatalf("installed a file the updater didn't download: %v", err)
	}
}

func TestInstallOnlyTakesItsOwnDownloads(t *testing.T) {
	in := fakeInstall(t)
	u, _, _ := serveRelease(t, in, in.release, "")
	elsewhere := filepath.Join(t.TempDir(), in.asset)
	if err := os.WriteFile(elsewhere, in.release, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := u.Install(context.Background(), elsewhere); err == nil || installedProgram(t, in) != "old program" {
		t.Fatalf("installed a file from elsewhere: %v", err)
	}
}

func TestABrokenReleaseLeavesTheAppAsItWas(t *testing.T) {
	in := fakeInstall(t)
	broken := zipFiles(t, map[string]string{"../evil": "x"})
	if strings.HasSuffix(in.asset, ".tar.gz") {
		broken = tarFiles(t, map[string]string{"../evil": "x"})
	}
	u, rel, _ := serveRelease(t, in, broken, "")
	path, err := u.Download(context.Background(), rel, func(dto.UpdateProgress) {})
	if err != nil {
		t.Fatal(err)
	}
	if err := u.Install(context.Background(), path); err == nil {
		t.Fatal("installed a release that escapes its folder")
	}
	if installedProgram(t, in) != "old program" || len(leftovers(t, in)) != 0 {
		t.Fatal("a failed install changed the app or left files")
	}
}
