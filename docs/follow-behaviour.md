# Follow behaviour

`domain/follow` is the state machine that decides when to join a friend's lobby. It is a port of the 0.4 release's `team-follow.cjs`, and `domain/follow/engine_test.go` carries over its behaviour tests. Change the behaviour there, and keep this page in step.

## Inputs

The engine reads events from the game's menu service (`dto.GameEvent`) and sends commands back through a `Sender`. The companion refreshes `TeamGetInfo` and `FriendsGetFriends` every `[follow] refresh_interval`; the engine never asks for anything else in the background.

Commands the app may send, enforced by `infrastructure/gamesocket`:

| Command | When |
| --- | --- |
| `TeamGetInfo`, `FriendsGetFriends` | Background refresh |
| `RequestCancelTeamInvitations`, `TeamLeave`, `JoinFriendGameLobby { auroraId }` | One join attempt, in this order, as the game's own Join Friend does |
| `SendGameListing`, `GetGameList` | Before answering the lobby confirmation: loads the custom game list that joining by name searches (without `SendGameListing` the list stays empty, checked live) |
| `JoinGameByGameName { gameName, gamePass }` | Answering the game's lobby confirmation or password request |
| `BanPlayerFromGameLobby { slot }` | Auto-ban: a blacklisted player in a lobby the player hosts (see Blacklist and auto-ban) |

`SendGameLobbySetup` and `GetGameInfo` closed the game during development and must never be added. `KickPlayerFromGameLobby` isn't allowed either.

## The friends roster

Every `FriendsFriendData` snapshot replaces the roster: the whole Battle.net friends list, shown in the window's picker with each friend's status (`Lobby open`, `In Warcraft III`, `Online in Battle.net`, `Away`, `Offline`). The roster is display-only. It survives starting Follow (which empties the presence the engine acts on) and is cleared on disconnect. Ticking a friend adds their BattleTag to the follow group (`manualTags`, up to 12).

## Recent players

`domain/follow/recent.go` (`follow.Recent`), driven by the companion with the events it already receives; it only reads them and never sends anything to the game.

