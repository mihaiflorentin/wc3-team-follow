// Package discovery finds the running Warcraft III client and its local
// menu service: the loopback port it serves the menu on and the menu
// session identifier.
//
// It only reads. It never writes game memory, injects code or looks for
// account credentials, and it inspects no process other than the game.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
	"github.com/mihaiflorentin/wc3-team-follow/port/dto"
)

// GameExe is the game executable's file name on Windows and under Wine.
const GameExe = "Warcraft III.exe"

const (
	probeTimeout = 2 * time.Second
	probeLimit   = 1 << 20
)

var (
	// ErrNoGame: no game client is running.
	ErrNoGame = errors.New("Warcraft III isn't running. Start it through Battle.net.")
	// ErrManyGames: more than one client is running; which one to follow is ambiguous.
	ErrManyGames = errors.New("More than one Warcraft III is running. Close the extra ones.")
	// ErrNoMenu: the game runs but its menu service is not up yet.
	ErrNoMenu = errors.New("Warcraft III is starting. Waiting for its menus to open.")
	// ErrNoSession: the menu session is not available yet.
	ErrNoSession = errors.New("Waiting for Warcraft III's menus to finish loading.")
	// ErrAccess: the operating system refused to let the app read the game.
	ErrAccess = errors.New("This computer blocked reading Warcraft III's menu session.")
)

var sessionPattern = regexp.MustCompile(`^\d{11,20}$`)

// game identifies the one running client.
type game struct {
	pid int
	// exe is the executable's path on this machine, when known.
	exe string
	// uid owns the process (Linux only).
	uid int
}

// platform is the operating-system specific part of discovery.
type platform interface {
	findGame(ctx context.Context) (game, error)
	// listeners returns loopback ports that may be the menu service, most
	// likely first.
	listeners(ctx context.Context, g game) ([]int, error)
	session(ctx context.Context, g game) (string, error)
	version(ctx context.Context, g game) string
}

// Discovery implements contract.GameDiscovery.
type Discovery struct {
	platform platform
	client   *http.Client
	logger   *slog.Logger
}

// New returns the discovery for this operating system.
func New(logger *slog.Logger) contract.GameDiscovery {
	return newDiscovery(newPlatform(), logger)
}

func newDiscovery(p platform, logger *slog.Logger) *Discovery {
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	return &Discovery{
		platform: p,
		client:   &http.Client{Timeout: probeTimeout, Transport: transport},
		logger:   logger,
	}
}

// Discover finds the game's menu port and session. The session is returned
// to the caller only; it is never logged.
func (d *Discovery) Discover(ctx context.Context) (dto.GameConnection, error) {
	g, err := d.platform.findGame(ctx)
	if err != nil {
		d.logger.DebugContext(ctx, "discovery.process", slog.Any("error", err))
		return dto.GameConnection{}, err
	}
	d.logger.DebugContext(ctx, "discovery.process", slog.Int("pid", g.pid), slog.String("exe", g.exe), slog.Int("uid", g.uid))
	ports, err := d.platform.listeners(ctx, g)
	if err != nil {
		return dto.GameConnection{}, fmt.Errorf("list game listeners: %w", err)
	}
	d.logger.DebugContext(ctx, "discovery.listeners", slog.Any("ports", ports))
	port := 0
	for _, p := range ports {
		if d.isMenu(ctx, p) {
			port = p
			break
		}
	}
	if port == 0 {
		return dto.GameConnection{}, ErrNoMenu
	}
	started := time.Now()
	session, err := d.platform.session(ctx, g)
	if err != nil {
		d.logger.DebugContext(ctx, "discovery.session", slog.Any("error", err), slog.String("took", time.Since(started).Round(time.Millisecond).String()))
		return dto.GameConnection{}, err
	}
	// The session itself is never logged, only that one was found.
	d.logger.DebugContext(ctx, "discovery.session", slog.Int("digits", len(session)), slog.String("took", time.Since(started).Round(time.Millisecond).String()))
	if !sessionPattern.MatchString(session) {
		return dto.GameConnection{}, ErrNoSession
	}
	version := d.platform.version(ctx, g)
	d.logger.DebugContext(ctx, "discovery.found", slog.Int("pid", g.pid), slog.Int("port", port), slog.String("version", version))
	return dto.GameConnection{Port: port, Session: session, Version: version, PID: g.pid}, nil
}

// isMenu reports whether port serves the game's menu page.
func (d *Discovery) isMenu(ctx context.Context, port int) bool {
	if port < 1024 || port > 65535 {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/webui/index.html", port), nil)
	if err != nil {
		return false
	}
	resp, err := d.client.Do(req)
	if err != nil {
		d.logger.DebugContext(ctx, "discovery.probe", slog.Int("port", port), slog.Any("error", err))
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, probeLimit))
	menu := err == nil && strings.Contains(strings.ToLower(string(body)), "gluemanager")
	d.logger.DebugContext(ctx, "discovery.probe", slog.Int("port", port), slog.Int("status", resp.StatusCode), slog.Bool("menu", menu))
	return menu
}

// isGameExe reports whether path names the game executable, whichever
// separator it uses.
func isGameExe(path string) bool {
	return strings.EqualFold(baseName(path), GameExe)
}

func baseName(path string) string {
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		return path[i+1:]
	}
	return path
}
