# Domain layer

Business rules live here, one package per bounded context (`domain/example` is the placeholder to replace).

- A domain package imports only `port/` and the standard library, so it never depends on the transports, the container, the config or an adapter.
- Each service is a type with behaviour, built by a constructor that takes its collaborators as `port/contract` interfaces plus a `*slog.Logger`, like `example.NewService`.
- The transports call the domain through an inbound contract (`contract.ExampleService`); the service is wired in `container/domain.go`.
- Tests construct the service with the mocks in `mock/` (see `domain/example/service_test.go`).
- DTO mapping happens at the edge (handlers), not in the domain.