- **Who:** each `GameLobbySetup` while the player is in a lobby (hosted with Auto-host, joined with Auto-join, or created or joined by hand) lists the other humans present: slots with `slotType` 0 (human) and `slotStatus` 2 (occupied), observers included, and never the player's own slot (`isSelf`) or a slot with the player's own name. Computers, open and closed slots, and names that aren't BattleTags (`name` is the full BattleTag on Battle.net, checked live) are left out.
- **Map:** `mapData.mapName` without the game's colour codes, else the base name of `mapData.mapPath` without its folder and extension, else `lobbyName`.
- **Last seen:** each lobby update stamps everyone in it with the companion's clock, and the companion's once-a-second tick refreshes them, so last seen is the last moment the two were together. Lobby updates stop once the match loads, so the last lobby's players stay present through the match. A player who leaves the lobby drops out of the next update and is no longer refreshed. It ends (one last stamp) on a score screen, a menu screen, `MultiplayerGameLeave`, `ExitedGame`, `LoggedOut`, a disconnect, Close all Warcraft III, or the app stopping.
- **First seen and lobbies shared:** a new entry starts with first seen = last seen and 1 lobby; first seen never moves after that. The lobby count grows by one when a lobby starts with the player in it or they join it, never on later updates of the same lobby, so leaving and rejoining the same lobby counts once. A lobby ends where last seen ends (above), or when an update names a different `lobbyName`. Lists saved before these existed load with first seen = last seen and 1 lobby.
- **Logging:** `ranking.recorded` (Info) once per recorded match with `wins`, `losses` and `source` (`flags`: each player's own flag; `local-only`: unfinished match; `lobby`: older shape), and `ranking.skipped` with a short `reason` when a score records nothing or leaves players out (not armed, replay, not a custom game, unfinished with others unknown, inconsistent flags, no lobby). BattleTags, account ids and payloads are never logged.
- **List:** one entry per BattleTag (matched case-insensitively), newest first, at most `dto.MaxRecentPlayers` (200). `FollowState.recent` carries it and `FollowState.lobbyTags` names who is present now; the window works out the status from those and the friends roster.
- **Saving:** `contract.RecentPlayers` (`recent-players.json`, written atomically next to `friends.json`). The companion saves at most every 5 seconds after a player or map was added or changed, at most once a minute when only last-seen times moved (each second in a match), and on shutdown. A failed save is logged and tried again. A missing or unreadable file loads as an empty list. Only `recent.saved` with a count is logged; no names.
- **Clear all** (`Companion.ClearRecentPlayers`) removes the file and the list, including who is present now (the next lobby update records them again). It doesn't touch the follow group.
- **Follow group:** ticking a recent player adds their BattleTag to `manualTags`, like Add by BattleTag; the 12-member limit is unchanged. Auto-join refuses to start with a group member who isn't a Battle.net friend (see Starting); Auto-host counts them for Same team and Start when full.

## Player menu

The window only: `cmd/gui/frontend/app.js` (`playerMenuGroups`, `openPlayerMenu`). Nothing is sent to the game and no bridge method is new.

- **Where:** every player row in Friends, Recent players and Blacklist, and every chip in the group bar. Right-click, Shift+F10 or the Menu key on the focused row opens it; a chip also opens it on click. Unavailable checkboxes (blacklisted, group full) are `aria-disabled` rather than disabled, so their rows stay reachable from the keyboard.
- **Items, only those that apply:** Copy BattleTag (Wails `ClipboardSetText`, else `navigator.clipboard`); Remove from group for a `manualTags` member, else Add to group (`Configure` with the new `tags`, through the same path as ticking, so Auto-join is turned back on for the new group as before), unavailable with "Your group is full (12)" when full; Add to blacklist (`AddToBlacklist([tag])`, which also takes them out of the group; the item says so when they are in it) or Remove from blacklist (`RemoveFromBlacklist`). Members followed through the game team get no group item, blacklisted players no Add to group, and the player's own tag neither group nor blacklist items.
- **Keyboard and focus:** ARIA `menu`/`menuitem`/`separator`, focus on the first item; the arrow keys (wrapping), Home, End and a first letter move; Enter or Space chooses; Esc or Tab closes. Focus returns to the row, found again by BattleTag after the lists redraw, or to its neighbour when the row left the list. A pointer press outside, scrolling, resizing or leaving the window closes it.
- **Placement:** fixed, below and right of the pointer or row, flipped above or left when there's no room, and kept 8 px inside the window; it scrolls when the window is shorter than the menu.
- **Confirmation:** a short line at the bottom ("Copied Player#1234", "Blacklisted Player#1234") and a polite status region for screen readers.
- **Groups:** items come in groups split by a separator: the player's note first (see Player notes), then Copy BattleTag and the group item, then the blacklist item.

## Player notes

A private note per BattleTag, kept on this computer only and never sent to the game, logged or put in `Diagnostics`.

- **Rules:** `domain/follow/notes.go`. `NoteKey` is the lower-cased, trimmed BattleTag; `NoteText` trims the text and refuses a note longer than `dto.MaxNoteLength` (500) characters (Unicode characters, not bytes) whole, never cutting it short ("A note can be at most 500 characters."). Empty text deletes the note.
- **Saving:** `Companion.SetNote(tag, text)` is the only writer. It saves the whole map through `contract.PlayerNotes` (`notes.json`, written atomically next to `friends.json`) and only then changes the state, so a failed save keeps the old note and returns "Your note couldn't be saved on this computer. Try again.". It logs `notes.saved` with a count, never a BattleTag or the text. A missing, unreadable or corrupt file loads as no notes; loading also lower-cases keys, trims text, cuts anything over 500 characters and drops empty notes.
- **Bridge:** `SetNote(tag, text)` and `DeleteNote(tag)` (which sends empty text).
- **Menu:** Add note when the player has none; Edit note and View note when they do. Every player row and chip offers them, the player's own included.
- **Popover:** `cmd/gui/frontend/app.js` (`openNote`). One fixed popover (`role="dialog"`), placed like the menu next to the row. Writing: a textarea, a count ("61 / 500", amber within 50 of the limit, red with "N over the 500-character limit" past it, when Save is unavailable and screen readers hear the limit once), and Save, Cancel and Delete (Delete only for an existing note). Ctrl+Enter or Cmd+Enter saves; saving empty text deletes. Errors stay in the popover so nothing typed is lost. Reading: the note as plain text with its line breaks (`textContent`, never HTML), when it was updated, and Edit note and Close. Esc closes and focus returns to the row; Tab stays inside. A click outside closes it unless the text was changed.
- **Lists:** rows in Friends, Recent players and Blacklist show a small note icon ("Has a note") after the name. The search box matches note text as well as BattleTags (and maps in Recent players).

## Player tooltip

The window only: `cmd/gui/frontend/app.js` (`playerFacts`, `showTip`). One shared tooltip (`role="tooltip"`) for every player row and group chip; nothing is sent to the game and no bridge method is new.

- **Content, in order, each part only when known:** the note (plain text, line breaks kept, at most 9 lines); the full BattleTag and status ("In your lobby", the friend's Battle.net status with an open lobby's name, or "Not a friend" once the roster has loaded); standing ("In your follow group", "In your follow group, from your game team", "Battle.net friend", "Blacklisted since 3 Oct"); history from the recent player entry (first seen, last seen, last map, lobbies shared).
- **Showing:** on hover after 500 ms; while one is up, or within 300 ms of one closing, the next row's shows at once. On keyboard focus (`:focus-visible`) at once, and the focused control gets `aria-describedby="player-tip"` while it shows. The pointer can move onto it; it hides 150 ms after the pointer leaves both, on focus leaving, Esc (which goes no further and leaves focus where it is; the next Esc reaches what's behind), a pointer press, resizing, leaving the window, or scrolling its row away (a focus tooltip moves with its row instead).
- **Placement:** fixed, below the row or chip, above when there's no room, 8 px inside the window. It never shows while the player menu or note popover is open, and opening either hides it, so it never covers them. The rows' own `title` tooltips are gone, so only this one shows.

## Ranking (the window's Scoreboard)

`domain/follow/ranking.go` (`follow.Ranking`), driven by the companion with the events it already receives; it only reads them and never sends anything to the game. The window shows it in the Ranking dialog (`cmd/gui/frontend`): a sortable table (Player, Games, Wins, Losses, Win rate, Last played), default win rate then games, the player's own row marked from `FollowState.selfTag`, and Clear ranking after a confirmation.

