# 4. Crash dialog and window supervisor

Status: accepted (2026-10-27)

## Context

On Windows the release is a GUI program with no console, and players start it from Explorer, Finder or a desktop menu on every OS. When the window failed, the process ended without a word: `main` printed `error: …` to a console nobody sees, and Wails and its WebView2 library end the process themselves on a web view failure (`os.Exit(1)` in `go-webview2`, `log.Fatal` in Wails' Windows frontend). `recover` and `debug.SetCrashOutput` don't see either; only the log file kept a trace.

## Decision

- Opening the window (no arguments) on Windows and Linux starts a supervisor: the same program runs itself again as the window process, marked with `WF_SUPERVISED=1` (`crashreport.Supervise`). The supervisor passes the child's output through to the console and keeps its last 256 KB.
- When the child stops with a non-zero code, the supervisor picks the line that explains it (`panic:`, `fatal error:` or a crash by signal such as `SIGSEGV:`, `[WebView2 Error]`, the app's `error:` line, or a standard-logger line from `log.Fatal`), falling back to the exit code, and shows a native error dialog naming the log folder: `MessageBoxW` on Windows; zenity, kdialog or xmessage on Linux (the message is always printed too).
- It stays quiet for a stop the player asked for (interrupt, terminate, hang-up, kill), and for an exit with code 1 that printed no reason before the window started (`gui.startup`): Wails on Linux exits that way when a second launch hands over to the open window. Wails runs the startup hook before it checks the lock, so the window first checks the lock's D-Bus name itself and logs `gui.already_open`; an exit with code 1 after that line is quiet too.
- If the window hasn't started within a minute (no `gui.startup`), the supervisor says so, with the usual cause and fix for the OS (the WebView2 Runtime on Windows, WebKitGTK on Linux), then closes the stuck window process. A window whose web view never starts would otherwise sit invisible and hold the one-instance lock, so every later launch handed over to it and exited without a word; that is what a Windows 11 player saw with v0.5.2.
- On Windows the WebView2 profile lives in the app's local data folder (`<data folder>\webview`) instead of WebView2's default in the roaming AppData, which is often redirected to OneDrive or a network share where WebView2 can fail to start.
- Every dialog the supervisor shows is also appended to `crash.log`, so a dialog closed unread still leaves a record.
- Wails' own messages go to the log as `gui.wails` at debug level, release builds included, so a report shows how far the window got.
- The supervisor passes interrupt, terminate and hang-up on to the window process (Linux).
- macOS has no supervisor: starting the bundle's binary again as a child would show a second Dock icon. The window process shows the dialog itself (osascript) when `run` returns an error or the main goroutine panics.
- An update (ADR 0009) ends the window with exit code `RestartCode` (75). The supervisor shows no dialog for it: it starts the program again from the same path, now the new version, as a detached instance with `WF_RESTART_AFTER=<its pid>`, and exits. The new instance waits for that process to exit before it supervises or opens its window, so it never meets the old single-instance lock. If the new version can't be started, the supervisor says so in a dialog. On that first start after an update, a window that crashes within 10 s is started once more, after waiting for the WebView2 data folder to be free, before the crash dialog shows (ADR 0009).
- Commands (`discover`, …) are never supervised: they print to the terminal that started them.

## Consequences

- On Windows and Linux two processes run while the window is open; both are `wc3-team-follow`. Stopping either closes the window.
- A failure inside Wails or WebView2 now reaches the player as a dialog with a reason they can report.
- On macOS, `os.Exit` or `log.Fatal` inside Wails, and panics in background goroutines, still end the window without a dialog; `team-follow.log` and `crash.log` keep the details.
- The supervisor recognises a crash from the child's console output, so the startup marker and the `error:` prefix are part of its contract.
