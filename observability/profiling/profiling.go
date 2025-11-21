package profiling

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"runtime"

	"github.com/felixge/fgprof"

	http_server "github.com/shortlink-org/go-sdk/http/server"
	"github.com/shortlink-org/go-sdk/logger"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
)

type PprofEndpoint *http.ServeMux

func New(ctx context.Context, log logger.Logger, tracer trace.TracerProvider, cfg *config.Config) (PprofEndpoint, error) {
	cfg.SetDefault("PROFILING_PORT", 7071)
	cfg.SetDefault("PROFILING_TIMEOUT", "30s")

	mux := http.NewServeMux()

	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/allocs", pprof.Handler("allocs"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	mux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))

	mux.Handle("/debug/pprof/fgprof", fgprof.Handler())

	go func() {
		serverCfg := http_server.Config{
			Port:    cfg.GetInt("PROFILING_PORT"),
			Timeout: cfg.GetDuration("PROFILING_TIMEOUT"),
		}

		server := http_server.New(ctx, mux, serverCfg, tracer, cfg)
		if err := server.ListenAndServe(); err != nil {
			log.Error(err.Error())
		}
	}()

	log.Info("pprof server started",
		slog.String("addr", "0.0.0.0:7071"),
	)

	runtime.SetMutexProfileFraction(10)
	runtime.SetBlockProfileRate(10)

	return mux, nil
}
