# Contracts

The interfaces here are the ports of the hexagon. Inbound ports are called by the window and commands; outbound ports are implemented by adapters and called by the domain.

This folder holds interfaces only. The data they accept and return are DTOs in `port/dto`.

| Contract | Direction | Implementation | Mock |
| --- | --- | --- | --- |
| `Companion` | inbound | `domain/companion` | `mock/companion` |
| `GameDiscovery` | outbound | `infrastructure/discovery` | `mock/game` |
| `GameDialer`, `GameSession` | outbound | `infrastructure/gamesocket` | `mock/game` |
| `PreferenceStore` | outbound | `infrastructure/settings` | `mock/preferences` |

Each contract has:

- one production implementation whose constructor returns the interface;
- a test double in `mock/<name>/` that starts with a compile-time assertion such as `var _ contract.GameDiscovery = (*Discovery)(nil)`;
- a lazy `(contract.X, error)` accessor in `container/` that wires the implementation.

Keep contracts small and named after what the consumer needs. I/O methods take `ctx context.Context` first and return an `error`.
