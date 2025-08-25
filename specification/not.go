package specification

import (
	"errors"
)

var ErrNotSatisfied = errors.New("specification not satisfied")

// NotSpecification is a composite specification that represents the logical NOT of another specification.
type NotSpecification[T any] struct {
	spec Specification[T]
}

func (n *NotSpecification[T]) IsSatisfiedBy(item *T) error {
	// If inner spec PASSES → NOT should FAIL
	err := n.spec.IsSatisfiedBy(item)
	if err == nil {
		return ErrNotSatisfied
	}

	// If inner spec FAILS → NOT should PASS
	return nil
}

func NewNotSpecification[T any](spec Specification[T]) *NotSpecification[T] {
	return &NotSpecification[T]{spec: spec}
}
