# Follow behaviour

`domain/follow` is the state machine that decides when to join a friend's lobby. It is a port of the 0.4 release's `team-follow.cjs`, and `domain/follow/engine_test.go` carries over its behaviour tests. Change the behaviour there, and keep this page in step.

## Inputs

The engine reads events from the game's menu service (`dto.GameEvent`) and sends commands back through a `Sender`. The companion refreshes `TeamGetInfo` and `FriendsGetFriends` every `[follow] refresh_interval`; the engine never asks for anything else in the background.

Commands the app may send, enforced by `infrastructure/gamesocket`:

| Command | When |
| --- | --- |
| `TeamGetInfo`, `FriendsGetFriends` | Background refresh |
| `RequestCancelTeamInvitations`, `TeamLeave`, `JoinFriendGameLobby { auroraId }` | One join attempt, in this order, as the game's own Join Friend does |
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

The first matching `RequestForPassword` for the pending join is answered with an empty password. A second one shows a password field. The password is sent once, never stored, and the request expires after 2 minutes. Confirmations that are unsolicited, late, for another lobby, or arrive after Stop or a queue are ignored.

## Pausing and resuming

Follow stays on across joins, pausing while the player is in a lobby, loading, in a match or queued. It resumes 2 seconds after evidence that the player is out of the match and back in the menus, then asks for fresh friend activity:

- **After a match:** the score screen (`SCORE_SCREEN`, `RANKED_SCORE_SCREEN`, `CAMPAIGN_SCORE`, `RANK_UP_ANIMATION`, `RATING_SPLASH_SCREEN`). The game shows it only once the match has ended, the click from it back to the menus is never reported, and the game's own Join Friend works from it. The `MultiplayerGameLeave` that follows keeps this evidence.
- **After leaving a lobby:** a menu `SetGlueScreen` or the custom game list (`GameList`).

A leave or loading signal alone never resumes. Stop always wins over automatic resume.

## Disconnects

Losing the game connection, or `LoggedOut`, forgets the roster and switches Follow off. The friend list and mode stay. Follow always starts off when the app opens.

## Diagnostics

`Diagnostics()` reports version, screen, counts and event-type counts only. It never contains BattleTags, account ids, lobby names, passwords or the menu session.
