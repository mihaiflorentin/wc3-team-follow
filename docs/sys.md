# Sys

The sys pack adds a small filesystem and environment port, `contract.Sys` (`port/contract/sys.go`), its adapter over `os` and `path/filepath` (`infrastructure/sys/client.go`) and an in-memory mock (`mock/sys/sys.go`). Code that takes `contract.Sys` instead of calling `os` directly can be tested without touching the disk or the process environment.

## Wiring

```go
// container/infrastructure.go
func (s *ServiceContainer) Sys() (contract.Sys, error) {
	return s.infrastructure.sys.get(func() (contract.Sys, error) {
		return sys.New(), nil
	})
}
```

The adapter is stateless, needs no config section and holds no resources to close.

## Contract

- Filesystem: `EnsureDir`, `Exists`, `Abs`, `Getwd`, `Open`, `OpenFile`, `ReadFile`, `WriteFile`, `Remove`, `RemoveAll`, `Rename`.
- Environment: `Environ`, `Getenv`, `LookupEnv`.

The methods mirror the standard library; filesystem errors are wrapped with the path involved, and `Exists` returns `false, nil` for a missing path.

## Injecting it into a consumer

Consumers take `contract.Sys` through their constructor. For example, to have the example service report a marker file:

```go
// domain/example/service.go
type Service struct {
	logger *slog.Logger
	files  contract.Sys
}

func NewService(logger *slog.Logger, files contract.Sys) *Service {
	return &Service{logger: logger, files: files}
}

func (s *Service) Ping(ctx context.Context) (dto.ExampleStatus, error) {
	maintenance, err := s.files.Exists("maintenance.flag")
	if err != nil {
		return dto.ExampleStatus{}, fmt.Errorf("check maintenance flag: %w", err)
	}
	if maintenance {
		return dto.ExampleStatus{}, errors.New("in maintenance")
	}
	return dto.ExampleStatus{}, nil
}
```

```go
// container/domain.go
func (s *ServiceContainer) ExampleService() (contract.ExampleService, error) {
	return s.domain.exampleService.get(func() (contract.ExampleService, error) {
		client, err := s.Sys()
		if err != nil {
			return nil, fmt.Errorf("resolve sys: %w", err)
		}
		return example.NewService(s.logger, client), nil
	})
}
```

## Testing

`mocksys.Client` keeps an in-memory filesystem and environment:

- `Files` (path → contents), `Dirs` (path → mode) and `Env` (name → value) seed the state; `WorkingDir` and `AbsBase` drive `Getwd` and `Abs`.
- Each filesystem method can be overridden with a `...Func` field.
- `Open` and `OpenFile` return the handle from `OpenFiles` when one is set for the path; otherwise they return a temporary file holding the stored contents, and `OpenFile` with `os.O_CREATE` creates an empty entry for a missing path.

`mock/sys/sys_test.go` covers the mock itself.

```go
client := &mocksys.Client{Files: map[string][]byte{"maintenance.flag": []byte("on")}}
svc := example.NewService(logger, client)
```
