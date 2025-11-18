package http_client

import "errors"

var (
	ErrInvalidLimiterConfig = errors.New("http_client: invalid limiter config")
	ErrDeadlineTooClose     = errors.New("http_client: deadline too close")
)
