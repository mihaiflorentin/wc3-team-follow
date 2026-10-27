package http

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
	"strconv"
	"time"
)

// Options configures the HTTP server. Zero-valued timeouts fall back to
// sensible defaults; MaxProcs <= 0 leaves GOMAXPROCS to the Go runtime.
type Options struct {
	Host         string
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	MaxProcs     int
	CertFile     string
	KeyFile      string
	Profile      bool
	Logger       *slog.Logger
}

func (o Options) addr() string {
	return net.JoinHostPort(o.Host, strconv.Itoa(o.Port))
}

func durationOr(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}

// StartServer serves handler (built by NewHandler) until ctx is cancelled,
// then shuts down gracefully.
func StartServer(ctx context.Context, handler http.Handler, opts Options) error {
	logger := opts.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	if opts.MaxProcs > 0 {
		runtime.GOMAXPROCS(opts.MaxProcs)
	}

	srv := &http.Server{
		Addr:           opts.addr(),
		Handler:        handler,
		ReadTimeout:    durationOr(opts.ReadTimeout, 20*time.Second),
		WriteTimeout:   durationOr(opts.WriteTimeout, 60*time.Second),
		IdleTimeout:    durationOr(opts.IdleTimeout, 120*time.Second),
		MaxHeaderBytes: 1 << 20,
	}

	if opts.Profile {
		startPprofServer(logger)
	}

	logger.Info("http.server", slog.String("addr", srv.Addr))

	errCh := make(chan error, 1)
	go func() {
		var err error
		if opts.CertFile != "" && opts.KeyFile != "" {
			err = srv.ListenAndServeTLS(opts.CertFile, opts.KeyFile)
		} else {
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return <-errCh
	case err := <-errCh:
		return err
	}
}

func startPprofServer(logger *slog.Logger) {
	logger.Info("http.pprof", slog.String("addr", "http://localhost:6060"))
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", http.HandlerFunc(pprof.Index))
	mux.HandleFunc("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	mux.HandleFunc("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	mux.HandleFunc("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	mux.HandleFunc("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	go func() {
		if err := http.ListenAndServe("localhost:6060", mux); err != nil {
			logger.Error("http.pprof", slog.Any("error", err))
		}
	}()
}
