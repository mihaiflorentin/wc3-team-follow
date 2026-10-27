# Warcraft Team Follow

A small desktop companion for **Warcraft III: Reforged** that follows your friends into custom game lobbies. Pick the people you play with from your Battle.net friends (and, if you like, your in-game team). Team Follow watches them and shows when one opens a lobby; turn on **Auto-join** and it joins the first one for you. It pauses while you're in a lobby, a match or a queue, and picks up again when you're back in Custom Games.

It runs on **Windows (x64)**, **macOS (Apple silicon)** and **Linux (x64, with the game running under Wine or Proton)**. It's a single program: no installer, runtime or game-file changes.

> Unofficial. Not affiliated with or approved by Blizzard Entertainment.

![Team Follow with Auto-join on, following a group of five friends: one has a lobby open, and the friends list shows who is in Warcraft III, online, away or offline](docs/screenshots/following.png)

<table>
<tr>
<td width="50%"><img src="docs/screenshots/first-run.png" alt="First run: a checklist shows the game is connected and asks you to choose friends"></td>
<td width="50%"><img src="docs/screenshots/watching.png" alt="Watching a group of five friends: one has a lobby open, and the Auto-join switch is off"></td>
</tr>
<tr>
<td>First run: a checklist shows what's left to do.</td>
<td>Watching: your group's status at a glance, before turning on Auto-join.</td>
</tr>
</table>

Screenshots use sample friends. Colours mean the same thing everywhere: amber is a lobby open (and auto-join on), violet is in Warcraft III, green is online, orange is away, grey is offline.

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
2. Tick the friends you play with in the **Friends** list, which shows your Battle.net friends with live status. You can also keep **Follow my team** on to include your in-game team. Your choice is saved on this computer.
3. Team Follow now **watches** them: their portraits show who is online or in Warcraft III, and Activity notes when one of them opens a lobby. Watching never joins anything.
4. To be taken into those lobbies, turn on **Auto-join** while you're on any of the game's menus (main menu, campaigns, profile, Custom Games, and so on). Team Follow then leaves your team and joins the first of them to open a lobby. It pauses while you're in a lobby, a match, a score screen or a queue, and resumes as soon as you're back on a menu. Leave Team Follow running; minimising is fine, and closing it turns Auto-join off.

**Auto-host** is for the player who hosts. In **Set up**, choose a map from your Maps folder (found automatically; you can pick it yourself too), a lobby name, public or private, map visibility, observers and how long to wait. Turn Auto-host on and Team Follow creates the lobby, then waits for you to press Start in Warcraft III. After each match it creates the next one. Optional switches:

- **Start when full**: starts once every slot is taken and everyone you follow is in, and cancels the start if someone leaves before the countdown locks.
- **Same team**: moves everyone you follow onto your team.
- **Auto-invite**: invites people you follow who are in Warcraft III but not in your lobby, for friends without Team Follow.

Auto-host and Auto-join are one at a time: turning one on turns the other off.

If a lobby asks for a password, Team Follow asks you for it once and never saves it. If something goes wrong, **Report a problem** gives you a report to copy and opens the folder with Team Follow's log files; send both (see [docs/install.md](docs/install.md#reporting-a-problem)).

How following decides what to join, and when it pauses, is described in [docs/follow-behaviour.md](docs/follow-behaviour.md).

## Privacy and safety

Team Follow talks only to the game's own local menu service on `127.0.0.1`. To connect, it reads the game's local menu session identifier from the game process. It never writes to the game, injects code, or looks for account credentials. The session identifier stays in memory and is never logged, shown or saved. Commands are limited to an allowlist of six menu commands the game's own Join Friend feature uses.

Live joining is enabled only on game builds that have been checked by hand (`[follow] verified_builds` in `config/config.toml`). On other builds Team Follow still watches, but Auto-join stays off.

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
