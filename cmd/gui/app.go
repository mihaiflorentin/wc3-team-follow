// Package gui is the driving desktop adapter: a Wails window whose page
// drives the companion through the Bridge.
package gui

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

//go:embed frontend
var frontend embed.FS

// instanceID names the single-instance lock.
const instanceID = "a6c1d2a4-wc3-team-follow"

// StateEvent is the event the page listens to for new state.
const StateEvent = "state"

// Deps are the collaborators the window needs.
type Deps struct {
	Companion contract.Companion
	Logger    *slog.Logger
}

// Options are the window settings.
type Options struct {
	Title     string
	Width     int
	Height    int
	MinWidth  int
	MinHeight int
	// AppVersion is shown in diagnostics.
	AppVersion string
	// Executable is this program's path, shown in setup instructions.
	Executable string
	// LogDir is the log folder, or "" when file logging is off.
	LogDir string
	// WebviewDataDir holds the web view's profile on Windows, or "" for
	// WebView2's default (a folder in the roaming AppData).
	WebviewDataDir string
}

// Run opens the window and blocks until it is closed. The companion runs
// for exactly as long as the window is open.
func Run(deps Deps, opts Options) error {
	assets, err := fs.Sub(frontend, "frontend")
	if err != nil {
		return fmt.Errorf("load interface: %w", err)
	}
	bridge := NewBridge(deps.Companion, opts.AppVersion, opts.Executable, opts.LogDir)

	var (
		wg     sync.WaitGroup
		cancel context.CancelFunc
		stop   func()
	)
	startup := func(ctx context.Context) {
		bridge.ctx = ctx
		// The crash reporter reads this to tell a window that never started.
		deps.Logger.Info("gui.startup")
		stop = deps.Companion.Subscribe(func(state dtoState) { wailsruntime.EventsEmit(ctx, StateEvent, state) })
		var runCtx context.Context
		runCtx, cancel = context.WithCancel(ctx)
		wg.Go(func() {
			if err := deps.Companion.Run(runCtx); err != nil {
				deps.Logger.Error("gui.companion", slog.Any("error", err))
			}
		})
	}
	shutdown := func(context.Context) {
		if stop != nil {
			stop()
		}
		if cancel != nil {
			cancel()
		}
		wg.Wait()
	}

	if alreadyOpen(instanceID) {
		// The crash reporter reads this to tell a second launch from a crash.
		deps.Logger.Info("gui.already_open")
	}
	err = wails.Run(&options.App{
		Title:            opts.Title,
		Width:            opts.Width,
		Height:           opts.Height,
		MinWidth:         opts.MinWidth,
		MinHeight:        opts.MinHeight,
		BackgroundColour: &options.RGBA{R: 0x1d, G: 0x2b, B: 0x2a, A: 0xff},
		// Wails' own messages go to the log file at debug level, release
		// builds included, so a window that never opens can be diagnosed.
		Logger:             wailsLog{deps.Logger},
		LogLevel:           logger.DEBUG,
		LogLevelProduction: logger.DEBUG,
		AssetServer:        &assetserver.Options{Assets: assets},
		OnStartup:          startup,
		OnShutdown:         shutdown,
		Bind:               []any{bridge},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: instanceID,
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				if bridge.ctx != nil {
					wailsruntime.WindowUnminimise(bridge.ctx)
					wailsruntime.WindowShow(bridge.ctx)
				}
			},
		},
		Mac: &mac.Options{
			Appearance: mac.NSAppearanceNameDarkAqua,
			About:      &mac.AboutInfo{Title: opts.Title, Message: "Version " + opts.AppVersion},
		},
		// The WebView2 profile lives in the app's local data folder: the
		// default, roaming AppData, is often redirected to OneDrive or a
		// network share, where WebView2 can fail to start.
		Windows: &windows.Options{Theme: windows.Dark, WebviewUserDataPath: opts.WebviewDataDir},
	})
	if err != nil {
		return fmt.Errorf("run window: %w", err)
	}
	return nil
}
