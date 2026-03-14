package module

import "context"

// StubModule is a placeholder for modules not yet implemented.
type StubModule struct {
	name string
}

// NewStub creates a stub module.
func NewStub(name string) *StubModule {
	return &StubModule{name: name}
}

func (s *StubModule) Name() string { return s.name }

func (s *StubModule) Check(_ context.Context, _ *RunContext) (*CheckResult, error) {
	return &CheckResult{
		Satisfied: false,
		Changes: []Change{
			{Description: s.name + " module not yet implemented"},
		},
	}, nil
}

func (s *StubModule) Apply(_ context.Context, rc *RunContext) (*ApplyResult, error) {
	if rc.DryRun {
		return &ApplyResult{Changed: false, Messages: []string{"[dry-run] skipped"}}, nil
	}
	return &ApplyResult{Changed: false, Messages: []string{"not yet implemented"}}, nil
}
