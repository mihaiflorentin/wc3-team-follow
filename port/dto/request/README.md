# Request DTOs

Plain structs for inbound HTTP shapes (decoded request bodies). Handlers decode into them, validate, and map them to domain calls. Data passed between the project's own layers belongs in `port/dto` itself. See `docs/data-contracts.md`.
