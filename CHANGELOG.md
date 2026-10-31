# Changelog

What changed in each version of Warcraft Team Follow, newest first. Team Follow shows these entries before you update and again on the first start after an update, so they are written for players.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.11.0] - 2026-10-31

### Added

- **Blacklist players who leave Legion TD early**: a new switch in the Blacklist tab, off until you turn it on. After each Legion TD match you play, Team Follow reads the game's replay of it and adds anyone who left before the match's final round began, or before the others voted the game over with -draw or -ff. The rounds come from the replay itself. It never adds you, your Battle.net friends or the people you follow. Each player it adds gets the reason "Left Legion TD early (round 3)", with the round they left in, which is also added to your note about them, and Activity says who was added.
- Blacklisted players now show why they are on your blacklist, under their name and in the player tooltip. Players you ban by hand in your lobby get "You banned them in" and the lobby's name and date.

### Fixed

- Buttons such as Set up, Check now and Choose map no longer look greyed out; they have a clear outline, and only buttons you can't use right now look faded.

## [0.10.0] - 2026-10-31

### Added

- **Keep maps up to date**: Team Follow checks for the newest version of Legion TD (Team OZE) when it starts and every few hours, and downloads it into your Maps folder when it's missing, with a progress bar while it downloads and a message once it's ready. Every download is checked against the checksum the map's authors publish on GitHub, and older versions stay in your Maps folder so you can still join lobbies on them. The new Maps section shows the version you have and the newest one, with Check now and a switch to turn it off.
- **Use the newest installed version**: when Auto-host's map is one Team Follow keeps up to date, such as Legion TD (Team OZE), each lobby uses the newest version in your Maps folder, including one downloaded during your last match. It's on by default; turn it off in Auto-host's Set up to host the exact file you chose. Set up and the Auto-host card show which version will be hosted.

### Changed

- **Greeting** now waits for someone new: Team Follow says hello 3 to 10 seconds after the first player you don't follow joins your lobby, once per lobby. It greets in lobbies you host too, including Auto-host's, and stays quiet when only your followed friends and computers are there.

## [0.9.6] - 2026-10-30

### Fixed

- **Start when full** no longer gets stuck when Warcraft III ignores the first start, for example while a player who just joined is still downloading the map. Team Follow presses Start again every few seconds until the countdown begins, and checks each time that nobody left and every slot is still filled.

## [0.9.5] - 2026-10-30

### Changed

- The **Scoreboard** is hidden for now, along with games won and lost on the player card, because some custom maps report every player as a winner. Team Follow also stops recording results until they can be trusted.

## [0.9.4] - 2026-10-30

### Added

- Hovering over a player in the **Scoreboard** shows the same player card as elsewhere in the window: notes, status, games won and lost, and more.

## [0.9.3] - 2026-10-30

### Added

- The **Scoreboard** has a search box to find a player, and shows 25 players per page.

### Fixed

- The **Scoreboard** now records a win or a loss for every player in your custom games, not only you and your follow group. It keeps up to 2000 players, dropping the ones you played with longest ago.

## [0.9.2] - 2026-10-30

### Changed

- Small fixes and improvements.

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

[Unreleased]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.11.0...HEAD
[0.11.0]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.9.6...v0.10.0
[0.9.6]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.9.5...v0.9.6
[0.9.5]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.9.4...v0.9.5
[0.9.4]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.9.3...v0.9.4
[0.9.3]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.9.2...v0.9.3
[0.9.2]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.9.1...v0.9.2
[0.9.1]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.8.0...v0.9.1
[0.8.0]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.6.1...v0.7.0
[0.6.1]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.5.4...v0.6.0
[0.5.4]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.5.3...v0.5.4
[0.5.3]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.5.2...v0.5.3
[0.5.2]: https://github.com/mihaiflorentin/wc3-team-follow/compare/v0.5.0...v0.5.2
[0.5.0]: https://github.com/mihaiflorentin/wc3-team-follow/releases/tag/v0.5.0
