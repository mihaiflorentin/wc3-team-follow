package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	_ "time/tzdata" // embed the timezone database: TZ resolves in images without zoneinfo

	"github.com/mihaiflorentin/wc3-team-follow/cmd/cli"
	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/logging"
)

// main is the composition root: it builds config, logger and container once
// and hands the container to the CLI, the only layer that resolves from it.
// @title             wc3-team-follow
// @version           1.0.0
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.NewConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := logging.New(cfg.Logging, os.Stdout)
	slog.SetDefault(logger)

	c := container.New(cfg, logger)
	return errors.Join(cli.Execute(c), c.Close())
}
