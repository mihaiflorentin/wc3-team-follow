package container

import (
	"github.com/mihaiflorentin/wc3-team-follow/domain/example"
	"github.com/mihaiflorentin/wc3-team-follow/port/contract"
)

type DomainContainer struct {
	exampleService lazy[contract.ExampleService]
}

// ExampleService returns the example domain service, built once with its
// collaborators injected.
func (s *ServiceContainer) ExampleService() (contract.ExampleService, error) {
	return s.domain.exampleService.get(func() (contract.ExampleService, error) {
		return example.NewService(s.logger), nil
	})
}