- **Observed event order** (live capture, game 3.0.1.24371, Legion TD, the host left the match early and lost): `GameLobbyGracefulExit`, `SetGlueScreen LOADING_SCREEN` (the match starts); at the end `ExitedGame`, `UpdateScoreInfo`, `SetGlueScreen SCORE_SCREEN`, `MultiplayerGameLeave`, then a second identical `UpdateScoreInfo`. The score arrives after the player leaves the game. In that capture every player, the other team's computers included, had `won: false`, because the match wasn't finished. A won match against computers (1100 s) had the same order, with `localPlayerWon: true` and `won: true` for the host; every computer, the host's teammates included, had `won: false`, so computers' flags are never used. Real players' flags are what the recorder trusts.
- **Lobby snapshot:** each `GameLobbySetup` replaces it with the occupied seats of players who take part: humans and computers (computers only to count teams), with their `team`. Observers are left out; if the local player (`isSelf`) is an observer, the match records nothing. The snapshot only identifies the local player and is the fallback data.
- **Armed:** `SetGlueScreen LOADING_SCREEN` arms a match. The first `UpdateScoreInfo` with `scoreInfo.localPlayerWon` as a boolean while armed records once and disarms, so the duplicate counts nothing. `ExitedGame`, `MultiplayerGameLeave` and `SCORE_SCREEN` don't disarm, because they come before the score. A campaign score (`payload.data`) or a malformed one is ignored and keeps the match armed.
- **No result:** a menu screen (not a score screen), a new lobby (`GameLobbySetup`), `LoggedOut`, a disconnect, closing the game and Clear disarm without recording. A score that isn't armed, a replay (`mapInfo.isReplay`) or a `mapInfo.gameType` other than `custom` (ladder, ranked, campaign) records nothing. Only custom-map games count; a melee game hosted from Custom Games reports `custom` too and is recorded.
- **Who:** from `scoreInfo.players`, every human (`isAi` false, non-empty `battleTag`): the local player (the lobby's own seat) and everyone else in the match, followed or not. Computers are never recorded. A player is counted once; tags match case-insensitively and the entry keeps the latest casing.
- **Win or loss, finished match** (some player has `won: true`): each recorded player's result is their own `won` flag. Flags that contradict themselves record nothing: humans on two or more teams with `won: true`, or the local player's flag disagreeing with `localPlayerWon`. A live 4v4 Legion TD the local player lost reported `won: true` for all 8 humans.
- **Win or loss, unfinished match** (no `won: true`: the local player left, or a draw): only what is certain. The local player gets a loss when `localPlayerWon` is false. If `localPlayerWon` is true, the local player and their teammates (same `teamIndex`) get a win. Nobody else is recorded, because their outcome is unknown.
- **Older shape:** if `players` is missing, has no `won` fields or names no human, the lobby snapshot decides. The local player's team gets the local result. A group member on another team gets the opposite when that is certain: the player won (they lost), or the match had exactly two teams, computers included (they won). Otherwise, such as a free-for-all the player lost, they get nothing. A seat without a team number is skipped.
- **List:** one entry per BattleTag (matched case-insensitively), newest first, at most `dto.MaxRecentPlayers` (200). `FollowState.recent` carries it and `FollowState.lobbyTags` names who is present now; the window works out the status from those and the friends roster.
- **Saving:** `contract.RecentPlayers` (`recent-players.json`, written atomically next to `friends.json`). The companion saves at most every 5 seconds after a player or map was added or changed, at most once a minute when only last-seen times moved (each second in a match), and on shutdown. A failed save is logged and tried again. A missing or unreadable file loads as an empty list. Only `recent.saved` with a count is logged; no names.
- **Clear all** (`Companion.ClearRecentPlayers`) removes the file and the list, including who is present now (the next lobby update records them again). It doesn't touch the follow group.
- **Follow group:** ticking a recent player adds their BattleTag to `manualTags`, like Add by BattleTag; the 12-member limit is unchanged. Auto-join refuses to start with a group member who isn't a Battle.net friend (see Starting); Auto-host counts them for Same team and Start when full.

## Player menu

The window only: `cmd/gui/frontend/app.js` (`playerMenuGroups`, `openPlayerMenu`). Nothing is sent to the game and no bridge method is new.

- **Where:** every player row in Friends, Recent players and Blacklist, and every chip in the group bar. Right-click, Shift+F10 or the Menu key on the focused row opens it; a chip also opens it on click. Unavailable checkboxes (blacklisted, group full) are `aria-disabled` rather than disabled, so their rows stay reachable from the keyboard.
- **Items, only those that apply:** Copy BattleTag (Wails `ClipboardSetText`, else `navigator.clipboard`); Remove from group for a `manualTags` member, else Add to group (`Configure` with the new `tags`, through the same path as ticking, so Auto-join is turned back on for the new group as before), unavailable with "Your group is full (12)" when full; Add to blacklist (`AddToBlacklist([tag])`, which also takes them out of the group; the item says so when they are in it) or Remove from blacklist (`RemoveFromBlacklist`). Members followed through the game team get no group item, blacklisted players no Add to group, and the player's own tag neither group nor blacklist items.
- **Keyboard and focus:** ARIA `menu`/`menuitem`/`separator`, focus on the first item; the arrow keys (wrapping), Home, End and a first letter move; Enter or Space chooses; Esc or Tab closes. Focus returns to the row, found again by BattleTag after the lists redraw, or to its neighbour when the row left the list. A pointer press outside, scrolling, resizing or leaving the window closes it.
- **Placement:** fixed, below and right of the pointer or row, flipped above or left when there's no room, and kept 8 px inside the window; it scrolls when the window is shorter than the menu.
- **Confirmation:** a short line at the bottom ("Copied Player#1234", "Blacklisted Player#1234") and a polite status region for screen readers.
- **Groups:** items come in groups split by a separator: the player's note first (see Player notes), then Copy BattleTag and the group item, then the blacklist item.

## Player notes

A private note per BattleTag, kept on this computer only and never sent to the game, logged or put in `Diagnostics`.

- **Rules:** `domain/follow/notes.go`. `NoteKey` is the lower-cased, trimmed BattleTag; `NoteText` trims the text and refuses a note longer than `dto.MaxNoteLength` (500) characters (Unicode characters, not bytes) whole, never cutting it short ("A note can be at most 500 characters."). Empty text deletes the note.
- **Saving:** `Companion.SetNote(tag, text)` is the only writer. It saves the whole map through `contract.PlayerNotes` (`notes.json`, written atomically next to `friends.json`) and only then changes the state, so a failed save keeps the old note and returns "Your note couldn't be saved on this computer. Try again.". It logs `notes.saved` with a count, never a BattleTag or the text. A missing, unreadable or corrupt file loads as no notes; loading also lower-cases keys, trims text, cuts anything over 500 characters and drops empty notes.
- **Bridge:** `SetNote(tag, text)` and `DeleteNote(tag)` (which sends empty text).
- **Menu:** Add note when the player has none; Edit note and View note when they do. Every player row and chip offers them, the player's own included.
- **Popover:** `cmd/gui/frontend/app.js` (`openNote`). One fixed popover (`role="dialog"`), placed like the menu next to the row. Writing: a textarea, a count ("61 / 500", amber within 50 of the limit, red with "N over the 500-character limit" past it, when Save is unavailable and screen readers hear the limit once), and Save, Cancel and Delete (Delete only for an existing note). Ctrl+Enter or Cmd+Enter saves; saving empty text deletes. Errors stay in the popover so nothing typed is lost. Reading: the note as plain text with its line breaks (`textContent`, never HTML), when it was updated, and Edit note and Close. Esc closes and focus returns to the row; Tab stays inside. A click outside closes it unless the text was changed.
- **Lists:** rows in Friends, Recent players and Blacklist show a small note icon ("Has a note") after the name. The search box matches note text as well as BattleTags (and maps in Recent players).

## Player tooltip

The window only: `cmd/gui/frontend/app.js` (`playerFacts`, `showTip`). One shared tooltip (`role="tooltip"`) for every player row and group chip; nothing is sent to the game and no bridge method is new.

- **Content, in order, each part only when known:** the note (plain text, line breaks kept, at most 9 lines); the full BattleTag and status ("In your lobby", the friend's Battle.net status with an open lobby's name, or "Not a friend" once the roster has loaded); standing ("In your follow group", "In your follow group, from your game team", "Battle.net friend", "Blacklisted since 3 Oct"); history from the recent player entry (first seen, last seen, last map, lobbies shared).
- **Showing:** on hover after 500 ms; while one is up, or within 300 ms of one closing, the next row's shows at once. On keyboard focus (`:focus-visible`) at once, and the focused control gets `aria-describedby="player-tip"` while it shows. The pointer can move onto it; it hides 150 ms after the pointer leaves both, on focus leaving, Esc (which goes no further and leaves focus where it is; the next Esc reaches what's behind), a pointer press, resizing, leaving the window, or scrolling its row away (a focus tooltip moves with its row instead).
- **Placement:** fixed, below the row or chip, above when there's no room, 8 px inside the window. It never shows while the player menu or note popover is open, and opening either hides it, so it never covers them. The rows' own `title` tooltips are gone, so only this one shows.

