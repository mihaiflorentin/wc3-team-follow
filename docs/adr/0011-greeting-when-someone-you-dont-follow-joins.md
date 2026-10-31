# 11. Greeting when someone you don't follow joins

Status: accepted (2026-10-31). Supersedes ADR 0010; extends ADR 0006.

## Context

ADR 0010 greeted only when the player joined someone else's lobby as a guest, a few seconds after joining. The player wants the opposite trigger: a hello is for the people they don't already play with, and the host should greet too. Greeting a lobby that holds only the player's own followed friends, or only computers, says nothing to anyone new.

## Decision

- The engine types one greeting in the lobby chat 3 to 10 seconds (random) after the first human the player doesn't follow is seated in the lobby the player is in, or 2 to 4 seconds after another player's greeting if one comes first (as in ADR 0010). It is always on; there is no setting.
- It applies whether the player hosts the lobby (by hand or with Auto-host) or joined it. `isHost` no longer matters. It never happens during a match.
- "Someone the player doesn't follow" is an occupied human slot (`slotStatus` 2, `slotType` 0) with a non-empty name that is not the player and not in the follow group (`Engine.group()`: the chosen friends, and the game team when it is included). Computers, open slots and players whose name hasn't arrived yet don't count. If the player joins a lobby where such a player already sits, the delay starts at once.
- Once per lobby, keyed by the lobby name: after a greeting was planned, later joins in the same lobby type nothing. Leaving the lobby, a match or a disconnect forgets it, so the next lobby can be greeted again. The countdown starting cancels a waiting greeting.
- The lines, the activity note, the limits of ADR 0006 and the injected randomness are unchanged from ADR 0010.

## Consequences

- AGENTS.md's typing rule now allows the greeting as host and in Auto-host's lobby.
- In Auto-host's lobby the greeting and Same team's `!swap` both type in the lobby chat; they are separate lines and never combined.
- A lobby of only followed friends and computers is never greeted.
