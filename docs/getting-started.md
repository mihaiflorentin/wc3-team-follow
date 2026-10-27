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
| `[logging]` | Console: `default` (`simple`, `color`, `json`, `pretty-json`), `level`. Log file: `file`, `file_level`, `file_max_mb`, `file_keep` (see `docs/logging.md`) |
| `[follow]` | `verified_builds` (game versions live joining is allowed on), `refresh_interval`, `discovery_interval` |
| `[storage]` | `dir_name`, the per-user data folder name. `WF_DATA_DIR` overrides the whole path, which is useful for isolated runs. |
| `[window]` | `title`, `width`, `height`, `min_width`, `min_height` |

## Working on the window

Serve `cmd/gui/frontend` with any static server and open it in a browser. Outside the app it uses a demo bridge with sample data:

```bash
python3 -m http.server 8000 --directory cmd/gui/frontend
# http://localhost:8000/?scene=watching   (first, live, paused, password, waiting, many, linux, unverified,
#                                         host-waiting, host-lobby); add &open=host-setup|maps|maps-search|report
```

## Trying discovery without the game

`infrastructure/discovery` tests run against a fake `/proc` and fake memory. To exercise the Linux path end to end, run any process with `exec -a 'C:\Games\Warcraft III.exe' <program>` that listens on `127.0.0.1`, answers `/webui/index.html` with `GlueManager` and holds `/webui-socket/<digits>` in memory, then run `wc3-team-follow discover`.

## Icons

`packaging/icon/` holds the app icon: `icon.svg` (Windows, Linux and the window), `icon-small.svg` (a bolder version for 16–32 px) and `icon-macos.svg` (on Apple's icon grid). `make icons` re-renders `icon.ico`, `icon-512.png`, `AppIcon.icns` and `cmd/gui/icon.png` from them; it needs macOS with `rsvg-convert`, ImageMagick and `iconutil`. The rendered files are committed, so builds need none of these tools.

| OS | Where the icon comes from |
| --- | --- |
| Windows | Resource #3 in the `.exe`, the ID Wails loads for the window and taskbar (`make winres`, `packaging/windows/winres.json`) |
| macOS | `Contents/Resources/AppIcon.icns` in the app bundle, named by `CFBundleIconFile` |
| Linux | Set on the window at runtime (`cmd/gui/icon.png`), plus the PNG and `.desktop` entry in the tarball |

## Screenshots

`docs/screenshots/` holds the README images, one per feature, taken from the demo page at 1100×960, rendered at 2× and scaled to 1400 px wide. Retake them after visible interface changes. On macOS they were rendered with an offscreen `WKWebView` (the engine the macOS app uses) with CSS transitions turned off, since an offscreen window doesn't run them.

| File | Demo URL |
| --- | --- |
| `following.png` | `?scene=live` |
| `watching.png` | `?scene=watching` |
| `first-run.png` | `?scene=first` |
| `auto-host.png` | `?scene=host-lobby` |
| `host-setup.png` | `?scene=watching&open=host-setup` |
| `map-browser.png` | `?scene=watching&open=maps` |
| `map-search.png` | `?scene=watching&open=maps-search` |
| `password.png` | `?scene=password` |
| `linux-setup.png` | `?scene=linux` |
| `report.png` | `?scene=live&open=report` |

The demo page only uses sample friends and maps; keep it that way, since the images are public.