## Ranking

`domain/follow/ranking.go` (`follow.Ranking`), driven by the companion with the events it already receives; it only reads them and never sends anything to the game. The window shows it in the Ranking dialog (`cmd/gui/frontend`): a sortable table (Player, Games, Wins, Losses, Win rate, Last played), default win rate then games, the player's own row marked from `FollowState.selfTag`, and Clear ranking after a confirmation.

- **Lobby snapshot:** each `GameLobbySetup` replaces it with the occupied seats of players who take part: humans and computers (computers only to count teams), with their `team`. Observers are left out; if the local player (`isSelf`) is an observer, the match records nothing. Lobby updates stop once a match loads, so the last one is the match.
- **Result:** `UpdateScoreInfo` with `scoreInfo.localPlayerWon` as a boolean, while a snapshot exists. A campaign score (`payload.data`) or a malformed one is ignored and keeps the snapshot. The snapshot is then dropped, so a repeated score event or screen counts nothing: every match is counted once. The score screen (`SetGlueScreen`) doesn't end the match, because the score event may come after it.
- **No result:** `MultiplayerGameLeave`, `ExitedGame`, `LoggedOut`, a menu screen, a disconnect, closing the game and a new lobby without a score end the match with nothing recorded.
- **Who:** the local player and every other human in the snapshot who isn't an observer, followed or not. Computers are never recorded. A player is counted once per match; tags match case-insensitively and the entry keeps the latest casing.
- **Win or loss:** the local player's team gets the local result. A group member on another team gets the opposite when that is certain: the player won (they lost), or the match had exactly two teams, computers included (they won). Otherwise, such as a free-for-all the player lost, they get nothing. A seat without a team number is skipped, and the match has no certain two-team case.
- **Per-team flag:** if `scoreInfo.players` entries carry `teamIndex` and a boolean `won`, a team's flag decides for the other teams, but only when the local team's flag matches `localPlayerWon` (otherwise the team numbers don't line up with the lobby's and the flags are ignored) and the entries for that team agree. Unconfirmed against the live game.
- **List:** one entry per BattleTag, most recently played first, at most `dto.MaxRankingEntries` (500). `FollowState.ranking` carries it.
- **Saving:** `contract.Ranking` (`ranking.json`, written atomically next to `friends.json`). The companion saves as soon as a match is recorded and on shutdown; a failed save is logged and retried on the next step. Only `ranking.saved` with a count is logged; no names.
- **Clear** (`Companion.ClearRanking`) removes the file and the list. It doesn't touch the follow group; a match in progress is disarmed and not recorded.

## Blacklist and auto-ban

`domain/follow/blacklist.go` (the list) and `domain/follow/ban.go` (banning), docs/adr/0008. The **Blacklist** tab in the People panel lists the blacklisted BattleTags with when each was added, adds one by BattleTag (checked as `Name#1234`), removes them, and holds the **Auto-ban** and **Add players I ban to the blacklist** switches. In Recent players, **Select to blacklist** turns the row checkboxes into a separate choice (red crosses, a red rail, "Who you follow doesn't change"); a bar shows "Blacklist N players" and Cancel, and asks to confirm before adding them. Escape steps back from the confirmation, then out of the choice. Blacklisted players show a Blacklisted badge and a struck-through avatar in Recent players and Friends, and can't be ticked to follow.

- **The list:** `FollowState.blacklist` (`{tag, added}`, newest first, at most `dto.MaxBlacklist`, 500), matched case-insensitively. The player's own BattleTag (from the team or a lobby's `isSelf` slot) is refused. Adding a player removes them from the follow group; a blacklisted player can't be added to it. Saved in `friends.json` with `autoBan` and `blacklistManualBans`, both on by default.
- **Who is banned:** in a `GameLobbySetup` with `isHost` true and Auto-ban on, an occupied human slot (`slotStatus` 2, `slotType` 0) whose non-empty `name` is blacklisted. Never the player's own slot, computers, open slots, or anyone in a lobby someone else hosts. It works in every lobby the player hosts, with or without Auto-host and Follow. A joining slot can first arrive with an empty `name`; nothing is decided until the BattleTag arrives.
- **How often:** once per player while they stay in the lobby. A player who drops out of an update is forgotten, so a rejoin is banned again. Leaving the lobby, a match starting or a disconnect starts over.
- **When not:** once `LobbyCountdown` reports `countdownLocked`, or once the match has started (match screens, `IsGameUIActive true`, loading). A running countdown that isn't locked doesn't stop a ban.
- **Each ban** notes "Banned Player#1234 (on your blacklist)." in Activity.

