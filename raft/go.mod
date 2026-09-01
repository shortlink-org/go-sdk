module github.com/shortlink-org/go-sdk/raft

go 1.27.0

require (
	github.com/shortlink-org/go-sdk/config v0.0.0-20260419222854-fd069f4d5106
	github.com/shortlink-org/go-sdk/grpc v0.0.0-20260417231502-a845b14b1f44
	github.com/stretchr/testify v1.12.1
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/Unleash/unleash-go-sdk/v6 v6.5.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bhope/hedge v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/fsnotify/fsnotify v1.10.1 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus v1.1.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.4 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/launchdarkly/eventsource v1.14.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/prometheus/client_golang v1.24.1 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/shortlink-org/go-sdk/auth v0.0.0-20260424225420-a63676f29741 // indirect
	github.com/shortlink-org/go-sdk/flight_trace v0.0.0-20260901121657-d7eab0200634 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/spf13/viper v1.21.0 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.71.0 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/sdk v1.46.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260831171406-18b4a7587f8a // indirect
)

replace github.com/shortlink-org/go-sdk/config => ../config

replace github.com/shortlink-org/go-sdk/grpc => ../grpc

replace github.com/shortlink-org/go-sdk/auth => ../auth

replace github.com/shortlink-org/go-sdk/observability => ../observability
