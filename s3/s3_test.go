//go:build unit || (media && s3)

package s3

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.uber.org/goleak"

	"github.com/shortlink-org/go-sdk/config"
	"github.com/shortlink-org/go-sdk/logger"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)

	os.Exit(m.Run())
}

func TestMinio(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	log, err := logger.New(logger.Configuration{})
	require.NoError(t, err, "Error init a logger")

	cfg, err := config.New()
	require.NoError(t, err, "Error init config")

	client := &Client{}

	c, err := testcontainers.Run(ctx, "minio/minio:RELEASE.2023-12-23T07-19-11Z",
		testcontainers.WithCmd("server", "--address", ":9000", "/data"),
		testcontainers.WithEnv(map[string]string{
			"MINIO_ROOT_USER":     "minio_access_key",
			"MINIO_ROOT_PASSWORD": "minio_secret_key",
		}),
		testcontainers.WithExposedPorts("9000/tcp"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/minio/health/live").
				WithPort("9000/tcp").
				WithStartupTimeout(3*time.Minute),
		),
	)
	require.NoError(t, err, "Could not start resource")

	t.Cleanup(func() {
		cancel()
		_ = c.Terminate(context.Background())

		err := os.Remove("./fixtures/download.json")
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	})

	host, err := c.Host(ctx)
	require.NoError(t, err)
	mapped, err := c.MappedPort(ctx, "9000/tcp")
	require.NoError(t, err)
	endpoint := fmt.Sprintf("%s:%s", host, mapped.Port())
	cfg.Set("S3_ENDPOINT", endpoint)

	require.Eventually(t, func() bool {
		url := fmt.Sprintf("http://%s/minio/health/live", endpoint)
		resp, err := http.Get(url)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return false
		}
		var errNew error
		client, errNew = New(ctx, log, cfg)
		return errNew == nil
	}, 3*time.Minute, time.Second, "minio ready")

	t.Run("UploadFile", func(t *testing.T) {
		err := client.CreateBucket(ctx, "test", minio.MakeBucketOptions{})
		if err != nil {
			t.Fatal(err)
		}

		file, err := os.Open("./fixtures/test.json")
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			err := file.Close()
			if err != nil {
				t.Fatal(err)
			}
		}()

		data, err := io.ReadAll(file)
		require.NoError(t, err, "Error reading file")

		reader := bytes.NewReader(data)

		err = client.UploadFile(ctx, "test", "test", reader)
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("DownloadFile", func(t *testing.T) {
		err := client.DownloadFile(ctx, "test", "test", "./fixtures/download.json")
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ListFiles", func(t *testing.T) {
		files, err := client.ListFiles(ctx, "test")
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, []string{"test"}, files)
	})

	t.Run("FileExists", func(t *testing.T) {
		exists, err := client.FileExists(ctx, "test", "test")
		if err != nil {
			t.Fatal(err)
		}

		require.Equal(t, true, exists)
	})

	t.Run("DeleteFile", func(t *testing.T) {
		err := client.RemoveFile(ctx, "test", "test")
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("FileNoExists", func(t *testing.T) {
		exists, err := client.FileExists(ctx, "test", "test")
		if err != nil {
			require.Equal(t, "The specified key does not exist.", err.Error())
		}

		require.Equal(t, false, exists)
	})

	t.Run("RemoveBucket", func(t *testing.T) {
		err := client.RemoveBucket(ctx, "test")
		if err != nil {
			t.Fatal(err)
		}
	})
}
