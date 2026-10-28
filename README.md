# Warcraft Team Follow

A desktop companion for **Warcraft III: Reforged** that gets you and your friends into the same custom games. Pick the friends you play with and Team Follow watches them; turn on **Auto-join** and it follows whoever opens a lobby first, or turn on **Auto-host** and it creates your lobby on your map and waits for them.

It runs on **Windows (x64)**, **macOS (Apple silicon)** and **Linux (x64, with the game under Wine or Proton)**. It's a single program: no installer, runtime or game-file changes.

> Unofficial. Not affiliated with or approved by Blizzard Entertainment.

![Team Follow with Auto-join on, following a group of five friends: one has a lobby open, and the friends list shows who is in Warcraft III, online, away or offline](docs/screenshots/following.png)

Screenshots use sample friends and maps. Colours mean the same thing everywhere: **amber** is a lobby open (and Auto-join or Auto-host on), **violet** is in Warcraft III, **green** is online, **orange** is away, **grey** is offline, and an empty ring is a player who isn't your Battle.net friend, whose status the game doesn't share.

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

The **Friends** tab is your Battle.net friends list, with live status; its badge counts who is online. Search it, show offline friends if you need them, and tick up to 12 people you play with. Your choice is saved on this computer.

The **Recent players** tab lists everyone who was in a lobby or match with you (lobbies you host, join with Auto-join or open by hand; observers included, never computers or you), most recent first, up to 200. Each row shows the full BattleTag, the last map you played together and when you last saw them, which keeps updating while you're still in the lobby or match with them. Status shows "In your lobby", a friend's Battle.net status, or "Not a friend" when the game doesn't say. Tick someone there to add them to the same group as the Friends tab: Auto-host counts them for Same team and Start when full, while Auto-join can only follow Battle.net friends into lobbies. **Clear all** empties the list after you confirm and leaves your group alone. The list is kept on this computer only and never sent anywhere.

![The Recent players tab: two players in your lobby, then earlier players with the map and when you last saw them](docs/screenshots/recent-players.png)

### Blacklist

The **Blacklist** tab keeps players out of every lobby you host. When one of them joins, Team Follow bans them with the game's own Ban, and Activity says who was banned. It works in lobbies Auto-host creates and in ones you create yourself, never in someone else's lobby, and stops once the countdown is locked or the match starts.
- Add a player by BattleTag, remove anyone, and turn **Auto-ban** off for a while without losing the list.
- **Add players I ban to the blacklist** (on by default): ban someone yourself in your lobby and they join the list. This reads the game's English confirmation, so it works only with the game in English.
- In **Recent players**, **Select to blacklist** lets you tick several players and blacklist them together after you confirm. Picking them there doesn't change who you follow; blacklisting someone you follow takes them out of your group.
- Blacklisted players carry a red **Blacklisted** badge. The list is kept on this computer only.

The game's Ban names a slot, not a player, so Team Follow reads everything the game has already sent before banning and checks the game's confirmation; if someone else was banned because the player left at that instant, Activity says so by name.

![The Blacklist tab: Auto-ban and Add players I ban switches, three blacklisted players with the date each was added, and Add by BattleTag](docs/screenshots/blacklist.png)

![Recent players while choosing whom to blacklist: two players ticked with red crosses, one already blacklisted, and the Blacklist 2 players bar](docs/screenshots/recent-blacklist.png)

### Player menu

Right-click any player in Friends, Recent players or Blacklist, or a portrait in your group, to act on them in one step:
- **Copy BattleTag** puts their full BattleTag on the clipboard, ready to paste in the game or a chat.
- **Add to group** or **Remove from group** changes who you follow, like ticking them in the list.
- **Add to blacklist** or **Remove from blacklist**; blacklisting someone you follow takes them out of your group.
- **Add note**, or **Edit note** and **View note** once you have one (see Player notes).

The menu shows only the actions that apply, and a short line confirms each one. From the keyboard, Shift+F10 or the Menu key opens it on the focused row; the arrow keys move, Enter chooses, and Esc closes it. Nothing is sent to the game.

![The player menu on a recent player in your group: Copy BattleTag, Remove from group, and Add to blacklist, which also takes them out of your group](docs/screenshots/player-menu.png)

![The player menu on a blacklisted player, at a narrow window width: Copy BattleTag and Remove from blacklist](docs/screenshots/player-menu-blacklist.png)

### Player notes

Keep a private note about anyone you play with: who carries in Legion TD, who leaves early, whose friend they are. Right-click a player and choose **Add note**; later, **Edit note** or **View note** reads it in full. A note holds up to 500 characters, and clearing the text deletes it. Players with a note show a small note icon in every list, and the search box finds notes too. Notes are kept on this computer only and are never sent to the game.

![Recent players with a note icon after three of the names](docs/screenshots/player-notes.png)

![Editing a note about a recent player: the text, a 61 of 500 character count, and Delete, Cancel and Save](docs/screenshots/note-popover.png)

