# Replay parser prototype

Status: prototype, for a future scoreboard. `infrastructure/replay` parses `.w3g` files into `dto.Replay`; `wc3-team-follow replay <file.w3g> [--json]` prints the result. Nothing in the companion or the window uses it yet.

## Where replays are

| OS | Folder |
| --- | --- |
| macOS, game 2.0 and later | `~/Library/Application Support/Warcraft III/BattleNet/<account id>/Replays/LastReplay.w3g` (confirmed) |
| macOS, older Reforged | `~/Library/Application Support/Blizzard/Warcraft III/Replay/LastReplay.w3g`; also `Logs/*_replay.w3g` and crash copies in `Errors/<date>/` |
| Windows | Probably `Documents\Warcraft III\BattleNet\<account id>\Replays\` (the Documents folder follows OneDrive). Not confirmed: neither reference nor our docs name it. |
| Linux | The Windows path inside the Wine or Proton prefix (not checked). |

The account id folder name must never be logged or saved, like the menu session.

## Format notes

- Header: 68 bytes, version 1, CRC32 over the header. Data blocks: zlib, 8192 bytes each, 12-byte block headers from version 10032 on (8 bytes before), with folded CRC32s.
- Game info: host record (0x00), game name, encoded map settings (map path, creator, 4-byte checksum, SHA-1), player records (0x16), Reforged metadata (0x39 before 2.0.2, 0x38 after), slot table (0x19).
- Reforged subtype 0x03 holds **all** players as repeated nested messages in field 1: `{1 id, 2 BattleTag, 3 clan, 4 portrait, 5 unknown (10 or 20), 6-8 unknown}`. w3gjs reads it as one flat message, which only works for the older layout. In current replays field 3 often holds the literal `clan`, so the clan is not reliable. Subtype 0x04 (skins) was empty in every replay seen.
- In 2.0.2+ replays action ids from **0x77** are one lower than before (raw 0x77 has the old 0x78 layout: a map sync such as `SyncCode`). w3gjs shifts only ids above 0x77 and misreads them.
- Leave result 0x0D, undocumented, appears from 1.32 to 3.0 for players who quit.

## What real replays gave

Five distinct games (1.32 build 6061, 1.33 build 6114, 2.00 build 6115 twice, 3.00 build 7003), all custom tower defence maps, all parsed without warnings:

- Reliable: version and build, duration, game name, host, map path, creator, checksum and SHA-1, settings, slots (team, color, race, handicap, computer and AI level, observers), BattleTags (2.0 and later), portrait, chat lines with time and channel, leave times and results, action counts per player.
- APM is low on these maps (1 to 50) because most actions are map syncs (0x62, 0x77 and up), which don't count. It is meaningful for melee, where it matches w3gjs's rules.
- Winner: `certain` only when a leave record says won, lost or draw. In the 3.0 replay results were 0x0D and 0x07, so the winner was only `likely`. A crash copy had no leave records at all.

## What replays don't hold

Kills, gold, lumber, experience, hero levels, items, the score screen, and ladder MMR or rank. A replay is the lobby plus every player's inputs. A full game simulation would be needed to get those values, which is not practical. Map stats sent with W3MMD syncs (action 0x6B, and 0x77 in 2.0.2+) are the only in-game values, and only on maps that send them.
