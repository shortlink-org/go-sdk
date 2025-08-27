package specification

import (
	"errors"
)

// OrSpecification is a composite specification that represents the logical OR of two other specifications.
type OrSpecification[T any] struct {
	Specs []Specification[T]
}

func (o *OrSpecification[T]) IsSatisfiedBy(item *T) error {
	var errs error

	for _, spec := range o.Specs {
		err := spec.IsSatisfiedBy(item)
		if err == nil {
			return nil
		}

		errs = errors.Join(errs, err)
	}

	return errs
}

func NewOrSpecification[T any](specs ...Specification[T]) *OrSpecification[T] {
	return &OrSpecification[T]{
		Specs: specs,
	}
}
