# Auto-host: team swap by chat, game mode command, and a simpler window

Status: ready for agent. Live checks marked **(live)** must pass with the game before the matching part ships.

## Problem Statement

When I host with Auto-host and Same team switched on, my friends don't end up on my team. Team 1 can be full while my friend sits alone on team 2, even when there's an open slot. The game only lets the host move its own slot (and computers on maps without fixed forces), so Team Follow can't move other players with the lobby's own team controls. Many maps, Castle Fight among them, have a chat command `!swap <slotX> <slotY>` that swaps two players. Team Follow doesn't use it.

Many maps also expect the host to type a game mode command (for example `-ar` or `-sd`) once the match has loaded. I have to type it myself every match, and Auto-host can't do it for me.

The window is also busier than it needs to be. The Your team card on the right repeats what the follow group already shows. The "Your group. Their status updates live." caption adds nothing. The Ask for lobby passwords switch hangs loose between the Auto-join and Auto-host cards and looks out of place.

## Solution

- **Same team uses the map's swap command.** When Same team is on and a group member is on another team than mine, Auto-host first moves what it can with the game's own team controls, as today. If that doesn't bring us together, it types `!swap <slotX> <slotY>` in the lobby chat. It reads the slot numbers from the lobby, picks a target slot on my team (an open slot or a computer), and tells me what it did.
- **Game mode.** Auto-host settings get a Game mode section with a command field (for example `-ar`) and a delay in seconds. That many seconds after the match has loaded, Team Follow types the command in the game chat once. An empty command means the feature is off and nothing is typed.
- **Simpler window.** The Your team card is removed and the group caption goes. Ask for lobby passwords moves inside the Auto-join card as a secondary option under its description, shown as part of Auto-join instead of floating between cards.

## User Stories

### Team swap

1. As a host with Same team on, I want a friend on the other team moved to mine, so that we play together without me fixing the lobby by hand.
2. As a host, I want Team Follow to use the game's own team controls first, so that the swap command is only used when they can't do it.
3. As a host, I want Team Follow to type `!swap <slotX> <slotY>` in the lobby chat when the game's controls can't move my friend, so that maps with a swap command can still bring us together.
4. As a host, I want the slot numbers in the command to match the slot numbers the map uses, so that the right players are swapped.
5. As a host, I want a friend swapped into an open slot on my team when there is one, so that no other player is moved.
6. As a host, I want a friend swapped with a computer on my team when my team is full of computers, so that a computer gives up its place instead of a human.
7. As a host, I want Team Follow never to swap a human who isn't in my group off my team, so that strangers in a public lobby aren't moved around.
8. As a host, I want a status line saying which swap was asked for ("Asked the map to swap slot 2 and slot 5 for Friend#1234."), so that I know what happened.
9. As a host, I want Team Follow to wait for the lobby to update after a swap before trying again, so that it doesn't flood the chat.
10. As a host, I want Team Follow to stop asking after a few swaps that didn't change the lobby, and tell me once that this map didn't answer the swap command, so that a map without `!swap` isn't spammed.
11. As a host, I want each group member handled one at a time, so that swaps don't fight each other.
12. As a host, I want swaps to stop once the countdown is locked or the match starts, so that nothing is typed into a starting game.
13. As a host with Start when full on, I want the start to wait until the group is on my team, so that a match doesn't start with us split.
14. As a host on a map with fixed forces, I want a clear note when no swap was possible, so that I know to move players myself.
15. As a host, I want the swap behaviour to work the same on Windows, macOS and Linux, so that my friends and I get the same result.

### Game mode

16. As a host, I want to enter a game mode command in Auto-host settings, so that it's typed for me in every match Auto-host starts.
17. As a host, I want to set how many seconds after the match loads the command is typed, so that maps that ignore early chat still get it.
18. As a host, I want an empty command to switch Game mode off, so that nothing is typed when I don't need it.
19. As a host, I want the command typed exactly once per match, so that the mode isn't toggled twice.
20. As a host, I want the command typed only in matches from a lobby Auto-host created, so that games I join or host by hand are untouched.
21. As a host, I want the command not typed if the match ends or I leave before the delay is up, so that it never lands in the menus or the next lobby.
22. As a host, I want a status line when the command was typed ("Typed -ar in the game chat."), so that I can confirm it ran.
23. As a host, I want a sensible delay range (0–120 seconds, default 5), so that a typo can't make it wait for ever.
24. As a host, I want the command limited to one line of normal chat length, so that it can't send something the chat would reject.
25. As a host, I want the Auto-host card summary to mention the game mode when one is set ("Game mode -ar after 5 s"), so that I can see it without opening setup.
26. As a host, I want my game mode and delay saved with the other Auto-host settings, so that they survive a restart.

### Window

27. As a player, I want the Your team card gone, so that the window shows only what I act on.
28. As a player, I want the "Your group. Their status updates live." caption gone, so that the group list speaks for itself.
29. As a player, I want Ask for lobby passwords inside the Auto-join card, so that it reads as an Auto-join option.
30. As a player, I want the layout to stay tidy at the window's minimum size without the right-hand column, so that nothing is left as an empty gap.
31. As a player using the keyboard, I want the moved switch to keep its label and focus order, so that it stays accessible.

