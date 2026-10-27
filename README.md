# Warcraft Team Follow

A small desktop companion for **Warcraft III: Reforged** that follows your friends into custom game lobbies. Pick the people you play with (your in-game team, friends you add by BattleTag, or both). When one of them opens a lobby, Team Follow joins it for you. It pauses while you're in a lobby, a match or a queue, and picks up again when you're back in Custom Games.

It runs on **Windows (x64)**, **macOS (Apple silicon)** and **Linux (x64, with the game running under Wine or Proton)**. It's a single program: no installer, runtime or game-file changes.

> Unofficial. Not affiliated with or approved by Blizzard Entertainment.

## Download

Get the file for your system from the [latest release](https://github.com/mihaiflorentin/wc3-team-follow/releases):

| System | File |
| --- | --- |
| Windows 10/11 x64 | `wc3-team-follow-windows-amd64.zip` |
| macOS 13+ on Apple silicon | `wc3-team-follow-macos-arm64.zip` |
| Linux x64 | `wc3-team-follow-linux-amd64.tar.gz` |

The [`edge`](https://github.com/mihaiflorentin/wc3-team-follow/releases/tag/edge) pre-release always holds the latest build from `master`.

Each system needs one setup step; see [docs/install.md](docs/install.md).

## Using it

1. Start Warcraft III through Battle.net. Team Follow connects by itself and shows your team.
2. Add friends by BattleTag (`Thrall#1234`) and/or keep **Follow my team** on. Friends must already be on your Battle.net friends list. Your list is saved on this computer.
3. Choose a mode:
   - **Preview** shows when a friend's lobby opens and never joins anything. Use it first.
   - **Auto-join** leaves your team and joins the friend's open lobby.
4. Open **Custom Games** in the game, then press **Start**. Leave Team Follow running; minimising is fine, and closing it stops Follow.

If a lobby asks for a password, Team Follow asks you for it once and never saves it. **Diagnostic report** gives you a summary you can share for bug reports; it contains no names, BattleTags or session details.

How following decides what to join, and when it pauses, is described in [docs/follow-behaviour.md](docs/follow-behaviour.md).

## Privacy and safety

Team Follow talks only to the game's own local menu service on `127.0.0.1`. To connect, it reads the game's local menu session identifier from the game process. It never writes to the game, injects code, or looks for account credentials. The session identifier stays in memory and is never logged, shown or saved. Commands are limited to an allowlist of six menu commands the game's own Join Friend feature uses.

Live joining is enabled only on game builds that have been checked by hand (`[follow] verified_builds` in `config/config.toml`). Other builds connect in Preview only.

## Development

```bash
make test       # all unit tests
make build      # build the app for this computer into bin/
./bin/wc3-team-follow             # open the window
./bin/wc3-team-follow discover    # check that the running game can be found
```

Building the window needs cgo on macOS (Xcode command line tools) and Linux (`libgtk-3-dev libwebkit2gtk-4.1-dev`); Windows cross-compiles with `make package-windows-amd64`. See [docs/getting-started.md](docs/getting-started.md).

Documentation:

- [AGENTS.md](AGENTS.md): the rules every change keeps.
- [docs/architecture.md](docs/architecture.md): layers, the companion loop and game discovery on each OS.
- [docs/follow-behaviour.md](docs/follow-behaviour.md): the follow state machine.
- [docs/install.md](docs/install.md): per-OS setup for players.
- [docs/getting-started.md](docs/getting-started.md), [docs/container.md](docs/container.md), [docs/data-contracts.md](docs/data-contracts.md), [docs/coding-style.md](docs/coding-style.md), [docs/logging.md](docs/logging.md), [docs/ci.md](docs/ci.md), [docs/adr/](docs/adr/).

## License

MIT. Copyright 2026 Mihai Mihaila.
