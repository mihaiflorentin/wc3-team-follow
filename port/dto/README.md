# DTOs

Plain data structs. Interfaces never live here; they go in `port/contract`.

- `port/dto` (`package dto`) — data passed between the project's own layers: domain results such as `ExampleStatus` (`port/dto/example.go`), and job payloads such as `ExampleJob`, the `worker` and `example-job` input. Add new files directly in this folder.
- `port/dto/request` — inbound HTTP DTOs.
- `port/dto/response` — outbound HTTP DTOs.

Do not add other subfolders. See `docs/data-contracts.md`.