**The slot race.** `BanPlayerFromGameLobby` takes only a slot. If the blacklisted player leaves and someone takes the slot before the game handles the ban, the wrong person is banned. There is no safe way to ask the game for the lobby (`GetGameInfo` and `SendGameLobbySetup` close the game), so:

1. A ban is chosen only while reading a `GameLobbySetup`, the newest, and never from a timer or an older one.
2. The engine only marks it pending. The companion first handles every event already waiting on the socket (`GameSession.TryReceive`, without waiting for more), then calls `Engine.SendPendingBan`, which finds the player in the newest update: it bans their slot now (a player who moved is banned in the new slot), or nobody if they left.
3. One ban is in flight at a time. The next waits for the game's confirmation, a lobby update without that player in that slot, or 3 seconds, and is then chosen from the next lobby update.
4. The confirmation "Host banned player [X]" is checked against the bans still waiting. X being a player meant confirms that ban. Otherwise, if the newest lobby update shows X in a slot one of them targeted, the game hit the wrong player: if X isn't blacklisted, Activity says "Banned Player#5678 by mistake: Player#1234 left just before the ban." and the companion logs `follow.ban_mistake`. There is no unban command.

**Bans by hand.** When the player bans someone from a lobby they host, the game sends the same "Host banned player [X]" line. When X matches no ban of Team Follow's waiting, neither by tag nor by the slot it targeted (so a hand ban within 3 seconds of an auto-ban isn't taken for a mistake, and the auto-ban keeps waiting), and **Add players I ban to the blacklist** on, X joins the blacklist ("Added Player#1234 to your blacklist (you banned them).") and leaves the follow group, and the companion saves it. Text that isn't a BattleTag, the player's own tag, players already listed, and lines while the player isn't the host are ignored. The line is the English client's wording; on other client languages this does nothing.

**Only the player's own lines.** Guests can type "Host banned player [X]" in the lobby chat too. A ban line counts (for the check in 4 and for bans by hand) only when the `ChatMessage`'s `message.sender` is the player: their BattleTag or its name part, any case, after an optional "<CLAN> " prefix. Any other sender, or none, is ignored entirely: no blacklist change, no warning, and a waiting ban stays waiting. The companion logs `follow.ban_line_ignored` with the sender only.

Live check (issue 16, macOS, game 3.0.1.24371, 2026-10-28):

- The game's Ban menu item sends `BanPlayerFromGameLobby {slot}`, and Kick sends `KickPlayerFromGameLobby {slot}` (the game's menu script).
- Sent by Team Follow while hosting, `BanPlayerFromGameLobby {slot: 2}` removed that player within 70 ms, without crashing the game. The game confirmed it with `ChatMessage` `source=gameChat`: "Host banned player [Name#1234]", then sent a `GameLobbySetup` without that slot.
- A joining player's first `GameLobbySetup` slot can have an empty `name`; the BattleTag arrives in an update a few milliseconds later.
- `GameLobbySetup` carries `isHost` (true when the local player hosts) and `playerHost`.
- The confirmation showed in the lobby chat as "[Lobby] <host's name>: Host banned player [...]", sent as the local player. The game's menu script reads the sender from `message.sender` (display name, possibly prefixed "<CLAN> ").
- Not live-tested yet: a ban during an unlocked countdown, the confirmation line on non-English clients, and the exact `message.sender` value of the confirmation (name only or full BattleTag; both are accepted).

## Watching

Whenever the game is connected, the engine watches the group: the team's other members when *Follow my team* is on, plus the chosen friends. When one of them is seen with a joinable lobby (see Joining), it notes "Thrall#1820 opened a lobby: Legion TD." once per lobby (keyed like attempts), from the roster, without starting anything and without sending any command. This is what the window shows while Auto-join is off.

## Menu screens

Joining happens only from the game's menus. The screen names are the keys of the game menu's own `ScreenEnum` (in the `GlueManager.js` the game serves on its local menu port), as `SetGlueScreen` sends them. The game's own "Join game" action in the friends list has no screen condition (it checks only that the friend has an open lobby, that you aren't queued, have no team invitations pending and aren't already in that lobby), and Team Follow sends the same commands, so every menu screen allows joining:

| Joining allowed | Joining paused |
| --- | --- |
| `MAIN_MENU`, `MULTIPLAYER`, `CHAT`, `SYSTEM_MENU`, `VERSUS`, `VERSUS_SETUP`, `CUSTOM_LOBBIES`, `MULTIPLAYER_CUSTOM_GAMES_SETUP`, `LEADERBOARD`, `TOURNAMENTS`, `TOURNAMENTS_RESULTS_SCREEN`, `CAMPAIGNS`, `CAMPAIGN_SELECTION`, `CAMPAIGN_EXPANSIONS`, `CAMPAIGN_MISSIONS`, `MISSION_SELECTION`, `CUSTOM_CAMPAIGNS`, `LOAD_SAVED_CAMPAIGN`, `SINGLE_PLAYER_MENU`, `LOAD_GAME`, `REPLAYS_SCREEN`, `PROFILE`, `ARMORY`, `COLLECTION`, `CREDITS_SCREEN` | Lobbies: `GAME_LOBBY`, `CUSTOM_GAME_LOBBY`, `CREATE_GAME_LOBBY` (the player is hosting), `SINGLE_PLAYER_CUSTOM_LOBBY`, `JOIN_LOBBY`. Matches: `LOADING_SCREEN`, `DISABLED_SCREEN`, `OUT_OF_MENUS`, `BLACK_SCREEN`. After a match: `SCORE_SCREEN`, `RANKED_SCORE_SCREEN`, `CAMPAIGN_SCORE`, `RANK_UP_ANIMATION`, `RATING_SPLASH_SCREEN`. Before login: `LOGIN_DOORS`, `LOGIN_OPTIONS`, `DEMO_SPLASH_SCREEN` |

