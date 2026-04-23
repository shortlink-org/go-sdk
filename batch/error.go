package batch

import (
	"errors"
)

// ErrInvalidType is returned when a value cannot be asserted to the expected type.
var ErrInvalidType = errors.New("invalid type")
