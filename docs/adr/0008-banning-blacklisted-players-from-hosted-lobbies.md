# 8. Banning blacklisted players from lobbies the player hosts

Status: accepted (2026-10-28). Issue 16.

## Context

Warcraft III has no blacklist. The host can ban a player from the current lobby through the name menu of their slot (Whisper, Kick, Ban, Close Slot, Report Player), but the ban lasts only for that lobby. Players asked Team Follow to keep a list of BattleTags and ban those players whenever they join a lobby the player hosts.

The game's Ban menu item sends `BanPlayerFromGameLobby {slot}` over the menu websocket, and Kick sends `KickPlayerFromGameLobby {slot}` (GlueManager.js on 3.0.1.24371). Checked live on 2026-10-28 (macOS, 3.0.1.24371):

- `BanPlayerFromGameLobby {slot: 2}` sent by Team Follow while hosting removed that player within 70 ms, without crashing the game.
- The game confirmed it with a lobby chat line, `ChatMessage` `source=gameChat`: "Host banned player [Name#1234]". It then sent a `GameLobbySetup` without that slot. A ban the host makes by hand produces the same line.
- A joining player's first `GameLobbySetup` slot can have an empty `name`; the BattleTag arrives in an update a few milliseconds later.
- `GameLobbySetup` carries `isHost` (true when the local player hosts) and `playerHost`.

Banning removes someone from a lobby and can't be undone from Team Follow (the game has no unban command), so this is a new kind of action, like typing (ADR 0006).

### The slot race

The command names a slot, not a player. If the blacklisted player leaves and someone else takes that slot between the lobby update Team Follow read and the moment the game handles the ban, the ban hits the wrong person. The game has no safe way to ask for the current lobby: `GetGameInfo` and `SendGameLobbySetup` both closed the game during development, and they stay banned from the allowlist.

## Decision

- `BanPlayerFromGameLobby` joins the allowlist in `infrastructure/gamesocket`. `KickPlayerFromGameLobby` is not added, and neither are the fetch commands above.
- The follow engine decides (`domain/follow/ban.go`). A ban is chosen only:
  - in a `GameLobbySetup` with `isHost` true, with Auto-ban on (the default);
  - for an occupied human slot (`slotStatus` 2, `slotType` 0) whose non-empty `name` matches a blacklisted BattleTag, case-insensitively;
  - never for the player's own slot (`isSelf`, or the player's own BattleTag), computers or open slots;
  - at most once per player while they stay in the lobby; a player who drops out of an update and rejoins is banned again;
  - never once `LobbyCountdown` reports `countdownLocked`, or once the match has started.
  It works in any lobby the player hosts, whether Auto-host created it or not, and never in someone else's lobby.
- Guards against the slot race:
  1. **Newest update only.** A ban is chosen only while reading a `GameLobbySetup`, never later from a timer or an older snapshot.
  2. **Read what is waiting first.** The engine only marks the ban pending. The companion then handles every event already received on the socket without waiting for more (`GameSession.TryReceive`, backed by the socket's reader goroutine), and calls `Engine.SendPendingBan`. That looks for the blacklisted player again in the newest lobby update: it bans the slot they are in now, or nobody if they left or the lobby, countdown or match no longer allows it.
  3. **One ban in flight.** After a ban, the next waits until the game confirms it, a lobby update no longer shows that player in that slot, or 3 seconds pass.
  4. **Check the confirmation.** The tag in "Host banned player [X]" is compared with the players meant by the bans still waiting (up to 3 seconds). A match confirms that ban. Otherwise, if the newest lobby update shows X in a slot one of those bans targeted, the game banned someone else: unless X is blacklisted too, Activity says so by name ("Banned Player#5678 by mistake: Player#1234 left just before the ban.") and the companion logs `follow.ban_mistake`. Nothing can undo it, and nothing tries. Any other X is a ban the player made by hand (below), and Team Follow's bans keep waiting for their own confirmation.
- Bans the player makes by hand: a "Host banned player [X]" line that matches no ban of Team Follow's waiting (by tag or by slot, as above), in a lobby the player hosts, adds X to the blacklist ("Added X to your blacklist (you banned them).") when **Add players I ban to the blacklist** is on (the default). Text that isn't a BattleTag, the player's own tag and players already listed are ignored. The line is the English client's; other client languages won't match, so this does nothing there.
- **Only the host's own lines.** Anyone in the lobby can type "Host banned player [X]" in the lobby chat. A line counts, for checking a ban or as a hand ban, only when its `message.sender` is the local player: their BattleTag or its name part, case-insensitively, after an optional leading "<CLAN> " prefix. A line with another sender or none is ignored entirely (no blacklist change, no warning, a waiting ban stays waiting) and logged as `follow.ban_line_ignored` with the sender only.
- The blacklist (`{tag, added}`, at most 500), Auto-ban and the hand-ban switch are saved in `friends.json`. Blacklisting a player removes them from the follow group, and a blacklisted player can't be added to it.

## Consequences

- AGENTS.md's safety rules list `BanPlayerFromGameLobby` and state when it may be sent.
- `contract.GameSession` gains `TryReceive`; every session implementation and double implements it.
- The guards narrow the race to the few milliseconds between the last event read and the game handling the command. They can't close it: there is no command that bans a player by name. When it happens, the player is told who was hit.
- Auto-ban relies on the game's English confirmation line only for checking and for hand bans. Banning itself doesn't depend on the client's language.
- Banning while the countdown runs but isn't locked is allowed; the game decides what a ban does to the countdown. Not live-tested yet.
- The sender check rests on the game's menu script, which reads `message.sender` (the display name, possibly prefixed "<CLAN> "), and on the live check showing the confirmation as "[Lobby] <host's name>: Host banned player [...]". The exact `sender` value (name only, or full BattleTag) still needs a live confirmation. If it turns out to be something else, the check fails closed: confirmations are ignored, mistakes go unreported and hand bans aren't added, but Auto-ban still bans.
