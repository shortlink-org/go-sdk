module github.com/shortlink-org/go-sdk/http

go 1.25.5

require (
	github.com/go-chi/chi/v5 v5.2.4
	github.com/golang-jwt/jwt/v5 v5.3.0
	github.com/google/uuid v1.6.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2
	github.com/shortlink-org/go-sdk/auth v0.0.0-20260107222628-ad66d85c8a41
	github.com/shortlink-org/go-sdk/config v0.0.0-20260107222411-453281b10921
	github.com/shortlink-org/go-sdk/flight_trace v0.0.0-20260107222411-453281b10921
	github.com/shortlink-org/go-sdk/grpc v0.0.0-20260107222411-453281b10921
	github.com/shortlink-org/go-sdk/logger v0.0.0-20260107222411-453281b10921
	github.com/stretchr/testify v1.11.1
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.64.0
	go.opentelemetry.io/otel v1.39.0
	go.opentelemetry.io/otel/sdk v1.39.0
	go.opentelemetry.io/otel/trace v1.39.0
	golang.org/x/sync v0.19.0
	google.golang.org/grpc v1.78.0
)

require (
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/Unleash/unleash-go-sdk/v5 v5.0.3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/common v0.67.4 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/spf13/viper v1.21.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	github.com/shortlink-org/go-sdk/auth => ../auth
	github.com/shortlink-org/go-sdk/config => ../config
	github.com/shortlink-org/go-sdk/flight_trace => ../flight_trace
	github.com/shortlink-org/go-sdk/grpc => ../grpc
	github.com/shortlink-org/go-sdk/logger => ../logger //lint:ignore gomoddirectives local development dependency
)
