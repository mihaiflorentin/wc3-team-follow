# Warcraft Team Follow

A desktop companion for **Warcraft III: Reforged** that gets you and your friends into the same custom games. Pick the friends you play with and Team Follow watches them; turn on **Auto-join** and it follows whoever opens a lobby first, or turn on **Auto-host** and it creates your lobby on your map and waits for them.

It runs on **Windows (x64)**, **macOS (Apple silicon)** and **Linux (x64, with the game under Wine or Proton)**. It's a single program: no installer, runtime or game-file changes.

> Unofficial. Not affiliated with or approved by Blizzard Entertainment.

![Team Follow with Auto-join on, following a group of five friends: one has a lobby open, and the friends list shows who is in Warcraft III, online, away or offline](docs/screenshots/following.png)

Screenshots use sample friends and maps. Colours mean the same thing everywhere: **amber** is a lobby open (and Auto-join or Auto-host on), **violet** is in Warcraft III, **green** is online, **orange** is away, **grey** is offline.

## Download

Get the file for your system from the [latest release](https://github.com/mihaiflorentin/wc3-team-follow/releases/latest):

| System | File |
| --- | --- |
| Windows 10/11 x64 | `wc3-team-follow-windows-amd64.zip` |
| macOS 13+ on Apple silicon | `wc3-team-follow-macos-arm64.zip` |
| Linux x64 | `wc3-team-follow-linux-amd64.tar.gz` |

The [`edge`](https://github.com/mihaiflorentin/wc3-team-follow/releases/tag/edge) pre-release always holds the latest build from `master`.

Each system needs one setup step so Team Follow may read the game; see [docs/install.md](docs/install.md).

## Features

### Getting started

Start Warcraft III through Battle.net and Team Follow connects by itself. Until everything is ready, a checklist shows what's left: the game running, friends chosen, then Auto-join.

![First run: a checklist shows Warcraft III is running and asks you to choose friends](docs/screenshots/first-run.png)

### Choose your friends and watch them

The **Friends** list is your Battle.net friends list, with live status. Search it, show offline friends if you need them, and tick up to 12 people you play with. Your choice is saved on this computer.

Team Follow watches them straight away: their portraits show who is online, away or in Warcraft III, and Activity notes when one of them opens a lobby. Watching never joins anything.

![Watching a group of five friends: one has a lobby open, Auto-join is off](docs/screenshots/watching.png)

### Auto-join

Turn on **Auto-join** from any of the game's menus. Team Follow then leaves your team and joins the first friend in your group who opens a lobby, exactly as the game's own Join Friend does. Under it, **Ask for lobby passwords** decides whether Team Follow asks you for a password when a lobby has one, or skips that lobby. It pauses while you're in a lobby, a match, a score screen or a queue, and resumes as soon as you're back on a menu, including straight from the score screen after a match. Change your group while it's on and it carries on with the new group.

### Lobby passwords

If a friend's lobby has a password, Team Follow asks you for it once. It's sent to the game and never saved.

![The game asks for a lobby password; Team Follow shows a password field](docs/screenshots/password.png)

### Auto-host

For the player who hosts. In **Set up**, choose the map, the lobby name, public or private, map visibility, observers and how long to wait before creating the lobby. Every other option uses the game's own Create Game defaults.

![Auto-host setup: map, lobby name, visibility, delay, observers and the Start when full, Same team and Auto-invite switches](docs/screenshots/host-setup.png)

Turn Auto-host on and Team Follow creates the lobby, then waits for you to press Start in Warcraft III. After each match it creates the next one. Three optional switches:

- **Start when full** starts the game once every slot is taken *and* everyone you follow is in. If someone leaves during the countdown it cancels the start, and starts again once the lobby is full again.
- **Same team** moves everyone you follow onto your team, with the host's own team control.
- **Auto-invite** invites the people you follow who are in Warcraft III but not in your lobby, for friends who don't run Team Follow.

![A hosted lobby is open: 3 of 6 slots taken, waiting for two friends, moving one to your team](docs/screenshots/auto-host.png)

Auto-host and Auto-join are one at a time: turning one on turns the other off.

### Map browser

**Choose map** opens your Maps folder, found automatically on every system, including Wine and Proton prefixes on Linux. You can always pick it yourself with **Change folder**. Browse folders with the breadcrumb, sort by **Map name** or **Date** by clicking the column headers, or search every folder at once.

![The map browser in the Download folder, sorted by name, with a map selected](docs/screenshots/map-browser.png)

![Searching all maps for "legion": results from every folder, each with the folder it's in](docs/screenshots/map-search.png)

### Setup help

When the system blocks Team Follow from reading the game (Linux and macOS each need one permission step), it says so and shows the exact command, with a Copy button.

![Linux setup: Team Follow shows the setcap command needed to read the game, with a Copy button](docs/screenshots/linux-setup.png)

### Report a problem

**Report a problem** shows a diagnostic report to copy and opens the folder with Team Follow's log files. The report has no names or BattleTags; the logs include BattleTags and lobby names but never passwords or the game's session. See [docs/install.md](docs/install.md#reporting-a-problem).

![Report a problem: a diagnostic report, the log folder's path, and Open log folder and Copy report buttons](docs/screenshots/report.png)

## Privacy and safety

Team Follow talks only to the game's own local menu service on `127.0.0.1`. To connect, it reads the game's local menu session identifier from the game process. It never writes to the game, injects code or looks for account credentials, and the session identifier is never logged, shown or saved.

It sends only an allowlist of commands, each one the game's own menu sends for the same purpose: Join Friend's commands for Auto-join, and Create Game's create button, the host's Start/Cancel and team controls and the friends list's Invite to game for Auto-host. Nothing starts a match unless the host turned on Start when full.

Joining and hosting are enabled only on game builds that have been checked by hand (`[follow] verified_builds` in `config/config.toml`). On other builds Team Follow still watches, but Auto-join and Auto-host stay off.

## Development

```bash
make test       # all unit tests
make build      # build the app for this computer into bin/
./bin/wc3-team-follow             # open the window
./bin/wc3-team-follow discover    # find the running game and the Maps folders
```

Building the window needs cgo on macOS (Xcode command line tools) and Linux (`libgtk-3-dev libwebkit2gtk-4.1-dev`); Windows cross-compiles with `make package-windows-amd64`. Pushing a tag such as `v1.2.3` builds all three and publishes a release. See [docs/getting-started.md](docs/getting-started.md) and [docs/ci.md](docs/ci.md).

Documentation:

- [AGENTS.md](AGENTS.md): the rules every change keeps.
- [docs/architecture.md](docs/architecture.md): layers, the companion loop and game discovery on each OS.
- [docs/follow-behaviour.md](docs/follow-behaviour.md): watching, Auto-join and Auto-host in detail.
- [docs/install.md](docs/install.md): per-OS setup, the Maps folder, and reporting problems.
- [docs/getting-started.md](docs/getting-started.md), [docs/container.md](docs/container.md), [docs/data-contracts.md](docs/data-contracts.md), [docs/coding-style.md](docs/coding-style.md), [docs/logging.md](docs/logging.md), [docs/ci.md](docs/ci.md), [docs/adr/](docs/adr/).

## License

MIT. Copyright 2026 Mihai Mihaila.
