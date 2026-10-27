# Response DTOs

Plain structs for outbound HTTP payloads. Handlers map domain results (such as `dto.ExampleStatus`) into them before writing the response. Data that never leaves the process belongs in `port/dto` itself. See `docs/data-contracts.md`.
