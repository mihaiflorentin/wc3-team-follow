# Architecture

Warcraft Team Follow is a hexagonal (ports and adapters) desktop app. The core (`domain/`) talks to the outside world only through interfaces in `port/contract`. The window drives the core; adapters in `infrastructure/` implement what the core needs. One composition root wires them together (`docs/container.md`, `docs/adr/0001-composition-root-and-constructor-injection.md`).

## Ports and adapters

```
            driving side                core                       driven side
  ┌──────────────────────────┐   ┌─────────────────────┐   ┌──────────────────────────────┐
  │ cmd/gui  (Wails window)  │──▶│ contract.Companion   │──▶│ contract.GameDiscovery        │
  │   frontend/ (HTML/JS)    │   │  domain/companion    │   │   infrastructure/discovery    │
  │ cmd/cli  (commands)      │   │   └ domain/follow    │   │ contract.GameDialer/Session   │
  └──────────────────────────┘   └─────────────────────┘   │   infrastructure/gamesocket   │
                                                           │ contract.PreferenceStore      │
                                                           │   infrastructure/settings     │
                                                           └──────────────────────────────┘
```

| Layer | Package | Role |
| --- | --- | --- |
| Composition root | `main.go`, `container/` | Builds config, logger and container; lazy accessors |
| Driving adapters | `cmd/cli/`, `cmd/gui/` | Commands; the window and its `Bridge` |
| Core | `domain/companion` | Discovery loop, the one game session, saving friends, publishing state |
| | `domain/follow` | The follow state machine (`docs/follow-behaviour.md`) |
| Ports | `port/contract`, `port/dto` | Interfaces and plain data |
| Driven adapters | `infrastructure/discovery` | Finds the game, its menu port and session, per OS |
| | `infrastructure/gamesocket` | Loopback websocket with the command allowlist |
| | `infrastructure/settings` | `friends.json` in the per-user data folder |

## Startup

1. `main.go` loads `config/config.toml` (embedded), builds the logger and the container, and runs the CLI.
2. With no command, `cmd/cli` resolves `contract.Companion` from the container and calls `gui.Run`.
3. `gui.Run` opens the Wails window. On startup it subscribes to state changes (pushed to the page as the `state` event) and starts `Companion.Run`. Closing the window cancels it, which switches Follow off and closes the game session.

The page calls the Go side as `window.go.gui.Bridge.*`: `State`, `Configure`, `SubmitPassword`, `Diagnostics`, `ExecutablePath`. Errors come back as rejected promises whose text is written for players.

## The companion loop

`domain/companion.Service` runs a one-second tick:

- `engine.Tick()` for timeouts and resume;
- every `[follow] refresh_interval`, while connected, `TeamGetInfo` and `FriendsGetFriends`;
- while disconnected, every `[follow] discovery_interval`, discovery and dial in the background.

A reader goroutine feeds each game event to the engine. One mutex serialises the engine; state is published to subscribers after every change. A discovery error becomes `FollowState.ConnectionHint` when it is a sentence for players (discovery's errors start with a capital letter), or a generic hint otherwise.

## Game discovery

Discovery finds exactly one game process, its loopback listener that serves `/webui/index.html` containing `GlueManager`, the menu session (11–20 digits after `/webui/index.html?guid=` or `/webui-socket/`) and the game version. Only the game process is inspected, read-only, with a 12 s and 2 GiB scan budget.

| | Windows | macOS | Linux (Wine/Proton) |
| --- | --- | --- | --- |
| Process | Toolhelp snapshot, `Warcraft III.exe` | `proc_listallpids` + `proc_pidpath`, `…/Warcraft III.app/Contents/MacOS/Warcraft III` | `/proc/*/cmdline` whose argv[0] (or argv[1] after a `wine*` loader) is `…\Warcraft III.exe` |
| Listeners | `GetExtendedTcpTable`, owned by the game | `lsof` for the user's loopback listeners, game first | `/proc/<pid>/net/tcp`: the game's sockets first, then the same user's (wineserver may own them) |
| Session | `VirtualQueryEx` + `ReadProcessMemory` over committed private memory | `task_for_pid` + `mach_vm_read` over private read-write regions (the game has `get-task-allow`) | `/proc/<pid>/maps` + `/proc/<pid>/mem` over anonymous private read-write mappings |
| Version | PE version resource of the image | `CFBundleVersion` in `Info.plist` | PE version of `Warcraft III.exe` via the Wine mapping or the prefix's `dosdevices`, read through `/proc/<pid>/root` for Proton |
| Permission | Same Windows user | Developer tools allowed (`DevToolsSecurity`); app signed with `com.apple.security.cs.debugger` | `CAP_SYS_PTRACE` (`setcap`) or `ptrace_scope=0` |

The OS-specific parts are `platform_<os>.go`. Parsing that doesn't need the OS (`procfs.go`, `lsof.go`, `scan.go`, `peversion.go`) builds everywhere, so its tests run on every CI runner. `procfs` takes its `/proc` root as a field for that reason. See `docs/adr/0003-game-discovery-per-os.md`.

## Window

`cmd/gui/frontend/` is plain HTML, CSS and JavaScript, embedded into the binary. There is no build step or Node.js. Opened outside the app (for example `python3 -m http.server -d cmd/gui/frontend`), the page uses a demo bridge; pick a state with `?scene=off|live|paused|password|waiting|linux|unverified`. See `docs/adr/0002-desktop-window-with-wails.md`.
