package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHelpFlag verifies that --help prints usage and exits 0.
func TestHelpFlag(t *testing.T) {
	binary := buildTestBinary(t)

	out, err := runBinary(t, binary, "--help")
	if err != nil {
		t.Fatalf("--help exited with error: %v\noutput: %s", err, out)
	}

	for _, want := range []string{"GORT", "GitOps Reconciliation Tool", "GORT_GITHUB_WEBHOOK_SECRET", "GORT_GITHUB_TOKEN"} {
		if !strings.Contains(out, want) {
			t.Errorf("--help output missing %q", want)
		}
	}
}

// TestVersionFlag verifies that --version prints a version string and exits 0.
func TestVersionFlag(t *testing.T) {
	binary := buildTestBinary(t)

	out, err := runBinary(t, binary, "--version")
	if err != nil {
		t.Fatalf("--version exited with error: %v\noutput: %s", err, out)
	}

	if !strings.Contains(out, "gort version") {
		t.Errorf("--version output missing 'gort version', got: %s", out)
	}
	if !strings.Contains(out, "test-build") {
		t.Errorf("--version output missing injected version 'test-build', got: %s", out)
	}
}

// buildTestBinary compiles the gort binary to a temp location for flag testing.
func buildTestBinary(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "gort")
	cmd := exec.Command("go", "build", //nolint:gosec // binary is a t.TempDir() output path, not user input
		"-ldflags=-X main.version=test-build",
		"-o", binary,
		".",
	)
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build test binary: %v\noutput: %s", err, out)
	}
	return binary
}

// runBinary executes the test binary with the given argument and returns the output.
func runBinary(t *testing.T, binary, arg string) (string, error) {
	t.Helper()

	cmd := exec.Command(binary, arg) //nolint:gosec // binary is built by buildTestBinary from our own source
	out, err := cmd.CombinedOutput()
	return string(out), err
}
