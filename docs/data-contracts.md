# Data contracts

Ports (`port/contract`) and DTOs (`port/dto`) are the only types that cross layer boundaries.

## Layout

The rule: interfaces in `port/contract/`, DTOs in `port/dto/`, HTTP DTOs in its subfolders `port/dto/request/` and `port/dto/response/`.

- `port/contract/` — interfaces only, no structs. Inbound ports (`contract.ExampleService`) are called by transports; outbound ports are implemented by adapters and called by the domain.
- `port/dto/` (`package dto`) — data passed between the project's own layers that is not an HTTP shape: domain results (`dto.ExampleStatus` in `port/dto/example.go`), and job payloads (`dto.ExampleJob`, run by the `worker` and `example-job` commands). Add new files here directly; the domain and adapters both import `port/dto`.
- `port/dto/request/` — inbound HTTP shapes: decoded request bodies.
- `port/dto/response/` — outbound HTTP shapes returned to clients (`response.Health`, `response.Readiness`, `response.Example`).

The example route shows the mapping: `contract.ExampleService.Ping` returns a `dto.ExampleStatus`, and the handler turns it into a `response.Example` (adding the `"pong"` message) before encoding it as JSON. The domain never formats output for a transport.

```go
// cmd/http/app/example/handler/example.go
func toExampleResponse(status dto.ExampleStatus) response.Example {
	return response.Example{Message: "pong", Examples: status.Examples}
}
```

## Rules

1. DTOs are plain structs with tags and no behaviour beyond simple validation.
2. Handlers decode the request into a request DTO, validate it, call the domain through a contract, then map the result to a response DTO. Domain types never reach the transport.
3. The domain works with its own rich types; a contract may accept or return DTOs when the data is a transport shape by nature.
4. When an HTTP contract changes, update `cmd/http/resource/swagger/swagger.json` in the same change; it is the single API spec served at `/docs/`.
