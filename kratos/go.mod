module github.com/shortlink-org/go-sdk/kratos

go 1.25.5

require (
	github.com/ory/client-go v1.22.22
	github.com/shortlink-org/go-sdk/config v0.0.0-20260107222411-453281b10921
	github.com/shortlink-org/go-sdk/logger 29f3e1960429
	github.com/spf13/viper v1.21.0
)

require (
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/shortlink-org/go-sdk/config => ../config

replace github.com/shortlink-org/go-sdk/logger => ../logger

