package grpc

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"runtime/debug"

	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/flight_trace"
	"github.com/shortlink-org/go-sdk/grpc/authforward"
	"github.com/shortlink-org/go-sdk/grpc/authjwt"
	flight_trace_interceptor "github.com/shortlink-org/go-sdk/grpc/middleware/flight_trace"
	grpc_logger "github.com/shortlink-org/go-sdk/grpc/middleware/logger"
	pprof_interceptor "github.com/shortlink-org/go-sdk/grpc/middleware/pprof"
	session_interceptor "github.com/shortlink-org/go-sdk/grpc/middleware/session"
	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

// Server represents a configured gRPC server instance.
type Server struct {
	Run      func()
	Server   *grpc.Server
	Endpoint string
}

type server struct {
	interceptorStreamServerList []grpc.StreamServerInterceptor
	interceptorUnaryServerList  []grpc.UnaryServerInterceptor
	optionsNewServer            []grpc.ServerOption

	port int
	host string

	log           logger.Logger
	serverMetrics *grpc_prometheus.ServerMetrics
	cfg           *config.Config
	authValidator *authjwt.Validator
}

// InitServer - initialize gRPC server.
func InitServer(
	ctx context.Context,
	log logger.Logger,
	tracer trace.TracerProvider,
	prom *prometheus.Registry,
	flightRecorder *flight_trace.Recorder,
	cfg *config.Config,
) (*Server, error) {
	cfg.SetDefault("GRPC_SERVER_ENABLED", true) // gRPC server enable

	if !cfg.GetBool("GRPC_SERVER_ENABLED") {
		//nolint:nilnil // it's correct logic
		return nil, nil
	}

	config, err := setServerConfig(log, tracer, prom, flightRecorder, cfg) //nolint:contextcheck // false positive
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s:%d", config.host, config.port)

	var lc net.ListenConfig

	lis, err := lc.Listen(ctx, "tcp", endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %w", err)
	}

	// Initialize the gRPC server.
	grpcServer := grpc.NewServer(config.optionsNewServer...)

	grpcServerInstance := &Server{
		Server: grpcServer,
		Run: func() {
			// Register reflection service on gRPC server.
			reflection.Register(grpcServer)

			// After all your registrations, make sure all of the Prometheus metrics are initialized.
			config.serverMetrics.InitializeMetrics(grpcServer)

			log.Info("Run gRPC server",
				slog.Int("port", config.port),
				slog.String("host", config.host),
			)

			err = grpcServer.Serve(lis)
			if err != nil {
				log.Error(err.Error())
			}
		},
		Endpoint: endpoint,
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()

		if config.authValidator != nil {
			_ = config.authValidator.Close()
		}

		log.Info("Shutdown gRPC server")
		grpcServer.GracefulStop()
	}()

	return grpcServerInstance, nil
}

// setServerConfig - set configuration.
func setServerConfig(
	log logger.Logger,
	tracer trace.TracerProvider,
	monitor *prometheus.Registry,
	flightRecorder *flight_trace.Recorder,
	cfg *config.Config,
) (*server, error) {
	cfg.SetDefault("GRPC_SERVER_PORT", "50051") // gRPC port
	grpcPort := cfg.GetInt("GRPC_SERVER_PORT")

	cfg.SetDefault("GRPC_SERVER_HOST", "0.0.0.0") // gRPC host
	grpcHost := cfg.GetString("GRPC_SERVER_HOST")

	config := &server{
		port: grpcPort,
		host: grpcHost,

		log: log,
		cfg: cfg,
	}

	config.WithLogger(log)
	config.WithTracer(tracer)
	config.WithAuthHeaders()
	config.WithAuthForward()
	config.WithPprofLabels()
	config.WithFlightTrace(flightRecorder, log)

	if monitor != nil {
		config.WithMetrics(monitor)
		config.WithRecovery(monitor)
	}

	config.optionsNewServer = append(config.optionsNewServer,
		// Initialize your gRPC server's interceptor.
		grpc.ChainUnaryInterceptor(config.interceptorUnaryServerList...),
		grpc.ChainStreamInterceptor(config.interceptorStreamServerList...),
	)

	// NOTE: made after initialize your gRPC server's interceptor.
	err := config.WithTLS()
	if err != nil {
		return nil, err
	}

	return config, nil
}

