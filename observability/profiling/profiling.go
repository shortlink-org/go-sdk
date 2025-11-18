package profiling

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"runtime"

	"github.com/felixge/fgprof"
	"github.com/spf13/viper"

	http_server "github.com/shortlink-org/go-sdk/http/server"
	"github.com/shortlink-org/go-sdk/logger"
)

type PprofEndpoint *http.ServeMux

func New(ctx context.Context, log logger.Logger) (PprofEndpoint, error) {
	viper.SetDefault("PROFILING_PORT", 7071)
	viper.SetDefault("PROFILING_TIMEOUT", "30s")

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
		cfg := http_server.Config{
			Port:    viper.GetInt("PROFILING_PORT"),
			Timeout: viper.GetDuration("PROFILING_TIMEOUT"),
		}

		server := http_server.New(ctx, mux, cfg, nil)
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
