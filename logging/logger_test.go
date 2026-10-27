package logging_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/logging"
)

func TestNew_TextFormatsKeepWithAttrsAndGroups(t *testing.T) {
	for _, format := range []logging.LoggerType{logging.LoggerTypeSimple, logging.LoggerTypeColor} {
		t.Run(string(format), func(t *testing.T) {
			var buf bytes.Buffer
			logger := logging.New(&config.LoggingConfig{Default: string(format), Level: "info"}, &buf)

			logger.With("request_id", "abc-123").WithGroup("http").Info("handled", "method", "GET")

			out := buf.String()
			for _, want := range []string{"handled", "request_id", "abc-123", "http.method", "GET"} {
				if !strings.Contains(out, want) {
					t.Errorf("output %q does not contain %q", out, want)
				}
			}
		})
	}
}

func TestNew_JSONFormatsKeepWithAttrsAndGroups(t *testing.T) {
	for _, format := range []logging.LoggerType{logging.LoggerTypeJSON, logging.LoggerTypePrettyJSON} {
		t.Run(string(format), func(t *testing.T) {
			var buf bytes.Buffer
			logger := logging.New(&config.LoggingConfig{Default: string(format), Level: "info"}, &buf)

			logger.With("request_id", "abc-123").WithGroup("http").With("route", "/health").Info("handled", "method", "GET")

			var got map[string]any
			if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
				t.Fatalf("output is not a JSON object: %v\n%s", err, buf.String())
			}
			if got["msg"] != "handled" {
				t.Errorf("msg = %v, want handled", got["msg"])
			}
			if got["request_id"] != "abc-123" {
				t.Errorf("request_id = %v, want abc-123", got["request_id"])
			}
			group, ok := got["http"].(map[string]any)
			if !ok {
				t.Fatalf("http group missing or not an object: %v", got["http"])
			}
			if group["method"] != "GET" {
				t.Errorf("http.method = %v, want GET", group["method"])
			}
			if group["route"] != "/health" {
				t.Errorf("http.route = %v, want /health", group["route"])
			}
		})
	}
}

func TestNew_AppliesConfiguredLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(&config.LoggingConfig{Default: string(logging.LoggerTypeSimple), Level: "warn"}, &buf)

	logger.Info("dropped")
	logger.Warn("kept")

	out := buf.String()
	if strings.Contains(out, "dropped") {
		t.Errorf("info record emitted at warn level: %q", out)
	}
	if !strings.Contains(out, "kept") {
		t.Errorf("warn record missing: %q", out)
	}
}

func TestNew_NilConfigDefaultsToSimpleInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.New(nil, &buf)

	logger.Debug("dropped")
	logger.Info("kept", "key", "value")

	out := buf.String()
	if strings.Contains(out, "dropped") {
		t.Errorf("debug record emitted at default level: %q", out)
	}
	if !strings.Contains(out, "INFO -> kept | key: value") {
		t.Errorf("unexpected simple output: %q", out)
	}
}
