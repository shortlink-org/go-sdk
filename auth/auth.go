package auth

import (
	"github.com/authzed/authzed-go/v1"
	"google.golang.org/grpc"

	"github.com/shortlink-org/go-sdk/config"
	rpc "github.com/shortlink-org/go-sdk/grpc"
)

func New(cfg *config.Config, options ...rpc.Option) (*authzed.Client, error) {
	cfg.SetDefault("SPICE_DB_COMMON_KEY", "secret-shortlink-preshared-key")
	cfg.SetDefault("SPICE_DB_TIMEOUT", "5s")

	clientCfg, err := rpc.SetClientConfig(cfg, options...)
	if err != nil {
		return nil, &ConfigurationError{Cause: err}
	}

	dialOptions := clientCfg.GetOptions()
	dialOptions = append(dialOptions,
		grpc.WithPerRPCCredentials(insecureMetadataCreds{"authorization": "Bearer " + cfg.GetString("SPICE_DB_COMMON_KEY")}),
		grpc.WithIdleTimeout(cfg.GetDuration("SPICE_DB_TIMEOUT")))

	client, err := authzed.NewClient(clientCfg.GetURI(), dialOptions...)
	if err != nil {
		return nil, &ClientInitError{Cause: err}
	}

	return client, nil
}
