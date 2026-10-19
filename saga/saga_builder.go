package saga

import (
	"context"

	"github.com/shortlink-org/go-sdk/saga/dag"
)

type Builder struct {
	*Saga

	errorList []error
}

func New(name string, setters ...Option) *Builder {
	newSaga := &Saga{
		name:  name,
		dag:   dag.New(),
		steps: make(map[string]*Step),
	}

	for _, setter := range setters {
		setter(&newSaga.Options)
	}

	return &Builder{
		Saga: newSaga,
	}
}

func (s *Builder) WithContext(ctx context.Context) *Builder {
	s.ctx = ctx //nolint:fatcontext // stores the caller-supplied context once; nothing is derived from s.ctx here
	return s
}

func (s *Builder) Build() (*Saga, []error) {
	return s.Saga, s.errorList
}
