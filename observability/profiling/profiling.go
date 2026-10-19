package profiling

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"runtime"

	"github.com/felixge/fgprof"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/config"
	http_server "github.com/shortlink-org/go-sdk/http/server"
)

// defaultProfilingPort is the port the pprof server listens on when PROFILING_PORT is unset.
const defaultProfilingPort = 7071

// mutexProfileFraction samples 1 in N mutex contention events.
const mutexProfileFraction = 10

// blockProfileRate samples one blocking event per N nanoseconds blocked.
const blockProfileRate = 10

type PprofEndpoint *http.ServeMux

func New(ctx context.Context, log *slog.Logger, tracer trace.TracerProvider, cfg *config.Config) (PprofEndpoint, error) {
	cfg.SetDefault("PROFILING_PORT", defaultProfilingPort)
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

		server := http_server.New(ctx, mux, serverCfg, cfg)

		err := server.ListenAndServe()
		if err != nil {
			log.Error(err.Error())
		}
	}()

	log.Info("pprof server started",
		slog.String("addr", "0.0.0.0:7071"),
	)

	runtime.SetMutexProfileFraction(mutexProfileFraction)
	runtime.SetBlockProfileRate(blockProfileRate)

	return mux, nil
}
