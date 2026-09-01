//go:build unit

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// requireProtoc prepares the environment this generator test needs: the protoc
// binary itself, plus a freshly built protoc-gen-rich-model on PATH.
//
// protoc is an external tool that neither `go test` nor CI installs, so a
// missing binary is skipped rather than failed — otherwise the test reports a
// broken generator when nothing is wrong with it. The plugin is built from the
// current source instead of relying on whatever is installed, so the test
// always exercises the code under test.
func requireProtoc(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("protoc"); err != nil {
		t.Skip("protoc not found in PATH; skipping generator test")
	}

	binDir := t.TempDir()

	build := exec.Command("go", "build", "-o", filepath.Join(binDir, "protoc-gen-rich-model"), ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build protoc-gen-rich-model: %v\n%s", err, out)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
