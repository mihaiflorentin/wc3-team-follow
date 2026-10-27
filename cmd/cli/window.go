package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mihaiflorentin/wc3-team-follow/cmd/gui"
	"github.com/mihaiflorentin/wc3-team-follow/config"
	"github.com/mihaiflorentin/wc3-team-follow/container"
	"github.com/mihaiflorentin/wc3-team-follow/logging"
)

func newWindowCmd(c *container.ServiceContainer) *cobra.Command {
	return &cobra.Command{
		Long: "Opens the Team Follow window. Closing it stops Follow.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := guiDeps(c)
			if err != nil {
				return fmt.Errorf("window: %w", err)
			}
			opts, err := guiOptions(c.Config())
			if err != nil {
				return fmt.Errorf("window: %w", err)
			}
			return gui.Run(deps, opts)
		},
	}
}

// guiDeps resolves everything the window needs from the container.
func guiDeps(c *container.ServiceContainer) (gui.Deps, error) {
	deps := gui.Deps{Logger: c.Logger()}
	var err error
	if deps.Companion, err = c.Companion(); err != nil {
		return deps, fmt.Errorf("resolve companion: %w", err)
	}
	return deps, nil
}

func guiOptions(cfg *config.Config) (gui.Options, error) {
	w := cfg.Window
	if w == nil {
		return gui.Options{}, errors.New("missing [window] config section")
	}
	exe, _ := os.Executable()
	logDir, webviewDir := "", ""
	if dataDir, err := config.DataDir(cfg.Storage); err == nil {
		webviewDir = filepath.Join(dataDir, "webview")
		if cfg.Logging != nil && cfg.Logging.File {
			logDir = logging.Dir(dataDir)
		}
	}
	return gui.Options{
		Title:      w.Title,
		Width:      w.Width,
		Height:     w.Height,
		MinWidth:   w.MinWidth,
		MinHeight:  w.MinHeight,
		AppVersion: version,
		Executable: exe,
		LogDir:     logDir,
		// Only Windows uses it.
		WebviewDataDir: webviewDir,
	}, nil
}
