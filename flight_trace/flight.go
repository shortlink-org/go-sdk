package flight_trace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime/trace"
	"sort"
	"time"

	"github.com/shortlink-org/go-sdk/config"
)

var (
	ErrRecorderNotInitialized = errors.New("flight recorder not initialized")
	ErrRecorderNotStarted     = errors.New("flight recorder not started")
	ErrCreateDumpPath         = errors.New("failed to create dump path")
	ErrCreateDumpFile         = errors.New("failed to create dump file")
	ErrWriteDump              = errors.New("failed to write flight dump")
	ErrStartRecorder          = errors.New("failed to start flight recorder")
)

// Recorder manages Go's Flight Recorder lifecycle and dump rotation.
type Recorder struct {
	fr       *trace.FlightRecorder
	dumpPath string
	cfg      *config.Config
}

// New initializes the Flight Recorder and starts background cleanup.
func New(ctx context.Context, cfg *config.Config) (*Recorder, error) {
	cfg.SetDefault("FLIGHT_RECORDER_ENABLED", true)
	cfg.SetDefault("FLIGHT_RECORDER_MIN_AGE", "1s")
	cfg.SetDefault("FLIGHT_RECORDER_MAX_BYTES", 20*1024*1024)
	cfg.SetDefault("FLIGHT_RECORDER_DUMP_PATH", "/tmp/flight_dumps")
	cfg.SetDefault("FLIGHT_RECORDER_MAX_DUMPS", 100)
	cfg.SetDefault("FLIGHT_RECORDER_CLEANUP_INTERVAL", "10m")

	if !cfg.GetBool("FLIGHT_RECORDER_ENABLED") {
		return nil, nil
	}

	dumpPath := cfg.GetString("FLIGHT_RECORDER_DUMP_PATH")
	if err := os.MkdirAll(dumpPath, 0o755); err != nil {
		return nil, ErrCreateDumpPath
	}

	fr := trace.NewFlightRecorder(trace.FlightRecorderConfig{
		MinAge:   cfg.GetDuration("FLIGHT_RECORDER_MIN_AGE"),
		MaxBytes: cfg.GetUint64("FLIGHT_RECORDER_MAX_BYTES"),
	})
	if err := fr.Start(); err != nil {
		return nil, ErrStartRecorder
	}

	rec := &Recorder{
		fr:       fr,
		dumpPath: dumpPath,
		cfg:      cfg,
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		fr.Stop()
	}()

	// Periodic cleanup
	go rec.periodicCleanup(cfg.GetDuration("FLIGHT_RECORDER_CLEANUP_INTERVAL"))

	return rec, nil
}

// DumpToFile writes the flight recorder buffer into a file under dumpPath.
func (wr *Recorder) DumpToFile(fileName string) error {
	if wr == nil {
		return ErrRecorderNotInitialized
	}

	if wr.fr == nil || !wr.fr.Enabled() {
		return ErrRecorderNotStarted
	}

	f, err := os.Create(filepath.Join(wr.dumpPath, fileName))
	if err != nil {
		return ErrCreateDumpFile
	}
	defer f.Close()

	if _, err = wr.fr.WriteTo(f); err != nil {
		return ErrWriteDump
	}

	go wr.cleanupOldDumps()

	return nil
}

// DumpToFileAsync runs DumpToFile asynchronously.
func (wr *Recorder) DumpToFileAsync(fileName string) {
	go func() {
		_ = wr.DumpToFile(fileName)
	}()
}

// cleanupOldDumps keeps only the N newest dump files, deleting older ones.
func (wr *Recorder) cleanupOldDumps() {
	maxDumps := wr.cfg.GetInt("FLIGHT_RECORDER_MAX_DUMPS")
	if maxDumps <= 0 {
		maxDumps = 100
	}

	files, err := filepath.Glob(filepath.Join(wr.dumpPath, "flight-*.out"))
	if err != nil || len(files) <= maxDumps {
		return
	}

	sort.Slice(files, func(i, j int) bool {
		fi, _ := os.Stat(files[i])
		fj, _ := os.Stat(files[j])
		return fi.ModTime().After(fj.ModTime()) // newest first
	})

	for _, old := range files[maxDumps:] {
		_ = os.Remove(old)
	}
}

// periodicCleanup periodically enforces dump rotation even if no new dumps are made.
func (wr *Recorder) periodicCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		wr.cleanupOldDumps()
	}
}
