package kratos

import "errors"

var (
	// ErrUnexpectedStatus is returned when the Kratos Admin API answers with a
	// status other than 200.
	errUnexpectedStatus = errors.New("kratos: unexpected status code")

	// ErrTraitsNotMap is returned when identity.Traits is not the map the
	// Admin API is documented to return.
	errTraitsNotMap = errors.New("kratos: identity traits is not a valid map")

	// ErrEmailMissing is returned when the identity carries no email trait.
	errEmailMissing = errors.New("kratos: email not found in identity traits")

	// ErrEmailNotString is returned when the email trait is present but is not
	// a non-empty string.
	errEmailNotString = errors.New("kratos: email is not a valid string")
)
