# Changelog

What changed in each version of Warcraft Team Follow, newest first. Team Follow shows these entries before you update and again on the first start after an update, so they are written for players.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.9.1] - 2026-10-30

### Added

- Team Follow now records a win or a loss for you and your follow group when a custom game's score screen shows the result. The record is kept on this computer only.
- A **Scoreboard** button in the header opens a table of your group's wins and losses, with games, win rate and when each player last played. Click a column heading to sort by it and again to reverse it. **Clear scoreboard** starts over after you confirm.
- Hovering over a player now also shows how many games they won and lost with you.
- Type **%d** in Auto-host's lobby name to number your games: **Legion TD #%d** opens as **Legion TD #1**, then **Legion TD #2** after the first game starts, and so on. The count starts at 1 each time you turn Auto-host on.

### Fixed

- **Same team** no longer moves you, the host, to another team. Group members are brought to your team, and are only ever swapped with real players, never with computers.

## [0.8.0] - 2026-10-28

### Added

- **Blacklist.** A third tab in the People panel keeps players out of lobbies you host. When a blacklisted player joins a lobby you host, Team Follow bans them with the game's own Ban, whether you created the lobby by hand or with Auto-host.
- Add players by BattleTag in the Blacklist tab, or pick several at once in Recent players with **Select to blacklist**.
- The **Auto-ban** switch in the Blacklist tab turns banning off without clearing your list.
- Before each ban, Team Follow reads every lobby update the game has already sent. It bans only the slot the blacklisted player holds at that moment, and warns you by name if the game reports a different player as banned.
- Players you ban yourself in a lobby you host are added to the blacklist automatically (English game client). A switch turns this off.
- Blacklisted players are marked in Recent players and Friends, and are taken out of your follow group. You can't add them to your follow group until you remove them from the blacklist.
- **Player menu.** Right-click any player name, or press Shift+F10 or the Menu key, to copy their BattleTag, add or remove them from your group, or add or remove them from the blacklist.
- **Player notes.** Write a private note about any player from the player menu with **Add note**, then edit or read it in full later. Notes hold up to 500 characters, are kept on this computer only, and clearing the text deletes one. Players with a note show a note icon, and the search box finds note text too.
- **Player tooltips.** Rest the pointer on a player, or move the keyboard focus to one, to see your note, their status, whether they're in your group, a friend or blacklisted, and your history together: when you first and last saw them, the last map and how many lobbies you've shared. Players recorded before this version start their lobby count at 1.
- **Update check.** Team Follow tells you when a new version is out, with what changed since your version. Choose **Update**, **Later** to hide the banner for now, or **Skip this version**. It warns you first when you're in a lobby or a match, or Auto-join or Auto-host is on.
- **Update** installs the new version for you: it downloads it with progress and a Cancel button, checks it against its published checksum, replaces the app and restarts Team Follow. If anything goes wrong, your version keeps running and the banner says why, with a link to the releases page. On Linux it shows the setcap command to run again afterwards, and on macOS it reminds you that Game mode may need the Accessibility permission again.
- An **Updates** section shows your version, with **Check for updates** and a switch to turn automatic checks off.
- **What's new.** The first time you start a newer version, Team Follow shows what changed since the version you ran before. It works offline. Open it again any time from the Updates section, where the **Changelog** lists every version.

### Changed

- In narrow windows, a long lobby name in Friends or Recent players is shortened so the player's BattleTag stays readable.

## [0.7.0] - 2026-10-28

### Added

- **Recent players.** The People panel has Friends and Recent players tabs. Recent players lists everyone you were in a lobby or match with: their BattleTag, status, the last map you played together and when you last saw them. Tick a player to add them to your follow group, or use **Clear all** to empty the list.
- The Friends tab shows how many friends are online.

## [0.6.1] - 2026-10-28

### Added

- App icon for the Windows, macOS and Linux builds. The Linux download now includes an icon and a `.desktop` entry.

## [0.6.0] - 2026-10-28

### Added

- **Game mode.** Auto-host can type a command such as `-ar` in the match chat a few seconds after the match starts, using keystrokes, only while Warcraft III is the window in front. Windows and macOS; Linux comes later.
- **Same team** can now use the game's `!swap` lobby command to bring a friend onto your team, by trading places with a player who isn't in your group.
- **Join the host's team** (Auto-join): moves you into an open slot on the host's team when you land on the other one.
- A `type` command for checking keystroke typing by hand.

### Changed

- The window is simpler: the Your team card is gone, and Ask for lobby passwords lives inside the Auto-join card.

## [0.5.4] - 2026-10-27

### Added

- **Close all Warcraft III** in the window closes every running copy of the game, after you confirm.

### Fixed

- Auto-join loads the custom game list before joining, so it finds lobbies more reliably.

## [0.5.3] - 2026-10-27

### Added

- An error dialog when the window fails to open, instead of closing silently.

### Fixed

- On Windows, the window's browser data now lives in Team Follow's own folder, not in roaming AppData.
- Auto-join skips lobbies with a password unless you choose to be asked for it, and Auto-host brings you to your group's team.

## [0.5.2] - 2026-10-27

### Fixed

- Auto-host opens lobbies correctly when the game runs under Wine or Proton on Linux.
- Team Follow reconnects after you sign out of Battle.net and back in.

## [0.5.0] - 2026-10-27

### Added

- **Auto-host:** creates your lobby on your chosen map and waits for your group. Start when full, Same team and Auto-invite are optional.
- **Map browser:** finds your Maps folder automatically, including under Wine and Proton, with sorting and a search across folders.
- **Auto-join from any game menu,** including straight from the score screen after a match.
- A friends picker, watching before joining, and log files for reporting problems.
- Warcraft Team Follow rebuilt in Go, with one app for Windows, macOS and Linux.

[Unreleased]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.9.1...HEAD
[0.9.1]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.8.0...v0.9.1
[0.8.0]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.6.1...v0.7.0
[0.6.1]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.5.4...v0.6.0
[0.5.4]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.5.3...v0.5.4
[0.5.3]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.5.2...v0.5.3
[0.5.2]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.5.0...v0.5.2
[0.5.0]: https://github.com/mihaiflorentin/wc3-team-follow/releases/tag/v0.5.0
