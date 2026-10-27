# Domain layer

Business rules live here, one package per bounded context:

- `domain/follow`: the follow state machine (`docs/follow-behaviour.md`). Pure: it takes a sender and a clock, and has no goroutines or I/O.
- `domain/companion`: the service the window drives (`contract.Companion`). It runs discovery, owns the one game session, feeds events to the engine, saves the friend list and publishes state.

Rules:

- A domain package imports only `port/` and the standard library, so it never depends on the window, the container, the config or an adapter.
- Collaborators arrive as `port/contract` interfaces plus a `*slog.Logger` through the constructor (`companion.New`). Settings arrive as an options struct, not config.
- Tests construct services with the mocks in `mock/` (see `domain/companion/service_test.go`).
