package flight_trace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime/trace"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shortlink-org/go-sdk/config"
)

// defaultMaxDumps is how many dump files are kept when the setting is unset or invalid.
const defaultMaxDumps = 100

// defaultMaxBytes is the flight recorder buffer size (20 MB) when the setting is unset.
const defaultMaxBytes = 20 * 1024 * 1024

// dumpDirPerm keeps dump files readable by the owner and its group only.
const dumpDirPerm = 0o750

var (
	ErrRecorderNotInitialized = errors.New("flight recorder not initialized")
	ErrRecorderNotStarted     = errors.New("flight recorder not started")
	ErrCreateDumpPath         = errors.New("failed to create dump path")
	ErrCreateDumpFile         = errors.New("failed to create dump file")
	ErrInvalidDumpFileName    = errors.New("dump file name escapes the dump path")
	ErrWriteDump              = errors.New("failed to write flight dump")
	ErrStartRecorder          = errors.New("failed to start flight recorder")
)

// Recorder manages Go's Flight Recorder lifecycle and dump rotation.
type Recorder struct {
	fr       *trace.FlightRecorder
	dumpPath string

	// maxDumps is read once at construction: the cleanup runs on background
	// goroutines, and config reads reach into a process-wide viper that another
	// goroutine may be writing.
	maxDumps int

	// mu serializes every use of fr. Stop resets state that WriteTo reads, and
	// the recorder rejects a WriteTo that overlaps another one, so both the
	// shutdown and the dumps have to take turns.
	mu      sync.Mutex
	stopped bool
}

// New initializes the Flight Recorder and starts background cleanup.
func New(ctx context.Context, cfg *config.Config) (*Recorder, error) {
	cfg.SetDefault("FLIGHT_RECORDER_ENABLED", true)
	cfg.SetDefault("FLIGHT_RECORDER_MIN_AGE", "1s")
	cfg.SetDefault("FLIGHT_RECORDER_MAX_BYTES", defaultMaxBytes)
	cfg.SetDefault("FLIGHT_RECORDER_DUMP_PATH", "/tmp/flight_dumps")
	cfg.SetDefault("FLIGHT_RECORDER_MAX_DUMPS", defaultMaxDumps)
	cfg.SetDefault("FLIGHT_RECORDER_CLEANUP_INTERVAL", "10m")

	if !cfg.GetBool("FLIGHT_RECORDER_ENABLED") {
		// A disabled recorder is a nil *Recorder, not an error: the HTTP and gRPC
		// middleware check for nil and skip dumping, and every method tolerates
		// a nil receiver.
		return nil, nil //nolint:nilnil // nil recorder is the documented "disabled" value the middleware relies on
	}

	dumpPath := cfg.GetString("FLIGHT_RECORDER_DUMP_PATH")

	err := os.MkdirAll(dumpPath, dumpDirPerm)
	if err != nil {
		return nil, ErrCreateDumpPath
	}

	recorder := trace.NewFlightRecorder(trace.FlightRecorderConfig{
		MinAge:   cfg.GetDuration("FLIGHT_RECORDER_MIN_AGE"),
		MaxBytes: cfg.GetUint64("FLIGHT_RECORDER_MAX_BYTES"),
	})

	err = recorder.Start()
	if err != nil {
		return nil, ErrStartRecorder
	}

	maxDumps := cfg.GetInt("FLIGHT_RECORDER_MAX_DUMPS")
	if maxDumps <= 0 {
		maxDumps = defaultMaxDumps
	}

	rec := &Recorder{
		fr:       recorder,
		dumpPath: dumpPath,
		maxDumps: maxDumps,
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		rec.stop()
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

	wr.mu.Lock()
	defer wr.mu.Unlock()

	if wr.fr == nil || wr.stopped || !wr.fr.Enabled() {
		return ErrRecorderNotStarted
	}

	// Clean is already applied by Join; repeating it here keeps the path
	// provably sanitized for the reader (and gosec) at the point of use.
	path := filepath.Clean(filepath.Join(wr.dumpPath, fileName))
	if !wr.insideDumpPath(path) {
		return ErrInvalidDumpFileName
	}

	file, err := os.Create(path)
	if err != nil {
		return ErrCreateDumpFile
	}

	_, err = wr.fr.WriteTo(file)
	closeErr := file.Close()

	if err != nil || closeErr != nil {
		return ErrWriteDump
	}

	go wr.cleanupOldDumps()

	return nil
}

// insideDumpPath reports whether a cleaned path is a file strictly below the
// dump directory, so a file name such as "../etc/passwd" cannot escape it.
func (wr *Recorder) insideDumpPath(path string) bool {
	rel, err := filepath.Rel(filepath.Clean(wr.dumpPath), path)
	if err != nil {
		return false
	}

	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// stop ends the recording. It is idempotent, and it waits for a dump that is
// already in flight instead of pulling the buffer out from under it.
func (wr *Recorder) stop() {
	wr.mu.Lock()
	defer wr.mu.Unlock()

	if wr.fr == nil || wr.stopped {
		return
	}

	wr.stopped = true
	wr.fr.Stop()
}

// DumpToFileAsync runs DumpToFile asynchronously. The result is not reported
// anywhere: the caller chose fire-and-forget, and this package has no logger.
func (wr *Recorder) DumpToFileAsync(fileName string) {
	go func() {
		_ = wr.DumpToFile(fileName) //nolint:errcheck // fire-and-forget by contract; no channel or logger to report the error through
	}()
}

// dumpFile is a dump on disk with the modification time the rotation sorts by.
type dumpFile struct {
	path    string
	modTime time.Time
}

// cleanupOldDumps keeps only the N newest dump files, deleting older ones.
func (wr *Recorder) cleanupOldDumps() {
	paths, err := filepath.Glob(filepath.Join(wr.dumpPath, "flight-*.out"))
	if err != nil || len(paths) <= wr.maxDumps {
		return
	}

	dumps := make([]dumpFile, 0, len(paths))

	for _, path := range paths {
		info, statErr := os.Stat(path)
		if statErr != nil {
			// Removed between Glob and Stat; nothing left to rotate.
			continue
		}

		dumps = append(dumps, dumpFile{path: path, modTime: info.ModTime()})
	}

	if len(dumps) <= wr.maxDumps {
		return
	}

	sort.Slice(dumps, func(i, j int) bool {
		return dumps[i].modTime.After(dumps[j].modTime) // newest first
	})

	for _, old := range dumps[wr.maxDumps:] {
		_ = os.Remove(old.path) //nolint:errcheck // best effort: a file that cannot be removed now is retried on the next cleanup
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
