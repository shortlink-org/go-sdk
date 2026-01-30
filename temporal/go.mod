module github.com/shortlink-org/go-sdk/temporal

go 1.25.5

require (
	github.com/shortlink-org/go-sdk/config v0.0.0-20260107222411-453281b10921
	github.com/shortlink-org/go-sdk/grpc v0.0.0-20260107222411-453281b10921
	github.com/shortlink-org/go-sdk/logger v0.0.0-20260107222411-453281b10921
	github.com/shortlink-org/go-sdk/observability v0.0.0-20260107222411-453281b10921
	go.opentelemetry.io/otel v1.39.0
	go.opentelemetry.io/otel/trace v1.39.0
	go.temporal.io/sdk v1.39.0
	go.temporal.io/sdk/contrib/opentelemetry v0.6.0
)

replace (
	github.com/shortlink-org/go-sdk/config => ../config
	github.com/shortlink-org/go-sdk/grpc => ../grpc
	github.com/shortlink-org/go-sdk/logger => ../logger
	github.com/shortlink-org/go-sdk/observability => ../observability
)
