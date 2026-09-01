package cache

// InitCacheError reports that the cache could not be opened.
type InitCacheError struct {
	err error
}

func (e *InitCacheError) Error() string {
	return "error init cache: " + e.err.Error()
}

// Unwrap lets errors.Is and errors.AsType reach the cause, so a caller can
// still tell a bad URI from a refused connection behind the wrapper.
func (e *InitCacheError) Unwrap() error {
	return e.err
}

// NewCacheError creates a new cache error.
func NewCacheError(op string, err error) error {
	return &BaseError{
		op:  op,
		err: err,
	}
}

// BaseError is an error returned by cache operations.
type BaseError struct {
	op  string
	err error
}

func (e *BaseError) Error() string {
	return "cache: " + e.op + ": " + e.err.Error()
}

// Unwrap lets errors.Is and errors.AsType reach the cause: without it the
// operation name was bought at the price of every detail behind it, and a
// caller could not ask whether the failure was a timeout or a closed client.
func (e *BaseError) Unwrap() error {
	return e.err
}

// Op reports the operation the failure came from.
func (e *BaseError) Op() string {
	return e.op
}
