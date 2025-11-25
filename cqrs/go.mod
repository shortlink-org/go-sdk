module github.com/shortlink-org/go-sdk/cqrs

go 1.25.4

replace github.com/shortlink-org/go-sdk/watermill => ../watermill

require (
	github.com/ThreeDotsLabs/watermill v1.5.1
	github.com/google/uuid v1.6.0
	github.com/sony/gobreaker v1.0.0
	go.opentelemetry.io/otel v1.38.0
	go.opentelemetry.io/otel/trace v1.38.0
	google.golang.org/protobuf v1.36.10
)

require (
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/lithammer/shortuuid/v3 v3.0.7 // indirect
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
)
