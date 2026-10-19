package dag

import "errors"

type VertexAlreadyExistsError struct {
	Id string //nolint:revive // public API: renaming the field breaks consumers
}

func (e *VertexAlreadyExistsError) Error() string {
	return "dag already contains a vertex with the id: " + e.Id
}

type VertexNotFoundError struct {
	Id string //nolint:revive // public API: renaming the field breaks consumers
}

func (e *VertexNotFoundError) Error() string {
	return "not found vertex by id: " + e.Id
}

var ErrIncorrectTypeAssertion = errors.New("incorrect type assertion")
