# 6. Typing in the game chat

Status: accepted (2026-10-28). Extended by ADR 0007 (the match chat, with keystrokes) and ADR 0010 (a greeting when joining a lobby).

## Context

Two Auto-host features need to type a line in the game's chat: Same team asks the lobby's `!swap <slotX> <slotY>` bot command, and Game mode types a mode command once the match has loaded (specs in `docs/specs/auto-host-swap-game-mode-and-layout.md`). Until now the companion only sent menu commands (watching, joining, hosting). Typing is a new kind of action: the text is shown to every player in the lobby or match.

The game's own chat box does it with `SendGameChatMessage`, payload `{content: "<text>"}`, over the menu websocket (GlueManager.js, Custom Game channel, on 3.0.1.24371).

## Decision

- `SendGameChatMessage` joins the allowlist in `infrastructure/gamesocket`. It is the same message the game's chat box sends.
- The follow engine has one way to send it: the unexported `Engine.sendChat(text)` in `domain/follow/host.go`. Nothing else builds the message.
- Limits, enforced by `sendChat` before anything is sent:
  - The text is trimmed.
  - It must be one line: no line breaks or other control characters.
  - It must not be empty.
  - It is at most 100 characters (`follow.MaxChatLine`, counted in characters, not bytes).
  - Text that breaks a limit is rejected with `follow.ErrChatLine` and nothing is sent. It is never cut short.
- Only Auto-host's own lobby and match, as the host, may type. `sendChat` does not know the phase, so each caller checks that Auto-host is on and the player hosts that lobby or match before calling it.
- The text is fixed by the feature (a command built from the player's settings), never free text from the player or from other players. Chat lines are not logged; the engine notes the command's purpose, not its content.
- Auto-join, watching and Auto-host's existing steps type nothing, and tests assert it.
- The one manual exception is the `say` command (`wc3-team-follow say "<text>"`, issue 8), a diagnostic the player runs from a terminal to check the chat. It applies the same limits through `follow.ChatLine`, which `sendChat` also uses, and prints only event names, chat lines and channel names, never the session, account ids or whole payloads.

## Consequences

- AGENTS.md's safety rules list `SendGameChatMessage` in the allowlist and state when typing is allowed.
- Adding a chat feature means a caller that gates on phase and Auto-host, plus a test that it types only then.
- The game may drop or filter a line (flood limits, muted host); `sendChat` reports a closed connection but cannot know the line was shown.
- Live check on Linux under Proton (issue 4): the command is accepted but nothing showed in a solo lobby or in a match; see `docs/follow-behaviour.md`. Re-checks on macOS and Linux follow in issue 7.
