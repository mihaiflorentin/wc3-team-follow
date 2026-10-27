package middleware

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// MaxLogBody caps how many request and response body bytes the access log
// captures.
const MaxLogBody = 4 << 10

const truncatedMarker = "…[truncated]"

// AccessLog logs one "http.access" record per request through logger, with
// the request and response details as typed attributes. The record's look
// (text, colour, JSON) is the logger's format, [logging] server_default.
// Bodies are capped at MaxLogBody; the response body is logged only for
// non-2xx responses.
func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqBody := captureRequestBody(r)

			rec := &bodyRecorder{Recorder: NewRecorder(w)}
			next.ServeHTTP(rec, r)

			logger.InfoContext(r.Context(), "http.access",
				slog.String("method", r.Method),
				slog.String("url", r.URL.String()),
				slog.Int("status", rec.StatusCode),
				slog.String("status_class", statusClass(rec.StatusCode)),
				slog.Duration("duration", time.Since(start)),
				slog.String("remote_address", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
				slog.String("request_body", reqBody),
				slog.String("response_body", rec.errorBody()),
			)
		})
	}
}

// statusClass groups a status code into "1xx" … "5xx", so log queries and
// colour rules can match a whole class.
func statusClass(code int) string {
	if code < 100 || code > 599 {
		return "unknown"
	}
	return strconv.Itoa(code/100) + "xx"
}

// bodyRecorder records the status code (via Recorder, which also forwards
// Flush and Unwrap) and keeps the first MaxLogBody bytes of the response
// body for the access log.
type bodyRecorder struct {
	*Recorder
	body      bytes.Buffer
	truncated bool
}

func (b *bodyRecorder) Write(p []byte) (int, error) {
	n, err := b.Recorder.Write(p)
	keep := min(n, MaxLogBody-b.body.Len())
	b.body.Write(p[:keep])
	if keep < n {
		b.truncated = true
	}
	return n, err
}

// errorBody returns the captured response body for non-2xx responses.
func (b *bodyRecorder) errorBody() string {
	if b.StatusCode < 300 {
		return ""
	}
	if b.truncated {
		return b.body.String() + truncatedMarker
	}
	return b.body.String()
}

// captureRequestBody reads at most MaxLogBody+1 bytes of the request body for
// the access log and puts them back in front of the unread remainder, so the
// handler still receives the full body. A large upload is never buffered.
func captureRequestBody(r *http.Request) string {
	if r.Body == nil || r.Body == http.NoBody {
		return ""
	}
	head, _ := io.ReadAll(io.LimitReader(r.Body, MaxLogBody+1))
	r.Body = struct {
		io.Reader
		io.Closer
	}{io.MultiReader(bytes.NewReader(head), r.Body), r.Body}

	if len(head) > MaxLogBody {
		return string(head[:MaxLogBody]) + truncatedMarker
	}
	return string(head)
}
