# DTOs

Plain data structs. Interfaces never live here; they go in `port/contract`.

- `follow.go`: what the window sends (`FollowConfig`) and renders (`FollowState` and its parts), the diagnostic report, and the saved `Preferences`.
- `mapupdate.go`: tracked maps and their releases (`TrackedMap`, `MapRelease`, `InstalledMap`, `MapProgress`) and the Maps section's state (`MapUpdatesState`, `MapUpdate`).
- `game.go`: `GameConnection` (from discovery to the dialer) and `GameEvent` (one message from the game).

Do not add subfolders. See `docs/data-contracts.md`.