A screen in neither list (a future game build) changes nothing. Joining from menus other than Custom Games follows the game's own rules but has not been live-tested yet.

**Which screens are reported.** Checked live on 3.0.1.24371 (27 October 2026): moving between menus by clicking is handled inside the game's menu page and sends no `SetGlueScreen` at all. The game reports only the screen changes it drives itself. A single-player custom game sent, in order: `MultiplayerGameCreateStarted`, `SetGlueScreen GAME_LOBBY`, `GameLobbySetup`, `LobbyCountdown`, `GameLobbyGracefulExit`, `SetGlueScreen LOADING_SCREEN`, `SetGlueScreen DISABLED_SCREEN`, `IsGameUIActive`; and on quitting: `ExitedGame`, `IsGameUIActive`, `SetGlueScreen SCORE_SCREEN`, `MultiplayerGameLeave`. Returning from the score screen to the main menu sent nothing.

So the engine knows the player is in the menus (`FollowState.inMenus`) only from game-driven evidence: a menu `SetGlueScreen`, the custom game list (`GameList`, sent while Custom Games is open), or a resume after a match. Otherwise the window asks the player to confirm before turning Auto-join on.

## Starting (Auto-join)

Start (Configure with `enabled: true`) remembers the selection: non-self team members when *Follow my team* is on, plus each manually added friend, without duplicates. It refuses when:

- a join is still pending, the game isn't connected, or the build isn't in `verified_builds`;
- the team roster is older than 20 seconds (when the team is included);
- the player is in a lobby, match, queue, or hasn't returned to the menus;
- the caller didn't confirm the player is in the menus (`fromMenus`). The window confirms automatically when the game has reported a menu screen since connecting, and asks the player otherwise;
- a chosen friend has no fresh entry in the friends roster, is not a Battle.net friend, or is the player. The roster, not the presence the engine acts on, is checked, so Auto-join can be re-applied straight after a group change.

Starting discards cached friend activity and asks for a fresh snapshot, so nothing is joined from data older than the start. Any change of selection or mode stops Follow; a selection change while Follow is on says so in the status. The window then turns Auto-join straight back on for the new group, unless the player is in a lobby, match or queue.

Team Follow never sends team invitations. The follow group lives in the app.

## Joining

The follow group works as one team: no friend is preferred. The first remembered player whose lobby is seen open is joined, which in practice is whoever opens a lobby first. Only when two are already open in the same update does watched order (team first, then chosen friends) decide.

A remembered player is joinable when their fresh (under 20 s) presence is online, a friend, in Warcraft III (`currentProgram = W3`), `currentGameState = open`, and has a valid account id.

- The window always starts with `diagnostic: false`. With `diagnostic: true` (preview, kept for tests and the API) a started engine announces the opportunity and sends nothing.
- **Auto-join** attempts each observed lobby once (keyed by player, presence epoch and lobby name), at most once every 10 seconds. A lobby counts as new when its name changes or the friend's lobby closes and reopens.
- A join that isn't confirmed within 15 seconds is dropped and not retried.
- A `GameLobbySetup` with the player in an occupied slot of the requested lobby confirms the join.

## Joining the host's team

With **Join the host's team** on (`joinHostTeam`, the default, saved with the friend list), Auto-join helps the group end up on one team in the lobby it joined. On each `GameLobbySetup` of that lobby, while the player is in it, when the group member it followed is the lobby's host (`playerHost`, matched by BattleTag or name) and the player sits on another team while the host's team has an open slot, the player's own slot moves there with `SetTeam {slot, team}`. Every player may move their own slot (the lobby's team header sends the same command); this is the only way into an open slot, since the game's `!swap` refuses one. Only the player's own slot ever moves, at most twice per lobby and team, and not while the countdown runs (`LobbyCountdown` `details.countdownActive`, never a `GameLobbySetup` field). If the game locks the player's slot (`teamChangeEnabled` false, as when the lobby locks teams), it notes once that the host's lobby doesn't let players change teams. Auto-host's lobbies are created with teams locked (the game's default); whether a member can move there still needs a live test with a friend running Team Follow. Leaving the lobby, a match starting or a disconnect ends this.

## Greeting when joining a lobby

Always on, no setting (ADR 0010). On a `GameLobbySetup` in which the player's own slot is taken (`isSelf`, `slotStatus` 2) and `isHost` is present and false, the engine picks one of `hi`, `hey`, `hello`, `hi all`, `hey everyone`, `hello there`, `hiya`, `glhf` at random and types it in the lobby chat (`sendChat`) after a random 3 to 10 seconds. If another player greets in the lobby chat first (a `ChatMessage` with `source=gameChat`, a sender who isn't the player, and a line opening with a greeting word such as hi, hey, hello, yo or glhf; `follow.IsGreeting`), the greeting is typed 2 to 4 seconds after theirs instead. Only the first greeting heard moves it. It works for any join, by Auto-join or by hand. It happens once per lobby (keyed by the lobby name; leaving, a match or a disconnect forgets the lobby, so the next one is greeted). It never happens as host, in an Auto-host lobby, or during a match. Leaving the lobby, the countdown starting, a match loading or Auto-host turning on before the timer fires cancels it. A failed send is not retried. The activity log notes it: `Said “hey” in the lobby.` The timer is checked on the one-second tick, so the line can be up to a second late.

## Lobby confirmations and passwords

The first matching `RequestForPassword` for the pending join is answered with an empty password, after the companion switches the custom game list on and asks for it (`SendGameListing`, then `GetGameList` each second, as the Custom Games screen does when it opens) and it has lobbies, or after 4 seconds: the game finds a lobby by name only in that list, so from the main menu the confirmation failed and looked like a password request. Repeats of the request while the list loads are ignored. A second request after the confirmation means the lobby wants a password. With **Ask for lobby passwords** off (`askPassword`, the default, saved with the friend list), the lobby is skipped with a note and not tried again while it stays open; switching it off also drops a password field already showing. With it on, a password field is shown. The password is sent once, never stored, and the request expires after 2 minutes. Confirmations that are unsolicited, late, for another lobby, or arrive after Stop or a queue are ignored.

