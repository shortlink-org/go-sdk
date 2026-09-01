//go:build unit

package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireProtoc prepares the environment the generator tests need: the protoc
// binary itself, plus a freshly built protoc-gen-go-orm on PATH.
//
// protoc is an external tool that neither `go test` nor CI installs, so a
// missing binary is skipped rather than failed — otherwise these tests report a
// broken generator when nothing is wrong with it. The plugin is built from the
// current source instead of relying on whatever is installed, so the tests
// always exercise the code under test.
func requireProtoc(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc not found in PATH; skipping generator test")
	}

	binDir := t.TempDir()

	build := exec.Command("go", "build", "-o", filepath.Join(binDir, "protoc-gen-go-orm"), ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build protoc-gen-go-orm: %v\n%s", err, out)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
