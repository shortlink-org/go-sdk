package flight_trace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/trace"
	"sort"
	"time"

	"github.com/spf13/viper"
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
}

// New initializes the Flight Recorder and starts background cleanup.
func New(ctx context.Context) (*Recorder, error) {
	viper.SetDefault("FLIGHT_RECORDER_ENABLED", true)
	viper.SetDefault("FLIGHT_RECORDER_MIN_AGE", "1s")
	viper.SetDefault("FLIGHT_RECORDER_MAX_BYTES", 20*1024*1024)
	viper.SetDefault("FLIGHT_RECORDER_DUMP_PATH", "/tmp/flight_dumps")
	viper.SetDefault("FLIGHT_RECORDER_MAX_DUMPS", 100)
	viper.SetDefault("FLIGHT_RECORDER_CLEANUP_INTERVAL", "10m")

	if !viper.GetBool("FLIGHT_RECORDER_ENABLED") {
		return nil, nil
	}

	dumpPath := viper.GetString("FLIGHT_RECORDER_DUMP_PATH")
	if err := os.MkdirAll(dumpPath, 0o755); err != nil {
		return nil, ErrCreateDumpPath
	}

	fr := trace.NewFlightRecorder(trace.FlightRecorderConfig{
		MinAge:   viper.GetDuration("FLIGHT_RECORDER_MIN_AGE"),
		MaxBytes: viper.GetUint64("FLIGHT_RECORDER_MAX_BYTES"),
	})
	if err := fr.Start(); err != nil {
		return nil, ErrStartRecorder
	}

	rec := &Recorder{
		fr:       fr,
		dumpPath: dumpPath,
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		fr.Stop()
	}()

	// Periodic cleanup
	go rec.periodicCleanup(viper.GetDuration("FLIGHT_RECORDER_CLEANUP_INTERVAL"))

	return rec, nil
}

// DumpSnapshot writes a trace snapshot to a file.
func (wr *Recorder) DumpSnapshot(fileNameSuffix string) (string, error) {
	if wr == nil {
		return "", ErrRecorderNotInitialized
	}

	if wr.fr == nil || !wr.fr.Enabled() {
		return "", ErrRecorderNotStarted
	}

	filePath := filepath.Join(
		wr.dumpPath,
		fmt.Sprintf("flight-%s-%d.out", fileNameSuffix, time.Now().UnixNano()),
	)

	f, err := os.Create(filePath)
	if err != nil {
		return "", ErrCreateDumpFile
	}
	defer f.Close()

	if _, err = wr.fr.WriteTo(f); err != nil {
		return "", ErrWriteDump
	}

	go wr.cleanupOldDumps()

	return filePath, nil
}

// DumpSnapshotAsync creates a dump in background.
func (wr *Recorder) DumpSnapshotAsync(suffix string) {
	go func() {
		_, _ = wr.DumpSnapshot(suffix)
	}()
}

// cleanupOldDumps keeps only the N newest dump files, deleting older ones.
func (wr *Recorder) cleanupOldDumps() {
	maxDumps := viper.GetInt("FLIGHT_RECORDER_MAX_DUMPS")
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
