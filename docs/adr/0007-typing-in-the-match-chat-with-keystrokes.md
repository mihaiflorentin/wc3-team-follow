# 7. Typing in the match chat with keystrokes

Status: accepted (2026-10-28). Extends ADR 0006.

## Context

Game mode (issue 6) types a map command such as `-ar` once the match has loaded. ADR 0006 types through the menu socket's `SendGameChatMessage`, but the live checks (issues 4 and 9, `docs/follow-behaviour.md`) showed that it reaches only the lobby chat. The game's menu code has no command for the match chat. On macOS, key events did reach the match chat: Enter, about 1 s for the chat box to open, the text one character at a time, then Enter. On one try, focus moved to another app partway through the line. The first letters went to the game as hotkeys, and the rest went to the other app. A frontmost check before every key prevented that on the next try.

The research note `docs/research/in-match-chat-keystrokes.md` compares the methods for each OS. The owner decided on 2026-10-28 that Game mode types its command with keystrokes. Team Follow sends them itself, and only when the player opts in. Windows and macOS come first. Linux needs a helper inside the game's Wine prefix, and that helper needs a live test on the Linux machine first.

## Decision

- **One port.** `contract.GameKeyboard` (`Status`, `RequestPermission`, `TypeLine(ctx, pid, line)`) is the only way to send a key. `infrastructure/keyboard` implements it, and `mock/keyboard` is its test double. The sequence and its checks sit in an untagged file (`keyboard.go`) behind a small OS seam, so they are tested on every OS. Each `platform_<os>.go` only reports status, names the frontmost process and presses one key.
- **Method per OS.**
  - **Windows:** `SendInput` through `user32.dll` (pure Go, `golang.org/x/sys/windows`, no cgo). Characters go as `KEYEVENTF_UNICODE`, so the keyboard layout doesn't matter. Enter goes as `VK_RETURN`. Each key's down and up go in one call. The target check is `GetForegroundWindow` and then `GetWindowThreadProcessId`, which must equal the game's pid. A NULL foreground window is "not the game". No permission is needed. A game run as administrator silently drops the keys (UIPI), and Team Follow can't detect that.
  - **macOS:** `CGEventCreateKeyboardEvent` plus `CGEventKeyboardSetUnicodeString`, with the US-layout key code and Shift flag, posted with `CGEventPost` to `kCGHIDEventTap`. Return is key code 36. The target check needs two answers to agree with the game's pid: `NSWorkspace.frontmostApplication`, and the owner of the topmost on-screen window from `CGWindowListCopyWindowInfo`. The second answer covers a terminal command, where NSWorkspace's answer can go stale without a main run loop. Posting needs the event-synthesizing grant (shown under Accessibility). `Status` uses `CGPreflightPostEventAccess`, which never prompts. `RequestPermission` uses `CGRequestPostEventAccess`. Team Follow is ad-hoc signed, so macOS ties the grant to one exact build, and the player has to grant it again after every update.
  - **Linux:** not supported yet. `Status` reports `Supported: false` with "Typing in the match chat isn't supported on Linux yet.", and every other method refuses. A later ticket adds the Wine-side `SendInput` helper after the live test the research note describes. The uinput and portal routes can't check which window receives the keys, so they stay forbidden.
- **Rules.** These hold in `TypeLine`, whatever the caller:
  - Keys go only to the discovered game process, and only while it is the window in front. The check runs before **every** key, including the first Enter and the last.
  - The moment the check fails, or `ctx` is cancelled, typing stops and nothing more is sent. The error says how many keys went out and that the chat box may hold part of the line. It wraps `keyboard.ErrNotFrontmost` when focus left.
  - Team Follow never brings the game forward, never activates or focuses any window, and never steals focus. The player brings the game forward.
  - Only Enter and printable ASCII characters (`0x20`–`0x7E`) are sent: no hotkeys, orders, Esc or modifiers other than the Shift a character needs. A line of 1 to 100 such characters is checked before the first key and refused whole, never cut short. Callers have already passed it through `follow.ChatLine`.
  - Timing: Enter, 1 s for the chat box, then each character with 40 ms after it, then Enter. Plain Enter opens the player's current chat mode. The macOS check used All.
- **Who may type.** Only Auto-host's own Game mode command, as the host, after the player entered one and switched the feature on. It is off by default, and nothing re-enables it on its own. The one manual exception is the `type` diagnostic (`wc3-team-follow type -- "<text>"`), which the player runs from a terminal. It waits `--delay` (3 s by default) so the player can bring the game forward, and it asks for the macOS grant only with `--ask-permission`. Lines are not logged, as in ADR 0006.
- **Safety rules.** Keystrokes write no game memory and inject no code. Discovery stays read-only, and nothing reads game memory to learn the chat box's state. Keystrokes are a second action that drives the game, next to closing it on request (ADR 0005). They are fenced the same way: one port, one adapter, explicit player action.

## Consequences

- AGENTS.md's safety rules and Layout table name the keyboard, its target rule and who may type. `docs/architecture.md` lists the adapter.
- Game mode (issue 6) resolves `contract.GameKeyboard` from the container (`GameKeyboard()`). It shows `KeyboardStatus.Reason` when typing isn't possible, and offers `RequestPermission` only when the player asks.
- Team Follow can't see whether the chat box is already open or whether the player is typing. A half-typed line of the player's merges with Team Follow's line. On macOS, the player's own keys can land between Team Follow's keys.
- The game gives no confirmation that a line was shown, so callers report "typed", not "shown".
- Blizzard's EULA forbids automated control of the game in broad terms ("bots"). Typing one fixed chat line that the player chose, only when the player opts in, is the smallest form of it. The window must say plainly what the feature does.
- Needs a live test before it ships, as `verified_builds` gates joining:
  - On Windows, whether the match chat accepts `KEYEVENTF_UNICODE` text at all (research Open question 1), and whether the first characters survive after the 1 s wait.
  - On macOS, whether the game reads the Unicode string or the key code (non-US layouts), where the grant shows in System Settings, and that a rebuilt app loses it.
  - Fullscreen, borderless and windowed modes on both systems.
