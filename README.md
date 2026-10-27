# wc3-team-follow

wc3-team-follow is a Go service generated on 2026-10-27T16:36:16+02:00. It follows a hexagonal (ports and adapters) architecture: business rules in `domain/` depend only on interfaces in `port/contract`, adapters implement those interfaces, and `main.go` wires everything together through constructor injection. Repository: https://github.com/mihaiflorentin/wc3-team-follow.

## Quick start

```bash
go mod tidy
make build
./bin/wc3-team-follow server
curl http://localhost:8080/health        # liveness
curl http://localhost:8080/health/ready  # readiness: checks every backend in use
```

`./bin/wc3-team-follow --help` lists the commands (`server`, `worker`, `example-job`), and `./bin/wc3-team-follow --version` prints the version injected at build time.

## Documentation

- `AGENTS.md` — the rules every change keeps: layout, dependency rules, injection, logging, testing, commands.
- `docs/getting-started.md` — setup, commands, configuration and endpoints.
- `docs/architecture.md` — layers, request flow and startup wiring.
- `docs/container.md` — the composition root and how to add a dependency.
- `docs/coding-style.md` — conventions for Go code in this repository.
- `docs/data-contracts.md` — where DTOs live and how they cross layers.
- `docs/logging-and-middleware.md` — the logger, log formats and the HTTP middleware order.
- `docs/adr/` — architectural decision records.
- `docs/sys.md` — filesystem and environment client.
- `docs/ci.md` — the CI pipeline(s), their jobs, triggers and permissions, and how to cut a release.

## Make targets

- `make build` — build `bin/wc3-team-follow` (`CGO_ENABLED=0`, version ldflags).
- `make build-all` — cross-compile for Linux, macOS and Windows into `dist/`.
- `make docker-build` — cross-compile the Linux binary inside the `golang` image into `dist/`.
- `make docker-image` — build the runtime image from `Dockerfile`, tagged `wc3-team-follow:latest` (override with `IMAGE_REPOSITORY` and `IMAGE_TAG`).
- `make test` — run `go test ./...`.
- `make fmt` — run `gofmt` on all Go files.
- `make lint` — run `golangci-lint`.
- `make tidy` — run `go mod tidy`.
- `make clean` — remove `bin/` and `dist/`.

## License

MIT; see `LICENSE`.
