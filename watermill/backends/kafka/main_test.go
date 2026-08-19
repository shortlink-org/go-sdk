package kafka_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	tc_kafka "github.com/testcontainers/testcontainers-go/modules/kafka"
)

//nolint:gocritic // the deferred cancel is intentionally skipped: os.Exit ends the process
func TestMain(m *testing.M) {
	//nolint:errcheck // cleanup path: there is nothing useful to do with the error
	_ = os.Setenv("WATERMILL_TEST_KAFKA_RETRY_INTERVAL", "50ms")
	//nolint:errcheck // cleanup path: there is nothing useful to do with the error
	_ = os.Setenv("WATERMILL_TEST_KAFKA_RETRY_MAX_INTERVAL", "500ms")
	//nolint:errcheck // cleanup path: there is nothing useful to do with the error
	_ = os.Setenv("WATERMILL_TEST_KAFKA_RETRY_MAX_RETRIES", "2")
	//nolint:errcheck // cleanup path: there is nothing useful to do with the error
	_ = os.Setenv("WATERMILL_TEST_HANDLER_TIMEOUT", "5s")

	if brokers := os.Getenv("WATERMILL_TEST_KAFKA_BROKERS"); brokers != "" {
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	container, err := tc_kafka.Run(ctx, "confluentinc/confluent-local:7.5.0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start kafka container: %v\n", err)
		os.Exit(1)
	}

	brokers, err := container.Brokers(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get kafka brokers: %v\n", err)

		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = container.Terminate(context.Background())

		os.Exit(1)
	}

	err = os.Setenv("WATERMILL_TEST_KAFKA_BROKERS", strings.Join(brokers, ","))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to set env: %v\n", err)

		//nolint:errcheck // cleanup path: there is nothing useful to do with the error
		_ = container.Terminate(context.Background())

		os.Exit(1)
	}

	code := m.Run()

	//nolint:errcheck // cleanup path: there is nothing useful to do with the error
	_ = container.Terminate(context.Background())

	os.Exit(code)
}