// WithMetrics - setup metrics.
func (s *server) WithMetrics(prom *prometheus.Registry) {
	s.serverMetrics = grpc_prometheus.NewServerMetrics(
		grpc_prometheus.WithServerHandlingTimeHistogram(
			grpc_prometheus.WithHistogramBuckets([]float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120}),
		),
	)
	prom.MustRegister(s.serverMetrics)

	exemplarFromCtx := grpc_prometheus.WithExemplarFromContext(exemplarFromContext)

	s.interceptorUnaryServerList = append(
		s.interceptorUnaryServerList,
		s.serverMetrics.UnaryServerInterceptor(exemplarFromCtx),
	)
	s.interceptorStreamServerList = append(
		s.interceptorStreamServerList,
		s.serverMetrics.StreamServerInterceptor(exemplarFromCtx),
	)
}

// WithTracer - setup tracing.
func (s *server) WithTracer(tracer trace.TracerProvider) {
	if tracer == nil {
		return
	}

	s.optionsNewServer = append(s.optionsNewServer, grpc.StatsHandler(
		otelgrpc.NewServerHandler(otelgrpc.WithTracerProvider(tracer))),
	)
}

// WithRecovery - setup recovery.
func (s *server) WithRecovery(prom *prometheus.Registry) {
	// Setup metric for panic recoveries.
	panicsTotal := promauto.With(prom).NewCounter(prometheus.CounterOpts{
		Name: "grpc_req_panics_recovered_total",
		Help: "Total number of gRPC requests recovered from internal panic.",
	})
	grpcPanicRecoveryHandler := func(panicValue any) error {
		panicsTotal.Inc()
		s.log.Error("recovered from panic",
			slog.String("panic", fmt.Sprintf("%v", panicValue)),
			slog.String("stack", string(debug.Stack())),
		)

		return status.Errorf(codes.Internal, "%s", panicValue)
	}

	recoveryHandler := grpc_recovery.WithRecoveryHandler(grpcPanicRecoveryHandler)

	// Recovery handlers should typically be last in the chain so that other middleware
	// (e.g., logging) can operate in the recovered state instead of being directly affected by any panic
	s.interceptorUnaryServerList = append(
		s.interceptorUnaryServerList,
		grpc_recovery.UnaryServerInterceptor(recoveryHandler),
	)
	s.interceptorStreamServerList = append(
		s.interceptorStreamServerList,
		grpc_recovery.StreamServerInterceptor(recoveryHandler),
	)
}

// WithLogger - setup logger.
func (s *server) WithLogger(log logger.Logger) {
	s.cfg.SetDefault("GRPC_SERVER_LOGGER_ENABLED", true) // Enable logging for gRPC server
	isEnableLogger := s.cfg.GetBool("GRPC_SERVER_LOGGER_ENABLED")

	if isEnableLogger {
		s.interceptorStreamServerList = append(s.interceptorStreamServerList, grpc_logger.StreamServerInterceptor(log))
		s.interceptorUnaryServerList = append(s.interceptorUnaryServerList, grpc_logger.UnaryServerInterceptor(log))
	}
}

// WithTLS - setup TLS.
func (s *server) WithTLS() error {
	s.cfg.SetDefault("GRPC_SERVER_TLS_ENABLED", false) // gRPC tls
	isEnableTLS := s.cfg.GetBool("GRPC_SERVER_TLS_ENABLED")

	s.cfg.SetDefault("GRPC_SERVER_CERT_PATH", "ops/cert/shortlink-server.pem") // gRPC server cert
	certFile := s.cfg.GetString("GRPC_SERVER_CERT_PATH")

	s.cfg.SetDefault("GRPC_SERVER_KEY_PATH", "ops/cert/shortlink-server-key.pem") // gRPC server key
	keyFile := s.cfg.GetString("GRPC_SERVER_KEY_PATH")

	if isEnableTLS {
		creds, errTLSFromFile := credentials.NewServerTLSFromFile(certFile, keyFile)
		if errTLSFromFile != nil {
			return fmt.Errorf("failed to setup TLS: %w", errTLSFromFile)
		}

		s.optionsNewServer = append(s.optionsNewServer, grpc.Creds(creds))
	}

	return nil
}

