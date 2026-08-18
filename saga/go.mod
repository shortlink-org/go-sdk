module github.com/shortlink-org/go-sdk/saga

go 1.26.2

require (
	github.com/shortlink-org/go-sdk/logger v0.0.0-20260423005905-959e3e589a42
	github.com/stretchr/testify v1.12.0
	go.opentelemetry.io/otel v1.43.0
	go.opentelemetry.io/otel/trace v1.43.0
	go.uber.org/goleak v1.3.0
	golang.org/x/sync v0.22.0
)

require github.com/spf13/viper v1.21.0 // indirect

require (
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/Unleash/unleash-go-sdk/v6 v6.4.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/launchdarkly/eventsource v1.10.0 // indirect
	github.com/pelletier/go-toml/v2 v2.3.1 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/shortlink-org/go-sdk/config v0.0.0-20260419222854-fd069f4d5106 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stretchr/objx v0.5.3 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/twmb/murmur3 v1.1.8 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/shortlink-org/go-sdk/config => ../config
