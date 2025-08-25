package specification

import (
	"errors"
)

// AndSpecification is a composite specification that represents the logical AND of two other specifications.
type AndSpecification[T any] struct {
	specs []Specification[T]
}

func (a *AndSpecification[T]) IsSatisfiedBy(item *T) error {
	var errs error

	for _, spec := range a.specs {
		err := spec.IsSatisfiedBy(item)
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}

func NewAndSpecification[T any](specs ...Specification[T]) *AndSpecification[T] {
	return &AndSpecification[T]{
		specs: specs,
	}
}
