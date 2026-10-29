# 10. Greeting when joining a lobby

Status: accepted (2026-10-30). Extends ADR 0006.

## Context

ADR 0006 lets Team Follow type in the game chat only for Auto-host, as the host of its own lobby or match. Players who join a friend's lobby by Auto-join or by hand then sit silent until someone speaks. A short hello from the joining player is what most people would type themselves. It is a second kind of typing: the text is shown to every player in a lobby someone else hosts.

The live checks (`docs/follow-behaviour.md`) showed that `SendGameChatMessage` reaches the lobby chat when other humans are in the lobby, and that the game echoes the line as `ChatMessage` with `source=gameChat`.

## Decision

- When the player joins a lobby as a guest, the engine types one greeting in the lobby chat after a random delay of 3 to 10 seconds, or 2 to 4 seconds after another player's greeting in the lobby chat if one comes first. It is always on; there is no setting.
- Guest means a `GameLobbySetup` in which the player's own slot is occupied (`isSelf`, `slotStatus` 2) and `isHost` is present and false. A missing `isHost`, the player's own hosted lobby (by hand or by Auto-host) and a match never greet.
- The line is one fixed greeting picked at random from `follow.Greetings()`: `hi`, `hey`, `hello`, `hi all`, `hey everyone`, `hello there`, `hiya`, `glhf`. All start with a lowercase letter. Nothing the player or other players wrote is ever typed.
- Once per lobby: the first `GameLobbySetup` that meets the rules starts the lobby, keyed by its name. Later updates of the same lobby do nothing. Leaving the lobby, a match or a disconnect forgets it, so the next lobby is greeted again.
- The greeting is cancelled if the player leaves the lobby, the countdown starts, a match loads or Auto-host turns on before the timer fires.
- It goes only through `Engine.sendChat`, so the limits of ADR 0006 apply (trimmed, one non-empty line, at most 100 characters). A failed send is not retried.
- The activity log notes the greeting, for example `Said “hey” in the lobby.`, so the player can see it happened. Chat lines are otherwise not logged.
- Randomness comes through `Engine.SetRandom` (the `follow.Random` interface, `math/rand/v2` in production), so tests fix it.

## Consequences

- AGENTS.md's safety rule for typing lists this case, and the ADR list includes 0010.
- A player cannot switch the greeting off. A later change can add a setting; it would be a new decision.
- The game may drop or filter the line (flood limits, a muted lobby); the engine cannot know it was shown.
- Auto-join, watching and Auto-host's other steps still type nothing besides this greeting and the lines of ADR 0006 and 0007.
