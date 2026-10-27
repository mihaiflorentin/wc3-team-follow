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

`SendGameLobbySetup` and `GetGameInfo` closed the game during development and must never be added.

## The friends roster

Every `FriendsFriendData` snapshot replaces the roster: the whole Battle.net friends list, shown in the window's picker with each friend's status (`Lobby open`, `In Warcraft III`, `Online in Battle.net`, `Away`, `Offline`). The roster is display-only. It survives starting Follow (which empties the presence the engine acts on) and is cleared on disconnect. Ticking a friend adds their BattleTag to the follow group (`manualTags`, up to 12).

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

1. **Waiting:** once `delaySeconds` have passed and the player is in the menus (no lobby, match, queue or post-match wait), the engine sends `CreateLobby` with the map's path as the game sees it (`MapsLocator.GamePath`), the name, `privateGame`, and `mapSettings` with the chosen visibility and observers. Everything else is the game's Create Game default: speed 2 (fast), lock teams and teams together on, shared control, random races and random hero off.
2. **Creating:** the lobby counts as open on `SetGlueScreen GAME_LOBBY` or `GameLobbySetup`. If neither arrives within 20 seconds, Auto-host turns off and says so.
3. **Lobby:** each `GameLobbySetup` is read for playable slots (`slotStatus` 0 open, 2 occupied; observers ignored), which group members are in (matched by BattleTag or name), and their `team`.
   - **Same team** puts the host and each group member on one team with `SetTeam {slot, team}`, one move per lobby update, each move at most twice per slot and team. The game lets the host move only its own slot and, on maps without fixed forces, computers (`teamChangeEnabled`; other players' slots are always locked for the host), and only into an open slot. So: a member who can move goes to the host's team if it has an open slot; otherwise the host moves to the member's team if that has one; otherwise a movable computer (`slotType` 1) leaves the full team to make room, and the move follows on the next update. When nothing can move, the window says so once.
   - **Auto-invite** sends `SendInviteToGame {target}` once per lobby to each group member who is a friend, online and in Warcraft III but not in the lobby, including friends who come online later.
   - **Start when full** sends `LobbyStart` when the lobby is full, the whole group is in and, with Same team, on the host's team. If that stops being true while the countdown runs (`LobbyCountdown` `details.countdownActive`), it sends `LobbyCancel`, unless the countdown is locked (`countdownLocked`), which the game doesn't allow to be cancelled.
   - Without Start when full, nothing starts the game: the host presses Start.
   - The host leaving the lobby (`MultiplayerGameLeave`) turns Auto-host off.
4. **Match → waiting:** loading and match screens mark the match; the score screen (or `ExitedGame`) starts the next delay, and the next lobby is created as in step 1.

Typing in the game chat: the engine can send one line with `SendGameChatMessage {content}` (`sendChat` in `host.go`, ADR 0006). The line is trimmed and must be a single non-empty line of at most 100 characters, or it is rejected and nothing is sent. It is meant only for Auto-host's own lobby or match as host; the features that use it check that themselves. Nothing in Auto-host types today. To check the chat by hand, run `wc3-team-follow say "<text>"`: it sends one line under the same limits and prints the game's events for `--wait` (10 s by default).

Live check (issue 4, game 3.0.1.24371 under Proton, 2026-10-28):

- In a private lobby where the host is the only human, `SendGameChatMessage` is accepted (no error, no crash) but nothing appears in the lobby chat, and no `ChatMessage` event comes back. The player's own typing in that lobby showed nothing either, so a solo lobby can't confirm or rule out lobby chat. It is still untested with a second human in the lobby.
- In a match, `SendGameChatMessage` does nothing: the menu page is inactive in-game (`IsGameUIActive true`, screen `DISABLED_SCREEN`), and the match chat is the game's own, which the menu socket doesn't reach.
- Map chat commands such as Castle Fight's `!swap` are read by the map's triggers, which run only in the match. A lobby chat line can't reach them, so Team Follow can't use `!swap` through the menu socket.
- The game never sent `LoadingScreenGameStart`. Loading ends with `LoadProgressUpdate` at `fillPercentage` 1.0, then the game chat channel's `OnChannelLeave`, `UpdateLoadingScreenInfo`, `SetGlueScreen DISABLED_SCREEN` and `IsGameUIActive true`, all within about 50 ms. With other humans the match begins once everyone has loaded (`LoadingPlayerReadyUpdate`), which may be later.

Commands, all taken from the game's own menu code: `CreateLobby` (Create Game's create button), `LobbyStart` and `LobbyCancel` (the host's lobby buttons), `SetTeam` (the host's team menu), `SendInviteToGame` (the friends list's Invite to game). Checked live on 3.0.1.24371 (27 October 2026): `CreateLobby` opens the hosted lobby, full detection counts computer players, Start when full waits for missing group members, and `LobbyStart` starts a full lobby. Under Proton on Linux (27 October 2026), `CreateLobby` opens a lobby with the map's slots and the host seated only when `filename` is written exactly as the game's own `MapList` writes it: forward slashes and no links (`C:/users/steamuser/Documents/…`). With backslashes, or through Proton's `My Documents` link, the lobby has one empty slot. `SetTeam` on the host's own slot moved the host to the other force (Castle Fight, 27 October 2026); slots of other players and of computers on that map came with `teamChangeEnabled` false. `LobbyCancel`, `SetTeam` on a computer and `SendInviteToGame` are not live-tested yet.
