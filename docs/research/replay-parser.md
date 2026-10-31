# Replay parser prototype

Status: in use by Blacklist players who leave Legion TD early (ADR 0013); still a prototype for a future scoreboard. `infrastructure/replay` parses `.w3g` files into `dto.Replay`; `wc3-team-follow replay <file.w3g> [--json]` prints the result. The companion finds the last replay with `contract.ReplayLocator` (`infrastructure/discovery/replays.go`) and reads it after each match while that switch is on.

## Where replays are

| OS | Folder |
| --- | --- |
| macOS, game 2.0 and later | `~/Library/Application Support/Warcraft III/BattleNet/<account id>/Replays/LastReplay.w3g` (confirmed) |
| macOS, older Reforged | `~/Library/Application Support/Blizzard/Warcraft III/Replay/LastReplay.w3g`; also `Logs/*_replay.w3g` and crash copies in `Errors/<date>/` |
| Windows | Probably `Documents\Warcraft III\BattleNet\<account id>\Replays\` (the Documents folder follows OneDrive). Not confirmed: neither reference nor our docs name it. The locator tries the Documents known folder, `%USERPROFILE%\Documents` and OneDrive's Documents. |
| Linux | The Windows path inside the Wine or Proton prefix (not checked). The locator tries `drive_c/users/*/Documents` and `My Documents` in every prefix discovery knows. |

The account id folder name must never be logged or saved, like the menu session.

## Format notes

- Header: 68 bytes, version 1, CRC32 over the header. Data blocks: zlib, 8192 bytes each, 12-byte block headers from version 10032 on (8 bytes before), with folded CRC32s.
- Game info: host record (0x00), game name, encoded map settings (map path, creator, 4-byte checksum, SHA-1), player records (0x16), Reforged metadata (0x39 before 2.0.2, 0x38 after), slot table (0x19).
- Reforged subtype 0x03 holds **all** players as repeated nested messages in field 1: `{1 id, 2 BattleTag, 3 clan, 4 portrait, 5 unknown (10 or 20), 6-8 unknown}`. w3gjs reads it as one flat message, which only works for the older layout. In current replays field 3 often holds the literal `clan`, so the clan is not reliable. Subtype 0x04 (skins) was empty in every replay seen.
- In 2.0.2+ replays action ids from **0x77** are one lower than before (raw 0x77 has the old 0x78 layout: a map sync such as `SyncCode`). w3gjs shifts only ids above 0x77 and misreads them.
- Leave result 0x0D, undocumented, appears from 1.32 to 3.0 for players who quit.
- Chat records (0x20) hold only what the saver could see: another team's allies chat is missing. Action **0x60** (8 bytes, then the line) is the line as the map's chat triggers receive it; every player's game records it, so it holds those lines too, without their channel. One line gives one 0x60 per matching trigger, so identical actions in the same time slot are one line. The parser adds the 0x60 lines that no chat record holds, with mode `unseen`.
- Action **0x62** (12 bytes, not shifted in 2.0.2+) is the game clock, sent by every player about once a second: a u32 that varies, the u32 clock value and a u32 repeat counter. While the match runs the value rises and the counter is 1. When it ends (a king dies, or a `-ff` or `-draw` vote passes) the clock stops: the same value is sent again with the counter going 2, 3, 4 … until players leave. The parser sets `Replay.ended` to when the last value was first sent with counter 1, once it was repeated; zero when the clock never stopped (the saver left before the end), and a value repeated mid-game that then rises again doesn't count. In real replays: a forfeit whose last `-ff` came at 21:35.6 froze at 21:36.49, players leaving 21:42 to 21:58; a game against computers froze at 20:54.37 when a king died, the player leaving at 21:07.8. 0x62 still counts as an action, not for APM.
- Orders 0x10 to 0x14 start with 2 flag bytes and a 4-byte item id. For training, building, buying and research the item id is the object type, stored backwards (`P40h` is `h04P`); built-in orders such as move and attack are numbers (0x000D0003). The parser keeps the object types as `Orders`.

## Naming orders from the map

`infrastructure/mapdata` reads a map file (an MPQ archive) and returns the names of the units, items, upgrades and abilities it defines or changes (`contract.MapObjects`). It finds files through the hash table, so protected maps without a file list work, and skips a decoy header before the real one at a 512-byte boundary. Names come from `war3mapSkin.w3u`/`.w3t`/`.w3q`/`.w3a` (Reforged 1.33 moved names there) and then `war3map.*`, with `TRIGSTR_` references resolved from `war3map.wts` and colour codes removed. `replay` finds the replay's map in the Maps folders (or takes `--map`). Legion TD Team OZE 11.4c-hf9 gave 1,762 names; its kings are "The King" (`h00K`, `h05K`, `h06H`), created by the script, and the "Upgrade King …" units (`u008`, `u009`, `u00A`) are what players buy for their king. Maps are read only, never copied into the repository.

## What real replays gave

Five distinct games (1.32 build 6061, 1.33 build 6114, 2.00 build 6115 twice, 3.00 build 7003), all custom tower defence maps, all parsed without warnings:

- Reliable: version and build, duration, game name, host, map path, creator, checksum and SHA-1, settings, slots (team, color, race, handicap, computer and AI level, observers), BattleTags (2.0 and later), portrait, chat lines with time and channel, leave times and results, action counts per player.
- APM is low on these maps (1 to 50) because most actions are map syncs (0x62, 0x77 and up), which don't count. It is meaningful for melee, where it matches w3gjs's rules.
- Winner: `certain` only when a leave record says won, lost or draw. In the 3.0 replay results were 0x0D and 0x07, so the winner was only `likely`. A crash copy had no leave records at all.

## What replays don't hold

Kills, damage, deaths (a king dying included), gold, lumber, experience, hero levels, items, the score screen, and ladder MMR or rank. A replay is the lobby plus every player's inputs; the game works everything else out again when a replay is watched, which is why the client shows damage and kills. A full game simulation would be needed to get those values, which is not practical. Map stats sent with W3MMD syncs (action 0x6B, and 0x77 in 2.0.2+) are the only in-game values, and only on maps that send them. Legion TD Team OZE sends none at the end: only each player's saved stats code at the start (`wSyncCode`, scrambled except the games count), and `wTH_PLAN` and `wBR_SUG`.

How a Legion TD game ended can still be read from chat: a forfeit is one team's `-ff` votes followed by the end, a draw is most players' `-draw` followed by the end, and a game played to a king's death has neither. The replay doesn't say which king died.
