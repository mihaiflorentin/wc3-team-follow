# Getting started

This guide covers local development, configuration and builds. Clone https://github.com/mihaiflorentin/wc3-team-follow before making changes.

## Prerequisites

- Go 1.27 or later.
- macOS: Xcode command line tools (`xcode-select --install`).
- Linux: `sudo apt install build-essential libgtk-3-dev libwebkit2gtk-4.1-dev` (or your distribution's equivalents).
- Windows: nothing extra to cross-compile; Edge WebView2 to run.

No Node.js or Wails CLI is needed: the window's files are plain HTML/CSS/JS embedded with `go:embed`, and Wails is used as a Go library.

## Commands

| Command | Purpose |
| --- | --- |
| `make test` / `go test ./...` | Run every unit test. Needs no game and no window. |
| `go vet ./...` | Static checks. |
| `make fmt`, `make lint`, `make tidy` | `gofmt`, `golangci-lint`, `go mod tidy`. |
| `make build` | Build the app for this computer into `bin/wc3-team-follow`. |
| `make run` | Build and open the window. |
| `./bin/wc3-team-follow discover` | Find the running game and print its process, version and menu port. |
| `make package-windows-amd64` | Cross-compile the Windows zip into `dist/` (any OS). |
| `make package-darwin-arm64` | Build and ad-hoc sign `Warcraft Team Follow.app` and its zip (macOS only). |
| `make package-linux-amd64` | Build the Linux tarball (Linux only). |

A plain `go build` without `-tags desktop,production` compiles, but the window then refuses to open; use the Makefile.

## Configuration

`config/config.toml` is embedded into the binary. Every key can be overridden by an environment variable named after its path, upper-cased with `_` (for example `LOGGING_LEVEL=info`).

| Section | Keys |
| --- | --- |
| `[logging]` | `default` (`simple`, `color`, `json`, `pretty-json`), `level` |
| `[follow]` | `verified_builds` (game versions live joining is allowed on), `refresh_interval`, `discovery_interval` |
| `[storage]` | `dir_name`, the per-user data folder name. `WF_DATA_DIR` overrides the whole path, which is useful for isolated runs. |
| `[window]` | `title`, `width`, `height`, `min_width`, `min_height` |

## Working on the window

Serve `cmd/gui/frontend` with any static server and open it in a browser. Outside the app it uses a demo bridge with sample data:

```bash
python3 -m http.server 8000 --directory cmd/gui/frontend
# http://localhost:8000/?scene=live   (off, paused, password, waiting, linux, unverified)
```

## Trying discovery without the game

`infrastructure/discovery` tests run against a fake `/proc` and fake memory. To exercise the Linux path end to end, run any process with `exec -a 'C:\Games\Warcraft III.exe' <program>` that listens on `127.0.0.1`, answers `/webui/index.html` with `GlueManager` and holds `/webui-socket/<digits>` in memory, then run `wc3-team-follow discover`.
