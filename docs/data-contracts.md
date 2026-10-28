# Data contracts

Ports (`port/contract`) and DTOs (`port/dto`) are the only types that cross layer boundaries.

## Layout

- `port/contract/`: interfaces only, no structs.
  - Inbound: `Companion`, which the window and commands drive.
  - Outbound: `GameDiscovery`, `GameDialer`, `GameSession`, `GameProcesses`, `GameKeyboard`, `MapsLocator`, `MapLibrary` and `PreferenceStore`, which adapters implement and the domain calls.
  - `GameCounter` is implemented by discovery's several-copies error; the companion reads it with `errors.As` to fill `FollowState.GameCount`, so the domain never imports the adapter or parses the message.
- `port/dto/` (`package dto`): plain data between layers. There are no subfolders: the app has no HTTP API.
  - `follow.go`: `FollowConfig` (what the page sends), `FollowState` (what it renders), `TeamMember`, `WatchedMember`, `Friend` (one Battle.net friend in `FollowState.Roster`), `PasswordRequest`, `JoinRecord`, `LogEntry`, `Diagnostics`, `Preferences`.
  - `follow.go` also holds Auto-host's `HostSettings`, `HostConfig` and `HostState`.
  - `game.go`: `GameConnection` (port, session, version, pid), `GameEvent` (one loosely typed message from the game) and `KeyboardStatus`.

## Rules

1. DTOs are plain structs with JSON tags and no behaviour.
2. `FollowState` and `FollowConfig` are the window's contract. Their JSON names are read by `cmd/gui/frontend/app.js`, so a renamed field changes both in the same change.
3. `FollowConfig` uses pointers for optional fields, so "not sent" differs from "false" or an empty tag list.
4. `GameEvent.Payload` keeps the game's own JSON shape as `map[string]any`, with numbers as `json.Number` so account ids keep their precision. `domain/follow/values.go` reads it the way the game's menu code does: a value of the wrong type counts as absent.
5. `Diagnostics.LogFolder` is filled by the window's bridge, so a report says where the logs are.
6. `FollowState.GameCount` (`gameCount`) is how many copies of the game discovery found while not connected, when more than one; otherwise 0. The window shows Close all Warcraft III in its headline when it is above 1.
7. `FollowState.Roster` carries friends' BattleTags and status for the picker. It never carries account ids, and `Diagnostics` never carries the roster.
8. `joinHostTeam` (Join the host's team) is in `FollowConfig`, `FollowState` and `Preferences`. It starts on. `friends.json` stores it only when off (`"joinHostTeam": false`), so a file saved before the option existed loads with it on.
9. Secrets never enter a DTO that leaves the process. `GameConnection.Session` exists only between discovery and the dialer, and the companion clears it. `FollowState` and `Diagnostics` carry no account ids, passwords or session.
10. `HostSettings.gameMode` is the command Game mode types once in a match from Auto-host's lobby (trimmed, one line of at most 100 characters; `""` turns Game mode off) and `gameModeDelaySeconds` how long after the match starts it is typed (kept within 0 and `dto.MaxGameModeDelaySeconds`, 120, which the delay field's `max` in `index.html` matches; 5 by default, also for settings saved before Game mode existed). Both are saved with the other Auto-host settings in `friends.json`.
11. `FollowState.Keyboard` (`keyboard`: `supported`, `permitted`, `reason`) is `GameKeyboard.Status`, refreshed by the companion each second. The window shows it in Auto-host setup and offers Allow typing when it is supported but not permitted; that button calls `Bridge.RequestKeyboardPermission`, the only path to `GameKeyboard.RequestPermission`.