## Implementation Decisions

- **Chat command.** Add `SendGameChatMessage` (payload `{content}`) to the game socket allowlist. It is the command the game's own lobby chat box sends for the Custom Game channel. The safety rule for game access gains it, with what it's used for: the swap command and the Game mode command, only while Auto-host's lobby or match is open, and only text the player typed in settings or the fixed `!swap` form.
- **(live) Chat during a match.** It's unknown whether `SendGameChatMessage` reaches the match chat after loading, since the menu page is hidden in-game. Check this live before Game mode ships. If it only works in the lobby, Game mode is dropped or redesigned (for example typed in the lobby just before the start). Record the result in `docs/follow-behaviour.md`.
- **(live) `!swap` semantics.** Before shipping, confirm in Castle Fight that the command works in the lobby (or only in-game), that it needs the host to type it, and whether slot numbers start at 1 or 0, matching the lobby's `slot` field. The swap logic takes the base and the chat context as constants, set from the live check.
- **Slot model.** The lobby update already gives each slot's `slot`, `team`, `slotStatus`, `slotType` and name. The follow engine's lobby player gains the numeric slot used for the command. Swap planning is a pure function of the lobby players, the host and the group. It returns either a game-control move (as today), a chat swap `(memberSlot, targetSlot)`, or a "can't" reason. Its order: (1) existing moves, (2) swap a member with an open slot on the host's team, (3) swap a member with a computer on the host's team, (4) note once.
- **Throttling.** One swap at a time. After a swap, wait for the next lobby update showing a change, or a timeout. After a fixed number of swaps with no change (3), stop for this lobby and note once that the map didn't answer the swap command. The count resets when Auto-host opens a new lobby.
- **Match load signal.** The game's `LoadingScreenGameStart` event (seen in the game's UI script) marks the match as loaded. **(live)** Confirm it arrives over the socket when loading ends. Fall back to the first `matchScreens` screen after `LOADING_SCREEN` if it doesn't. Game mode schedules its command `delay` seconds after that signal on the engine's clock and cancels on leaving the match.
- **Settings.** `HostSettings` gains `gameMode` (string, trimmed, at most 100 characters, a single line) and `gameModeDelaySeconds` (int, 0–120, default 5). Both are saved in `friends.json` with the other Auto-host settings. They are part of the window's data contract, so `app.js` changes in the same change (see `docs/data-contracts.md`).
- **Behaviour lives in `domain/follow`.** The window only edits settings and shows notes. Swap planning, throttling and the Game mode timer are engine behaviour, driven by events and `Tick`.
- **Window.** Remove the Your team card and its update code from `index.html`, `app.js` and `style.css`, remove the group caption, and let the main column use the freed width. Move the `ask-password` switch into the Auto-join card body as a compact secondary switch under the description, using the card's existing spacing and type scale. Auto-host setup gets a Game mode group: a text field ("Game mode command", placeholder `-ar`) and a number field ("Seconds after loading", 0–120), with helper text saying an empty command turns it off. Interface text is sentence case and says what happens.
- **Docs.** Update `docs/follow-behaviour.md` (swap order, the chat commands, Game mode timing, live-check results), `docs/data-contracts.md`, and the allowlist list in `AGENTS.md`. Typing in the game chat is a new kind of game access, so it gets ADR 0006, which records why and the limits above.

## Testing Decisions

- Test behaviour through the follow engine, the existing highest seam. Drive `follow.Engine` with the fake clock and sender, feed lobby update and match events, and assert the commands sent and the notes. Don't test the planning helper's internals.
- Swap cases: open slot on the host's team leads to a chat swap with the right slot numbers. A full team of computers leads to a swap with a computer. A team full of strangers gets no swap and one note. The game's controls are used when possible. No resend before the lobby changes. The give-up after 3 unanswered swaps. No swaps after the countdown locks. Start when full waits for the group.
- Game mode cases: the command is sent once after the delay. Nothing is sent when the command is empty. Nothing is sent when the match ends before the delay. Nothing is sent in a match Auto-host didn't create. The delay is clamped.
- Settings round-trip in `infrastructure/settings` tests. The allowlist test covers the new command.
- Window: `cmd/gui/bridge_test.go` for settings passing through. Check the page in a browser with the demo bridge scenes, including a scene with Game mode set and one without the team card.
- Prior art: the Same team and computer-move tests in `domain/follow/host_test.go` and the join flow tests in `domain/follow/engine_test.go`.

## Out of Scope

- Moving players on maps that have neither team controls for them nor a swap command.
- Chat commands other than `!swap` and the single Game mode command, such as several commands, scripts or per-map presets.
- Typing anything in games Auto-host didn't create, or as a non-host.
- Reading or reacting to other players' chat.
- Redesigning the rest of the window beyond the items above.

## Further Notes

- Two facts need live checks with the game: chat during a match, and how `!swap` behaves (where it works and which slot numbering it uses). Do these first. They decide whether Game mode ships as described and which slot base the swap uses.
- Earlier live results that still apply: `SetTeam` moves only the host's own slot, plus computers on maps without fixed forces. `SendGameLobbySetup` and `GetGameInfo` crashed the game and stay banned.