// WithAuthHeaders - map Istio outputClaimToHeaders into context.
func (s *server) WithAuthHeaders() {
	s.cfg.SetDefault("GRPC_AUTH_HEADERS_ENABLED", true)
	if !s.cfg.GetBool("GRPC_AUTH_HEADERS_ENABLED") {
		return
	}

	s.interceptorUnaryServerList = append(
		s.interceptorUnaryServerList,
		session_interceptor.SessionUnaryServerInterceptor(),
	)
	s.interceptorStreamServerList = append(
		s.interceptorStreamServerList,
		session_interceptor.SessionStreamServerInterceptor(),
	)
}

// WithAuthJWT - setup JWT validation for gRPC server.
// Use only when Istio RequestAuthentication is not enforcing JWT.
func (s *server) WithAuthJWT() error {
	s.cfg.SetDefault("GRPC_AUTH_JWT_ENABLED", false)
	if !s.cfg.GetBool("GRPC_AUTH_JWT_ENABLED") {
		return nil
	}

	s.cfg.SetDefault("GRPC_AUTH_JWKS_CACHE_TTL", "1h")
	s.cfg.SetDefault("GRPC_AUTH_JWKS_HTTP_TIMEOUT", "10s")
	s.cfg.SetDefault("GRPC_AUTH_JWKS_BACKOFF_MIN", "500ms")
	s.cfg.SetDefault("GRPC_AUTH_JWKS_BACKOFF_MAX", "30s")
	s.cfg.SetDefault("GRPC_AUTH_JWT_LEEWAY", "30s")

	validator, err := authjwt.NewValidator(authjwt.ValidatorConfig{
		JWKSURL:         s.cfg.GetString("GRPC_AUTH_JWKS_URL"),
		Issuer:          s.cfg.GetString("GRPC_AUTH_JWT_ISSUER"),
		Audience:        s.cfg.GetString("GRPC_AUTH_JWT_AUDIENCE"),
		SkipAudience:    s.cfg.GetBool("GRPC_AUTH_JWT_SKIP_AUDIENCE"),
		SkipIssuer:      s.cfg.GetBool("GRPC_AUTH_JWT_SKIP_ISSUER"),
		Leeway:          s.cfg.GetDuration("GRPC_AUTH_JWT_LEEWAY"),
		JWKSCacheTTL:    s.cfg.GetDuration("GRPC_AUTH_JWKS_CACHE_TTL"),
		JWKSHTTPTimeout: s.cfg.GetDuration("GRPC_AUTH_JWKS_HTTP_TIMEOUT"),
		JWKSBackoffMin:  s.cfg.GetDuration("GRPC_AUTH_JWKS_BACKOFF_MIN"),
		JWKSBackoffMax:  s.cfg.GetDuration("GRPC_AUTH_JWKS_BACKOFF_MAX"),
	})
	if err != nil {
		return err
	}

	s.authValidator = validator

	s.interceptorUnaryServerList = append(
		s.interceptorUnaryServerList,
		authjwt.UnaryServerInterceptor(validator, authjwt.InterceptorConfig{Logger: s.log}),
	)
	s.interceptorStreamServerList = append(
		s.interceptorStreamServerList,
		authjwt.StreamServerInterceptor(validator, authjwt.InterceptorConfig{Logger: s.log}),
	)

	return nil
}

// WithAuthForward - capture validated token for downstream forwarding.
func (s *server) WithAuthForward() {
	s.interceptorUnaryServerList = append(
		s.interceptorUnaryServerList,
		authforward.UnaryServerInterceptor(),
	)
	s.interceptorStreamServerList = append(
		s.interceptorStreamServerList,
		authforward.StreamServerInterceptor(),
	)
}

// WithPprofLabels - setup pprof labels.
func (s *server) WithPprofLabels() {
	s.interceptorUnaryServerList = append(s.interceptorUnaryServerList, pprof_interceptor.UnaryServerInterceptor())
	s.interceptorStreamServerList = append(s.interceptorStreamServerList, pprof_interceptor.StreamServerInterceptor())
}

// WithFlightTrace - setup flight trace.
func (s *server) WithFlightTrace(flightRecorder *flight_trace.Recorder, log logger.Logger) {
	s.interceptorUnaryServerList = append(
		s.interceptorUnaryServerList,
		flight_trace_interceptor.UnaryServerInterceptor(flightRecorder, log, s.cfg),
	)
	s.interceptorStreamServerList = append(
		s.interceptorStreamServerList,
		flight_trace_interceptor.StreamServerInterceptor(flightRecorder, log, s.cfg),
	)
}