## Pausing and resuming

Follow stays on across joins, pausing while the player is in a lobby, loading, in a match or queued. It resumes 2 seconds after evidence that the player is out of the match and back in the menus, then asks for fresh friend activity:

- **After a match:** the score screen (`SCORE_SCREEN`, `RANKED_SCORE_SCREEN`, `CAMPAIGN_SCORE`, `RANK_UP_ANIMATION`, `RATING_SPLASH_SCREEN`). The game shows it only once the match has ended, the click from it back to the menus is never reported, and the game's own Join Friend works from it. The `MultiplayerGameLeave` that follows keeps this evidence.
- **After leaving a lobby:** a menu `SetGlueScreen` or the custom game list (`GameList`).

A leave or loading signal alone never resumes. Stop always wins over automatic resume.

## Disconnects

Losing the game connection, or `LoggedOut`, forgets the roster and switches Follow off. The friend list and mode stay. Follow always starts off when the app opens. After `LoggedOut` the companion closes that connection and looks for the game again at once: the same account signing in from another copy signs the first one out, and that copy may keep running. If both copies are still running, the window says so (see `docs/install.md`) until the extra one is closed, and offers **Close all Warcraft III**.

**Close all Warcraft III** (`Companion.CloseGames`, also in Report a problem) runs only after the player confirms. It closes every copy of the game this user runs (`contract.GameProcesses`, see `docs/adr/0005-closing-the-game-on-request.md`), drops the game session (Follow and Auto-host go off, as on any disconnect), notes "Closed 2 copies of Warcraft III." (or "No copy of Warcraft III was running.") in Activity, and looks for the game again straight away. The player then starts the game again through Battle.net.

## Diagnostics

`Diagnostics()` reports version, screen, counts and event-type counts only. It never contains BattleTags, account ids, lobby names, passwords or the menu session.

## Auto-host

`domain/follow/host.go`. The host's setup (`dto.HostSettings`) is saved with the friend list. Turning Auto-host on turns Auto-join off, and the other way round.

1. **Waiting:** once `delaySeconds` have passed and the player is in the menus (no lobby, match, queue or post-match wait), the engine sends `CreateLobby` with the map's path as the game sees it (`MapsLocator.GamePath`), the name (each `%d` in it replaced by the game number: 1 when Auto-host is turned on, one more each time a hosted game starts, so `Legion TD #%d` gives `Legion TD #1`, `Legion TD #2`…; a lobby that never started keeps its number, and `HostState.lobbyName` shows the resolved name), `privateGame`, and `mapSettings` with the chosen visibility and observers. Everything else is the game's Create Game default: speed 2 (fast), lock teams and teams together on, shared control, random races and random hero off.
2. **Creating:** the lobby counts as open on `SetGlueScreen GAME_LOBBY` or `GameLobbySetup`. If neither arrives within 20 seconds, Auto-host turns off and says so.
3. **Lobby:** each `GameLobbySetup` is read for playable slots (`slotStatus` 0 open, 2 occupied; observers ignored), which group members are in (matched by BattleTag or name), and their `team`.
   - **Same team** brings each group member to the host's team with `SetTeam {slot, team}`, one move per lobby update, each move at most twice per slot and team. The game lets the host move only its own slot and, on maps without fixed forces, computers (`teamChangeEnabled`; other players' slots are always locked for the host), and only into an open slot. Group members always come to the host's team: the host never moves, and computers are never moved or swapped for it (moving the host split it from the rest of the group when members sat on different teams). So: a member who can move goes to the host's team if it has an open slot. Otherwise:
     1. **Open slot on the host's team:** `!swap` can't use it, so the member's own Team Follow moves there (see Joining the host's team). The host says so once and waits 10 seconds for it.
     2. **Swap:** the host types `!swap <member position> <other position>` in the lobby chat (`sendChat`), where a position is the lobby's `slot` + 1, trading the member with a human on the host's team who isn't in the follow group ("Asked the game to swap Friend#1234 with Player#5678."). Open slots, computers and group members are never targets. This also runs after the 10 seconds above, for a member without Team Follow.
     3. **Otherwise** the window says once that the member can't be moved to the host's team.

     Swaps are throttled: one at a time; each waits for a lobby update with different seating, or 5 seconds. A swap after which the member still isn't on the host's team counts as unanswered, and after 3 Same team stops typing for this lobby and says so once. Nothing is typed while the countdown runs. The counts reset with each new lobby.
   - **Auto-invite** sends `SendInviteToGame {target}` once per lobby to each group member who is a friend, online and in Warcraft III but not in the lobby, including friends who come online later.
   - **Start when full** sends `LobbyStart` when the lobby is full, the whole group is in and, with Same team, on the host's team. If that stops being true while the countdown runs (`LobbyCountdown` `details.countdownActive`), it sends `LobbyCancel`, unless the countdown is locked (`countdownLocked`), which the game doesn't allow to be cancelled.
   - Without Start when full, nothing starts the game: the host presses Start.
   - The host leaving the lobby (`MultiplayerGameLeave`) turns Auto-host off.
4. **Match → waiting:** loading and match screens mark the match; the score screen (or `ExitedGame`) starts the next delay, and the next lobby is created as in step 1.

**Game mode** (`domain/follow/gamemode.go`) types `gameMode`, for example `-ar`, once per match in the match chat, only in a match from a lobby Auto-host created:

