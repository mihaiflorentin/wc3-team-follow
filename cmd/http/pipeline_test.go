package http_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpserver "github.com/mihaiflorentin/wc3-team-follow/cmd/http"
	"github.com/mihaiflorentin/wc3-team-follow/cmd/http/middleware"
	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/logging"
)

// These tests mount ad-hoc routes behind the same middleware pipeline that
// NewHandler uses, to exercise behaviour no production route has.

// accessLogFormats covers every format selectable through [logging]
// server_default.
var accessLogFormats = []logging.LoggerType{
	logging.LoggerTypeSimple, logging.LoggerTypeColor, logging.LoggerTypeJSON, logging.LoggerTypePrettyJSON,
}

// accessLogDeps returns Deps whose access log is written to w in format.
func accessLogDeps(format logging.LoggerType, w io.Writer) httpserver.Deps {
	cfg := &config.LoggingConfig{Default: string(logging.LoggerTypeJSON), ServerDefault: string(format)}
	return httpserver.Deps{Logger: discardLogger(), AccessLogger: logging.NewAccessLogger(cfg, w)}
}

func TestPipeline_AccessLogIsAnEventWithTypedAttributes(t *testing.T) {
	var logs bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("GET /teapot", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "short and stout", http.StatusTeapot)
	})

	httpserver.Pipeline(accessLogDeps(logging.LoggerTypeJSON, &logs), httpserver.HandlerOptions{}, mux).
		ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/teapot?x=1", nil))

	var record map[string]any
	if err := json.Unmarshal(logs.Bytes(), &record); err != nil {
		t.Fatalf("access log is not one JSON record: %v\n%s", err, logs.String())
	}
	want := map[string]any{
		"msg":           "http.access",
		"method":        "GET",
		"url":           "/teapot?x=1",
		"status":        float64(http.StatusTeapot),
		"status_class":  "4xx",
		"response_body": "short and stout\n",
	}
	for key, value := range want {
		if record[key] != value {
			t.Errorf("%s = %#v, want %#v", key, record[key], value)
		}
	}
	if _, ok := record["duration"].(float64); !ok {
		t.Errorf("duration = %#v, want a number", record["duration"])
	}
}

func TestPipeline_AccessLogUsesTheEventNameInEveryFormat(t *testing.T) {
	for _, format := range accessLogFormats {
		t.Run(string(format), func(t *testing.T) {
			var logs bytes.Buffer
			mux := http.NewServeMux()
			mux.HandleFunc("GET /ok", func(http.ResponseWriter, *http.Request) {})

			httpserver.Pipeline(accessLogDeps(format, &logs), httpserver.HandlerOptions{}, mux).
				ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ok", nil))

			for _, want := range []string{"http.access", "status", "200", "/ok"} {
				if !strings.Contains(logs.String(), want) {
					t.Errorf("access log does not contain %q:\n%s", want, logs.String())
				}
			}
		})
	}
}

func TestPipeline_StreamingHandlerCanFlush(t *testing.T) {
	for _, format := range accessLogFormats {
		t.Run(string(format), func(t *testing.T) {
			deps := accessLogDeps(format, io.Discard)
			mux := http.NewServeMux()
			mux.HandleFunc("GET /stream", func(w http.ResponseWriter, r *http.Request) {
				if _, ok := w.(http.Flusher); !ok {
					t.Error("response writer does not implement http.Flusher")
				}
				_, _ = io.WriteString(w, "chunk-1\n")
				if err := http.NewResponseController(w).Flush(); err != nil {
					t.Errorf("ResponseController.Flush: %v", err)
				}
				_, _ = io.WriteString(w, "chunk-2\n")
			})

			rec := httptest.NewRecorder()
			httpserver.Pipeline(deps, httpserver.HandlerOptions{}, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/stream", nil))

			if !rec.Flushed {
				t.Error("flush did not reach the underlying response writer")
			}
			if got := rec.Body.String(); got != "chunk-1\nchunk-2\n" {
				t.Errorf("body = %q", got)
			}
		})
	}
}

// countingReader counts how many bytes the pipeline pulls from the client.
type countingReader struct {
	r io.Reader
	n int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += n
	return n, err
}

func TestPipeline_RequestBodyLogIsTruncatedAtCap(t *testing.T) {
	payload := strings.Repeat("x", 4*middleware.MaxLogBody)

	for _, format := range accessLogFormats {
		t.Run(string(format), func(t *testing.T) {
			var logs bytes.Buffer
			deps := accessLogDeps(format, &logs)
			mux := http.NewServeMux()
			mux.HandleFunc("POST /ignore-body", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusAccepted)
			})

			body := &countingReader{r: strings.NewReader(payload)}
			req := httptest.NewRequest(http.MethodPost, "/ignore-body", body)
			httpserver.Pipeline(deps, httpserver.HandlerOptions{}, mux).ServeHTTP(httptest.NewRecorder(), req)

			if body.n > middleware.MaxLogBody+1 {
				t.Errorf("access log read %d request bytes, want at most %d", body.n, middleware.MaxLogBody+1)
			}
			logged := logs.String()
			if !strings.Contains(logged, strings.Repeat("x", middleware.MaxLogBody)) {
				t.Errorf("log does not contain the first %d body bytes", middleware.MaxLogBody)
			}
			if strings.Contains(logged, strings.Repeat("x", middleware.MaxLogBody+1)) {
				t.Errorf("log contains more than %d body bytes", middleware.MaxLogBody)
			}
			if !strings.Contains(logged, "[truncated]") {
				t.Errorf("log does not mark the body as truncated:\n%.300s", logged)
			}
		})
	}
}

func TestPipeline_HandlerReceivesFullRequestBody(t *testing.T) {
	payload := strings.Repeat("y", 4*middleware.MaxLogBody)

	for _, format := range accessLogFormats {
		t.Run(string(format), func(t *testing.T) {
			deps := accessLogDeps(format, io.Discard)
			var received string
			mux := http.NewServeMux()
			mux.HandleFunc("POST /echo", func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				received = string(b)
			})

			req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(payload))
			httpserver.Pipeline(deps, httpserver.HandlerOptions{}, mux).ServeHTTP(httptest.NewRecorder(), req)

			if received != payload {
				t.Errorf("handler received %d bytes, want %d", len(received), len(payload))
			}
		})
	}
}

func TestPipeline_PanickingRouteIs500AndLogged(t *testing.T) {
	var logs bytes.Buffer
	deps := httpserver.Deps{Logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("kaboom") })

	rec := httptest.NewRecorder()
	httpserver.Pipeline(deps, httpserver.HandlerOptions{}, mux).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("GET /boom = %d, want 500", rec.Code)
	}
	for _, want := range []string{"http.panic", "kaboom", "/boom"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs do not contain %q:\n%s", want, logs.String())
		}
	}
}
