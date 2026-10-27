# 3. Game discovery per operating system

Status: accepted (2026-10-27)

## Context

Warcraft III: Reforged serves its menu from a loopback HTTP and websocket service. Connecting needs the port and a menu session id that appears only in the game's memory (`/webui/index.html?guid=<digits>`, `/webui-socket/<digits>`). 0.4 found both on Windows with `GetExtendedTcpTable` and `ReadProcessMemory`. The port must also work with the native macOS game and with the Windows game under Wine or Proton on Linux.

## Decision

- One `contract.GameDiscovery` with an implementation per OS (`platform_windows.go`, `platform_darwin.go`, `platform_linux.go`) and shared, OS-independent parsing and scanning. See the table in `docs/architecture.md`.
- **Windows**: as 0.4. Toolhelp snapshot, the game's own loopback listeners, committed private memory, and the PE version resource.
- **macOS**: the game is ad-hoc signed with `get-task-allow` and no hardened runtime, so `task_for_pid` and `mach_vm_read` can read it. The app is ad-hoc signed with `com.apple.security.cs.debugger`. Users may need `sudo DevToolsSecurity -enable` once. Listeners come from `lsof`, the version from `Info.plist`.
- **Linux (Wine/Proton)**: everything comes from `/proc`. The game is the process whose argv[0], or argv[1] after a `wine*` loader, is `…\Warcraft III.exe`. Listeners come from the game's `net/tcp`, preferring its own sockets and then the same user's, because wineserver may own the socket. Memory is read through `/proc/<pid>/mem`, which needs `CAP_SYS_PTRACE` or `ptrace_scope=0`; the app explains this with the exact `setcap` command. The version is the PE version of the mapped `.exe` or the prefix's `dosdevices` path, read through `/proc/<pid>/root` for Proton's container.
- Every candidate port is confirmed by fetching `/webui/index.html` and finding `GlueManager`. Exactly one game process must be running.
- Discovery's errors are sentences for players. The session is never logged, printed or saved.

## Consequences

- Each OS needs one permission step, documented in `docs/install.md`.
- The Linux and macOS paths were checked against stand-in processes (a fake game in a Linux container, and a `get-task-allow` process on macOS), not yet against a live Reforged client. The `discover` command exists to check that on a real machine.
- Live joining stays gated by `[follow] verified_builds`. The macOS client reports its own version (for example `3.0.0.24268`), which must be added once verified.