![A note that's too long at a narrow window width: the count says it is 80 over the 500-character limit and Save is unavailable](docs/screenshots/note-popover-narrow.png)

Team Follow watches them straight away: their portraits show who is online, away or in Warcraft III, and Activity notes when one of them opens a lobby. Watching never joins anything.

![Watching a group of five friends: one has a lobby open, Auto-join is off](docs/screenshots/watching.png)

### Auto-join

Turn on **Auto-join** from any of the game's menus. Team Follow then leaves your team and joins the first friend in your group who opens a lobby, exactly as the game's own Join Friend does. Under it, **Ask for lobby passwords** decides whether Team Follow asks you for a password when a lobby has one, or skips that lobby. **Join the host's team** (on by default) moves you into an open slot on the host's team when you land on the other one. It pauses while you're in a lobby, a match, a score screen or a queue, and resumes as soon as you're back on a menu, including straight from the score screen after a match. Change your group while it's on and it carries on with the new group.

### Lobby passwords

If a friend's lobby has a password, Team Follow asks you for it once. It's sent to the game and never saved.

![The game asks for a lobby password; Team Follow shows a password field](docs/screenshots/password.png)

### Auto-host

For the player who hosts. In **Set up**, choose the map, the lobby name, public or private, map visibility, observers and how long to wait before creating the lobby. Every other option uses the game's own Create Game defaults.

![Auto-host setup: map, lobby name, visibility, delay, observers and the Start when full, Same team and Auto-invite switches](docs/screenshots/host-setup.png)

Turn Auto-host on and Team Follow creates the lobby, then waits for you to press Start in Warcraft III. After each match it creates the next one. Three optional switches:

- **Start when full** starts the game once every slot is taken *and* everyone you follow is in. If someone leaves during the countdown it cancels the start, and starts again once the lobby is full again.
- **Same team** brings everyone you follow onto your team:
  - First, with the host's own team control.
  - Friends who run Team Follow then move themselves into an open slot on your team.
  - Otherwise Team Follow types the game's `!swap` command in the lobby chat, trading the friend with a player on your team who isn't in your group. It never swaps the map's computers, and it stops after three swaps that don't work.
- **Auto-invite** invites the people you follow who are in Warcraft III but not in your lobby, for friends who don't run Team Follow.

**Game mode** types a command such as `-ar` in the match chat a few seconds after the match starts, once per match, for maps like Legion TD or Castle Fight that are set up by chat. Leave the command empty to turn it off.
- It types with keystrokes, and only while Warcraft III is the window in front. It checks before every key and stops if the focus moves.
- On macOS it needs the Accessibility permission, which the setup asks for.
- It works on Windows and macOS; Linux support is still to come.

![A hosted lobby is open: 3 of 6 slots taken, waiting for two friends, moving one to your team](docs/screenshots/auto-host.png)

Auto-host and Auto-join are one at a time: turning one on turns the other off.

### Map browser

**Choose map** opens your Maps folder, found automatically on every system, including Wine and Proton prefixes on Linux. You can always pick it yourself with **Change folder**. Browse folders with the breadcrumb, sort by **Map name** or **Date** by clicking the column headers, or search every folder at once.

![The map browser in the Download folder, sorted by name, with a map selected](docs/screenshots/map-browser.png)

![Searching all maps for "legion": results from every folder, each with the folder it's in](docs/screenshots/map-search.png)

### Setup help

When the system blocks Team Follow from reading the game (Linux and macOS each need one permission step), it says so and shows the exact command, with a Copy button.

![Linux setup: Team Follow shows the setcap command needed to read the game, with a Copy button](docs/screenshots/linux-setup.png)

### Updates

Team Follow checks GitHub for a new version when it starts and every few hours. When one is out, a banner shows the version and what changed since yours, with **Update**, **Later** and **Skip this version**; it warns you first if updating would interrupt a lobby, a match, Auto-join or Auto-host. Nothing is downloaded until you choose Update, which opens the release's download page. The **Updates** section beside Activity shows your version and has **Check for updates** and a **Check automatically** switch.

![An update banner: Team Follow 0.9.0 is available, with Update, Later and Skip this version, and What's new listing the changes since your version](docs/screenshots/update.png)

### Report a problem

**Report a problem** shows a diagnostic report to copy and opens the folder with Team Follow's log files. The report has no names or BattleTags; the logs include BattleTags and lobby names but never passwords or the game's session. See [docs/install.md](docs/install.md#reporting-a-problem).

![Report a problem: a diagnostic report, the log folder's path, and Open log folder and Copy report buttons](docs/screenshots/report.png)

## Privacy and safety

Team Follow talks only to the game's own local menu service on `127.0.0.1`. To connect, it reads the game's local menu session identifier from the game process. It never writes to the game, injects code or looks for account credentials, and the session identifier is never logged, shown or saved.

It sends only an allowlist of commands, each one the game's own menu sends for the same purpose: Join Friend's commands for Auto-join, and Create Game's create button, the host's Start/Cancel and team controls, the friends list's Invite to game and the lobby chat box (for `!swap`) for Auto-host. Nothing starts a match unless the host turned on Start when full. The only keystrokes it ever sends are the Game mode command you entered, into the Warcraft III window while it is in front ([ADR 0007](docs/adr/0007-typing-in-the-match-chat-with-keystrokes.md)).

Checking for updates asks GitHub's public API (`api.github.com`) over HTTPS for the latest stable release and its changelog. It sends nothing about you or the game, and you can turn automatic checks off ([ADR 0009](docs/adr/0009-self-update.md)).

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
