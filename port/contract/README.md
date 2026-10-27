# Contracts

The interfaces here are the ports of the hexagon. Inbound ports are called by the transports; outbound ports are implemented by adapters and called by the domain.

This folder holds interfaces only. The data they accept and return are DTOs in `port/dto` (HTTP shapes in `port/dto/request` and `port/dto/response`).

```go
// port/contract/example.go
type ExampleService interface {
	// Ping reports the state of the example domain.
	Ping(ctx context.Context) (dto.ExampleStatus, error)
	// RunJob performs one example job. The worker calls it per consumed
	// payload and the example-job command calls it once.
	RunJob(ctx context.Context, job dto.ExampleJob) error
}
```

Each contract has:

- one production implementation whose constructor returns the interface;
- a test double in `mock/<name>/` that starts with a compile-time assertion such as `var _ contract.ExampleService = (*Service)(nil)`;
- a lazy `(contract.X, error)` accessor in `container/` that wires the implementation.

Keep contracts small and named after what the consumer needs. I/O methods take `ctx context.Context` first and return an `error`.
