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

// TestEnvWithDeprecatedFallback_NewKey verifies that the new env var name is used
// when set, without any deprecation warning.
func TestEnvWithDeprecatedFallback_NewKey(t *testing.T) {
	binary := buildTestBinary(t)

	out, _ := runBinaryWithEnv(t, binary, "", map[string]string{
		"GORT_GITHUB_WEBHOOK_SECRET": "new-secret",
		"GORT_GITHUB_TOKEN":          "fake-token",
		"GORT_CLAUDE_API_KEY":        "fake-key",
	})

	if strings.Contains(out, "deprecated") {
		t.Error("should not log deprecation warning when new key is set")
	}
	// Should get past loadConfig (fail later on kubeconfig, not on missing env var).
	if strings.Contains(out, "required environment variable not set") {
		t.Error("should not report missing env var when new key is set")
	}
}

// TestEnvWithDeprecatedFallback_OldKey verifies that the old env var name works
// as a fallback and emits a deprecation warning.
func TestEnvWithDeprecatedFallback_OldKey(t *testing.T) {
	binary := buildTestBinary(t)

	out, _ := runBinaryWithEnv(t, binary, "", map[string]string{
		"GORT_WEBHOOK_SECRET": "old-secret",
		"GORT_GITHUB_TOKEN":   "fake-token",
		"GORT_CLAUDE_API_KEY": "fake-key",
	})

	if !strings.Contains(out, "deprecated") {
		t.Error("should log deprecation warning when only old key is set")
	}
	if strings.Contains(out, "required environment variable not set") {
		t.Error("should not report missing env var when old key is set")
	}
}

// TestEnvWithDeprecatedFallback_NeitherKey verifies that the binary exits with
// an error when neither the new nor old env var is set.
func TestEnvWithDeprecatedFallback_NeitherKey(t *testing.T) {
	binary := buildTestBinary(t)

	out, err := runBinaryWithEnv(t, binary, "", map[string]string{
		"GORT_GITHUB_TOKEN":   "fake-token",
		"GORT_CLAUDE_API_KEY": "fake-key",
	})

	if err == nil {
		t.Fatal("expected non-zero exit when neither key is set")
	}
	if !strings.Contains(out, "GORT_GITHUB_WEBHOOK_SECRET") {
		t.Errorf("error should reference GORT_GITHUB_WEBHOOK_SECRET, got: %s", out)
	}
}

// runBinary executes the test binary with the given argument and returns the output.
func runBinary(t *testing.T, binary, arg string) (string, error) {
	t.Helper()

	cmd := exec.Command(binary, arg) //nolint:gosec // binary is built by buildTestBinary from our own source
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// runBinaryWithEnv executes the test binary with a given argument and custom environment.
func runBinaryWithEnv(t *testing.T, binary, arg string, env map[string]string) (string, error) {
	t.Helper()

	var args []string
	if arg != "" {
		args = append(args, arg)
	}
	cmd := exec.Command(binary, args...) //nolint:gosec // binary is built by buildTestBinary from our own source
	// Start with a clean environment to avoid inheriting any GORT_ vars.
	cmd.Env = []string{}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}
