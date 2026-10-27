# 5. Closing the game's processes on request

Status: accepted (2026-10-27)

## Context

Under Proton or Wine, starting Warcraft III again often leaves the old `Warcraft III.exe` running, signed out. Discovery then finds two games and refuses to pick one ("Warcraft III is running 2 times"), so Team Follow can't connect until the player finds and ends the extra process in a task manager or with `kill`. Until now, discovery only read from the game (ADR 0003).

## Decision

- A separate outbound port, `contract.GameProcesses`, with one method: `CloseAll(ctx) (int, error)`. `contract.GameDiscovery` stays read-only.
- It is implemented in `infrastructure/discovery` (`processes.go`) and reuses discovery's own process finding, so it only ever touches what discovery identifies as the game executable: `Warcraft III.exe` (Windows, or argv under Wine/Proton) or the binary inside `Warcraft III.app` (macOS). Battle.net and every other program are out of reach.
- Only the current user's games: the process uid on Linux (`/proc/<pid>/status`) and macOS (`kern.proc.pid`), the process token's user SID on Windows. A process whose owner can't be read is left alone.
- Linux and macOS send SIGTERM, wait up to 3 s, then SIGKILL what is left. Windows calls `TerminateProcess`. Before each signal the pid is checked again to still be the game, so a pid the system reused is left alone.
- It runs only when the player clicks **Close all Warcraft III** (shown in the window when discovery finds more than one copy, and always in **Report a problem**) and confirms. Nothing calls it automatically.
- Discovery's several-copies error implements `contract.GameCounter`, so the companion reports `FollowState.gameCount` without parsing text and without the domain importing the adapter.

## Consequences

- AGENTS.md's safety rules name closing the game as the one action that changes a process, and under which conditions.
- Anything unsaved in the game is lost; the confirmation says so.
- The process selection and the ask-then-force sequence are tested against a fake `/proc` with a fake signal function. Closing real processes on Windows and macOS has not been tested live yet.
