package middleware

import "net/http"

// Recorder wraps an http.ResponseWriter and records the status code. It
// forwards Flush and exposes the wrapped writer through Unwrap, so
// http.ResponseController and streaming handlers keep working.
type Recorder struct {
	http.ResponseWriter
	StatusCode  int
	wroteHeader bool
}

func NewRecorder(w http.ResponseWriter) *Recorder {
	return &Recorder{ResponseWriter: w, StatusCode: http.StatusOK}
}

func (r *Recorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.StatusCode = code
		r.wroteHeader = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *Recorder) Write(b []byte) (int, error) {
	r.wroteHeader = true
	return r.ResponseWriter.Write(b)
}

// Flush implements http.Flusher; it is a no-op when the wrapped writer
// cannot flush.
func (r *Recorder) Flush() {
	r.wroteHeader = true
	_ = http.NewResponseController(r.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the wrapped writer.
func (r *Recorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}
