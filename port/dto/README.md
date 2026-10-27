# DTOs

Plain data structs. Interfaces never live here; they go in `port/contract`.

- `follow.go`: what the window sends (`FollowConfig`) and renders (`FollowState` and its parts), the diagnostic report, and the saved `Preferences`.
- `game.go`: `GameConnection` (from discovery to the dialer) and `GameEvent` (one message from the game).

Do not add subfolders. See `docs/data-contracts.md`.
