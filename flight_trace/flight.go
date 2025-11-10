package flight

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/trace"
	"time"

	"github.com/spf13/viper"
)

// Recorder - structure for flight recorder
type Recorder struct {
	fr       *trace.FlightRecorder
	dumpPath string
}

func New() (*Recorder, error) {
	if !viper.GetBool("FLIGHT_RECORDER_ENABLED") {
		return nil, nil // disabled
	}

	dumpPath := viper.GetString("FLIGHT_RECORDER_DUMP_PATH")

	// ensure dump path exists
	if err := os.MkdirAll(dumpPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create flight recorder dump path %q: %w", dumpPath, err)
	}

	fr := trace.NewFlightRecorder(trace.FlightRecorderConfig{
		MinAge:   viper.GetDuration("FLIGHT_RECORDER_MIN_AGE"),
		MaxBytes: viper.GetUint64("FLIGHT_RECORDER_MAX_BYTES"),
	})
	if err := fr.Start(); err != nil {
		return nil, fmt.Errorf("failed to start flight recorder: %w", err)
	}

	return &Recorder{
		fr:       fr,
		dumpPath: dumpPath,
	}, nil
}

// DumpSnapshot triggers a dump to a file in the configured dumpPath
func (wr *Recorder) DumpSnapshot(fileNameSuffix string) (string, error) {
	if wr == nil || wr.fr == nil || !wr.fr.Enabled() {
		return "", fmt.Errorf("flight recorder not enabled or started")
	}
	// compose file name
	filePath := filepath.Join(wr.dumpPath, fmt.Sprintf("flight-%s-%d.out", fileNameSuffix, time.Now().UnixNano()))
	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create flight dump file %q: %w", filePath, err)
	}
	defer f.Close()

	_, err = wr.fr.WriteTo(f)
	if err != nil {
		return "", fmt.Errorf("failed to write flight dump: %w", err)
	}

	return filePath, nil
}

// Stop stops the flight recorder when shutting down
func (wr *Recorder) Stop() error {
	if wr == nil || wr.fr == nil {
		return fmt.Errorf("flight recorder not initialized")
	}

	wr.fr.Stop()

	return nil
}
