package logging_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/logging"
)

func TestRotatingFileKeepsTheNewestFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "app.log")
	f, err := logging.OpenRotatingFile(path, 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"aaaaaaaa\n", "bbbbbbbb\n", "cccccccc\n", "dddddddd\n"} {
		if _, err := f.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()
	read := func(name string) string { b, _ := os.ReadFile(name); return string(b) }
	if got := read(path); got != "dddddddd\n" {
		t.Errorf("current = %q", got)
	}
	if got := read(path + ".1"); got != "cccccccc\n" {
		t.Errorf(".1 = %q", got)
	}
	if got := read(path + ".2"); got != "bbbbbbbb\n" {
		t.Errorf(".2 = %q", got)
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Error("kept more than 2 old files")
	}
}

func TestRotatingFileAppendsAcrossRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	for _, line := range []string{"one\n", "two\n"} {
		f, _ := logging.OpenRotatingFile(path, 1<<20, 1)
		_, _ = f.Write([]byte(line))
		_ = f.Close()
	}
	if b, _ := os.ReadFile(path); string(b) != "one\ntwo\n" {
		t.Fatalf("file = %q", b)
	}
}

func TestSetupWritesDebugJSONWithSourceToTheFile(t *testing.T) {
	dir := t.TempDir()
	var console bytes.Buffer
	cfg := &config.LoggingConfig{Default: "simple", Level: "info", File: true, FileLevel: "debug", FileMaxMB: 1, FileKeep: 1}
	logger, files, err := logging.Setup(cfg, &console, dir)
	if err != nil {
		t.Fatal(err)
	}
	logger.Debug("only.in.file", "k", "v")
	logger.Info("everywhere")
	if err := files.Close(); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(console.String(), "only.in.file") || !strings.Contains(console.String(), "everywhere") {
		t.Errorf("console = %q", console.String())
	}
	f, err := os.Open(filepath.Join(logging.Dir(dir), logging.LogFileName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var lines []map[string]any
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var line map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			t.Fatalf("not JSON: %s", scanner.Text())
		}
		lines = append(lines, line)
	}
	if len(lines) != 2 || lines[0]["msg"] != "only.in.file" || lines[0]["k"] != "v" {
		t.Fatalf("file lines = %v", lines)
	}
	if src, ok := lines[0]["source"].(map[string]any); !ok || !strings.HasSuffix(src["file"].(string), "file_test.go") {
		t.Errorf("source = %v", lines[0]["source"])
	}
	if _, err := os.Stat(filepath.Join(logging.Dir(dir), logging.CrashFileName)); err != nil {
		t.Errorf("crash file: %v", err)
	}
}

func TestSetupWithoutFileLogsToTheConsoleOnly(t *testing.T) {
	dir := t.TempDir()
	logger, files, err := logging.Setup(&config.LoggingConfig{Default: "simple", Level: "info"}, &bytes.Buffer{}, dir)
	if err != nil || logger == nil || files.Dir != "" {
		t.Fatalf("logger=%v files=%+v err=%v", logger, files, err)
	}
	if _, err := os.Stat(logging.Dir(dir)); !os.IsNotExist(err) {
		t.Error("created a log folder with file logging off")
	}
}
