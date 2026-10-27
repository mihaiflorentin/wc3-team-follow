# 2. Desktop window with Wails

Status: accepted (2026-10-27)

## Context

The 0.4 release was a Windows-only bundle: a WinForms window (C#, .NET Framework 4.8) that started a bundled Node.js helper and talked to it over an authenticated loopback HTTP API, with PowerShell or C# for game discovery. The Go port must run on Windows x64, macOS on Apple silicon and Linux x64 as one program per OS, with a better interface.

Options considered:

- **Fyne or Gio**: pure Go widgets with no web view, but every screen is rebuilt in Go and a custom visual design is harder to achieve.
- **Wails v2 as a library**: the OS's own web view (WebView2, WKWebView, WebKitGTK) renders plain HTML/CSS/JS embedded in the binary, and Go methods are bound directly to the page.
- **A local HTTP server and the default browser**, as 0.4's development mode did: no cgo, but a browser tab is easy to lose and needs the token and same-origin protections 0.4 carried.

## Decision

- Use Wails v2 as a Go library (`github.com/wailsapp/wails/v2`), without its CLI or a Node.js toolchain. The page is plain files under `cmd/gui/frontend/`, embedded with `go:embed`.
- The page calls the companion in-process through `gui.Bridge`; state changes are pushed as a `state` event. There is no local HTTP API, token or port.
- Build with `-tags desktop,production`. macOS and Linux need cgo, so their releases build on native CI runners; Windows needs no cgo and cross-compiles. Linux targets WebKitGTK 4.1 (`webkit2_41`).
- One instance per user (`SingleInstanceLock`); a second launch focuses the existing window.

## Consequences

- One binary per OS, with no bundled runtime. Linux needs `libwebkit2gtk-4.1` installed, which desktop distributions ship.
- The design is CSS: colours, type and layout live in `style.css`. Outside the app the page runs on a demo bridge, so it can be designed in a browser.
- `go test ./...` compiles the Wails packages; on Linux that needs the WebKitGTK development headers, which CI installs.
- Closing the window stops Follow, as in 0.4. There is no tray icon.
