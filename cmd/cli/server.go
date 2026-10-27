package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	httpserver "github.com/mihaiflorentin/wc3-team-follow/cmd/http"
	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/logging"
)

func newServerCmd(c *container.ServiceContainer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Starts the HTTP server",
		Long:  "Starts the HTTP server using the [http] section of config.toml. Flags override the configuration only when set explicitly.",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := serverOptions(c.Config().HTTP, cmd)
			if err != nil {
				return fmt.Errorf("server: %w", err)
			}
			opts.Logger = c.Logger()

			deps, err := httpDeps(c)
			if err != nil {
				return fmt.Errorf("server: %w", err)
			}
			handlerOpts, err := httpHandlerOptions(c.Config(), cmd)
			if err != nil {
				return fmt.Errorf("server: %w", err)
			}

			// Only portable signals: os.Interrupt and SIGTERM exist on every GOOS.
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			// --shutdown-max-requests cancels this context to shut down gracefully.
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			deps.Shutdown = cancel

			serveErr := httpserver.StartServer(ctx, httpserver.NewHandler(deps, handlerOpts), opts)
			if serveErr != nil {
				serveErr = fmt.Errorf("start http server: %w", serveErr)
			}
			// Graceful shutdown: release every resource the container created.
			return errors.Join(serveErr, c.Close())
		},
	}
	addServerFlags(cmd)
	return cmd
}

// httpDeps resolves everything the HTTP layer needs from the container. A
// backend that cannot be reached fails here, before the server listens.
func httpDeps(c *container.ServiceContainer) (httpserver.Deps, error) {
	deps := httpserver.Deps{
		Logger:       c.Logger(),
		AccessLogger: logging.NewAccessLogger(c.Config().Logging, os.Stdout),
		// The container lists the readiness check of every adapter it has
		// created, including ones created after the server started.
		Readiness: c,
	}

	var err error
	if deps.ExampleService, err = c.ExampleService(); err != nil {
		return deps, fmt.Errorf("resolve example service: %w", err)
	}
	return deps, nil
}

// httpHandlerOptions reads the HTTP layer settings from cfg and the flags.
func httpHandlerOptions(cfg *config.Config, cmd *cobra.Command) (httpserver.HandlerOptions, error) {
	var opts httpserver.HandlerOptions
	var err error
	if opts.ReadinessTimeout, err = parseTimeout("http.readiness_timeout", cfg.HTTP.ReadinessTimeout); err != nil {
		return opts, err
	}
	opts.MaxRequests, _ = cmd.Flags().GetUint64("shutdown-max-requests")
	opts.MaxRequestsJitter, _ = cmd.Flags().GetUint64("shutdown-max-requests-jitter")
	return opts, nil
}

func addServerFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	flags.String("host", "", "Overrides http.host from config")
	flags.Int("port", 0, "Overrides http.port from config")
	flags.Duration("read-timeout", 0, "Overrides http.read_timeout from config")
	flags.Duration("write-timeout", 0, "Overrides http.write_timeout from config")
	flags.Duration("idle-timeout", 0, "Overrides http.idle_timeout from config")
	flags.Int("pool", 0, "Sets GOMAXPROCS; when unset the Go runtime default applies")
	flags.String("cert-file", "", "Sets the server certificate file (enables TLS with --key-file)")
	flags.String("key-file", "", "Sets the server key file (enables TLS with --cert-file)")
	flags.Bool("profile", false, "Starts the profiling server on localhost:6060")
	flags.Uint64("shutdown-max-requests", 0, "Gracefully shuts down the server after serving x requests (0 disables it)")
	flags.Uint64("shutdown-max-requests-jitter", 0, "Adds a random 0..x requests to --shutdown-max-requests so replicas do not restart together")
}

// serverOptions resolves the server settings from the [http] config section,
// applying a CLI flag only when it was set explicitly.
func serverOptions(cfg config.HTTPConfig, cmd *cobra.Command) (httpserver.Options, error) {
	opts := httpserver.Options{
		Host: cfg.Host,
		Port: cfg.Port,
	}

	var err error
	if opts.ReadTimeout, err = parseTimeout("http.read_timeout", cfg.ReadTimeout); err != nil {
		return opts, err
	}
	if opts.WriteTimeout, err = parseTimeout("http.write_timeout", cfg.WriteTimeout); err != nil {
		return opts, err
	}
	if opts.IdleTimeout, err = parseTimeout("http.idle_timeout", cfg.IdleTimeout); err != nil {
		return opts, err
	}

	flags := cmd.Flags()
	if flags.Changed("host") {
		opts.Host, _ = flags.GetString("host")
	}
	if flags.Changed("port") {
		opts.Port, _ = flags.GetInt("port")
	}
	if flags.Changed("read-timeout") {
		opts.ReadTimeout, _ = flags.GetDuration("read-timeout")
	}
	if flags.Changed("write-timeout") {
		opts.WriteTimeout, _ = flags.GetDuration("write-timeout")
	}
	if flags.Changed("idle-timeout") {
		opts.IdleTimeout, _ = flags.GetDuration("idle-timeout")
	}
	if flags.Changed("pool") {
		opts.MaxProcs, _ = flags.GetInt("pool")
	}
	opts.CertFile, _ = flags.GetString("cert-file")
	opts.KeyFile, _ = flags.GetString("key-file")
	opts.Profile, _ = flags.GetBool("profile")

	return opts, nil
}

func parseTimeout(key, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, value, err)
	}
	return d, nil
}
