package grpc

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Client struct {
	interceptorUnaryClientList  []grpc.UnaryClientInterceptor
	interceptorStreamClientList []grpc.StreamClientInterceptor
	optionsNewClient            []grpc.DialOption

	port int
	host string
	cfg  *config.Config
}

func (c *Client) GetURI() string {
	return fmt.Sprintf("%s:%d", c.host, c.port)
}

// InitClient - set up a connection to the server.
func InitClient(_ context.Context, log logger.Logger, cfg *config.Config, options ...Option) (*grpc.ClientConn, func(), error) {
	config, err := SetClientConfig(cfg, options...)
	if err != nil {
		return nil, nil, err
	}

	// Set up a connection to the server peer
	conn, err := grpc.NewClient(
		config.GetURI(),
		config.optionsNewClient...,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to gRPC server: %w", err)
	}

	log.Info("Run gRPC Client",
		slog.Int("port", config.port),
		slog.String("host", config.host),
	)

	cleanup := func() {
		_ = conn.Close()
	}

	return conn, cleanup, nil
}

// SetClientConfig - set configuration
func SetClientConfig(cfg *config.Config, options ...Option) (*Client, error) {
	cfg.SetDefault("GRPC_CLIENT_PORT", "50051") // gRPC port
	grpcPort := cfg.GetInt("GRPC_CLIENT_PORT")

	cfg.SetDefault("GRPC_CLIENT_HOST", "0.0.0.0") // gRPC host
	grpcHost := cfg.GetString("GRPC_CLIENT_HOST")

	config := &Client{
		port: grpcPort,
		host: grpcHost,
		cfg:  cfg,
	}

	config.apply(options...)

	// Initialize your gRPC server's interceptor.
	config.optionsNewClient = append(
		config.optionsNewClient,
		grpc.WithChainUnaryInterceptor(config.interceptorUnaryClientList...),
		grpc.WithChainStreamInterceptor(config.interceptorStreamClientList...),
	)

	// NOTE: made after initialize your gRPC Client's interceptor.
	err := config.withTLS()
	if err != nil {
		return nil, err
	}

	return config, nil
}

// GetOptions - return options for gRPC Client.
func (c *Client) GetOptions() []grpc.DialOption {
	return c.optionsNewClient
}

// withTLS - setup TLS
func (c *Client) withTLS() error {
	c.cfg.SetDefault("GRPC_CLIENT_TLS_ENABLED", false) // gRPC TLS
	isEnableTLS := c.cfg.GetBool("GRPC_CLIENT_TLS_ENABLED")

	c.cfg.SetDefault("GRPC_CLIENT_CERT_PATH", "ops/cert/intermediate_ca.pem") // gRPC Client cert
	certFile := c.cfg.GetString("GRPC_CLIENT_CERT_PATH")

	if isEnableTLS {
		creds, err := credentials.NewClientTLSFromFile(certFile, "")
		if err != nil {
			return fmt.Errorf("failed to setup TLS: %w", err)
		}

		c.optionsNewClient = append(c.optionsNewClient, grpc.WithTransportCredentials(creds))

		return nil
	}

	c.optionsNewClient = append(c.optionsNewClient, grpc.WithTransportCredentials(insecure.NewCredentials()))

	return nil
}