- The delay starts when the match starts: the first `SetGlueScreen DISABLED_SCREEN` or `IsGameUIActive true` in Auto-host's match phase (live check, issue 9). `LoadingScreenGameStart` never arrives, and the loading bar fills before the other players are ready.
- After `gameModeDelaySeconds` (0 to 120, 5 by default) the engine queues the line once. It never types itself: the companion takes the line (`TakeChatLine`), types it with `GameKeyboard.TypeLine` into the game's pid outside the engine lock, and reports back (`ChatLineTyped`, with a nil error when it was typed). A line due while no game is connected is dropped, never typed into a game that is gone, and the companion waits for typing to finish before it stops. The status line then says "Typed -ar in the game chat." (typed, not shown: the game never confirms a line, ADR 0007), or "Game mode didn't type -ar." with the reason, such as Warcraft III not being in front or the typing permission missing. Nothing is retried.
- Nothing is typed when the command is empty, in a match Auto-host didn't create, or when the match ends or the player leaves before the delay is over (`ExitedGame`, a score screen, `MultiplayerGameLeave`, or the game disconnecting); the status line says the match ended first. `IsGameUIActive false` alone doesn't cancel it, since alt-tab or the in-game menu may send it.
- When the keyboard isn't supported (Linux for now) or not permitted, the companion doesn't press any key and reports the keyboard's reason.

Typing in the game chat: the engine can send one line with `SendGameChatMessage {content}` (`sendChat` in `host.go`, ADR 0006). The line is trimmed and must be a single non-empty line of at most 100 characters, or it is rejected and nothing is sent. It is meant only for Auto-host's own lobby or match as host and the greeting when joining a lobby (ADR 0010); the features that use it check that themselves. Same team's `!swap` (above) types in the lobby chat with it. Game mode types in the match chat with keystrokes instead, because the menu socket can't reach it. To check the chat by hand, run `wc3-team-follow say "<text>"`: it sends one line under the same limits and prints the game's events for `--wait` (10 s by default).

Live check (issue 4, game 3.0.1.24371 under Proton, 2026-10-28):

- In a private lobby where the host is the only human, `SendGameChatMessage` is accepted (no error, no crash) but nothing appears in the lobby chat, and no `ChatMessage` event comes back. The player's own typing in that lobby showed nothing either, so a solo lobby can't confirm or rule out lobby chat. It is still untested with a second human in the lobby.
- In a match, `SendGameChatMessage` does nothing: the menu page is inactive in-game (`IsGameUIActive true`, screen `DISABLED_SCREEN`), and the match chat is the game's own, which the menu socket doesn't reach.
- Map chat commands such as Castle Fight's `!swap` are read by the map's triggers, which run only in the match. A lobby chat line can't reach them, so Team Follow can't use `!swap` through the menu socket. (Corrected by the macOS check below: `!swap` is the game's own lobby command.)
- The game never sent `LoadingScreenGameStart`. Loading ends with `LoadProgressUpdate` at `fillPercentage` 1.0, then the game chat channel's `OnChannelLeave`, `UpdateLoadingScreenInfo`, `SetGlueScreen DISABLED_SCREEN` and `IsGameUIActive true`, all within about 50 ms. With other humans the match begins once everyone has loaded (`LoadingPlayerReadyUpdate`), which may be later.

Live check (issue 9, macOS 15.6.1, native game 3.0.1.24371, Castle Fight, 2026-10-28):

- **Lobby chat needs a second human.** With the host as the only human (computers only), neither `SendGameChatMessage` nor the player's own typing shows, in the lobby or in a match. With other humans in a public lobby, `SendGameChatMessage` reaches the lobby chat: the game echoes it as `ChatMessage` with `source=gameChat` and the same content within about 60 ms, and the player sees it as their own line.
- **`!swap` is the game's lobby command, not the map's.** Typed in the lobby (by the player or through `SendGameChatMessage`), the game answers under the host's name, for example "Command 'swap' matched no players for input '6'". It numbers the playable positions 1 to 8 down the lobby, which is `slot` + 1; the map's own computers (slots 8–15 in Castle Fight) get no number. `!swap 1 5` trades the first player of each team. It only trades two players: an open position or `0` is refused with "matched no players", and nobody moves. A swap therefore never changes how many players each team has. It can bring a group member to the host's team only by sending someone else from that team the other way. The map's computers can't be swapped.
- **The match chat isn't reachable through the menu socket.** In the match, `SendGameChatMessage` is accepted and nothing appears or comes back. The game's menu code has no command for the match chat (its chat commands are lobby, whisper, team, clan, LAN and Battle.net channels only).
- **Keystrokes reach the match chat.** With the player's chat set to All, System Events key events (Enter, about 1 s for the chat box to open, the text one character at a time, Enter) posted the line in the match's All chat. It needed the Accessibility grant for the app sending the keys. A first try that brought the game forward and then typed lost focus partway: the first letters went to the game as hotkeys and the rest, with the final Enter, into another app. A per-key frontmost check prevented that on the next try.
- **Match start:** the player's own `LoadProgressUpdate` reaches 1.0, the lobby's `gameChat` channel is left, `LoadingPlayerReadyUpdate` follows for each player, and the match starts with `SetGlueScreen DISABLED_SCREEN` and `IsGameUIActive true` once the last player is ready (3.5 s after the host's bar filled here). Game mode timing should start from that, not from the loading bar.

Commands, all taken from the game's own menu code: `CreateLobby` (Create Game's create button), `LobbyStart` and `LobbyCancel` (the host's lobby buttons), `SetTeam` (the host's team menu, and every player's team header for their own slot), `SendInviteToGame` (the friends list's Invite to game). Checked live on 3.0.1.24371 (27 October 2026): `CreateLobby` opens the hosted lobby, full detection counts computer players, Start when full waits for missing group members, and `LobbyStart` starts a full lobby. Under Proton on Linux (27 October 2026), `CreateLobby` opens a lobby with the map's slots and the host seated only when `filename` is written exactly as the game's own `MapList` writes it: forward slashes and no links (`C:/users/steamuser/Documents/…`). With backslashes, or through Proton's `My Documents` link, the lobby has one empty slot. `SetTeam` on the host's own slot moved the host to the other force (Castle Fight, 27 October 2026); slots of other players and of computers on that map came with `teamChangeEnabled` false. `LobbyCancel`, `SetTeam` on a computer and `SendInviteToGame` are not live-tested yet.
