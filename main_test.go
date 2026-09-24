package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var (
	testWorkDir string
	cliPath     string
)

func TestMain(m *testing.M) {
	var err error
	testWorkDir, err = os.MkdirTemp(".", ".rrtee-test-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cliPath = filepath.Join(testWorkDir, "rrtee")
	build := exec.Command("go", "build", "-o", cliPath, ".")
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build rrtee: %v\n%s", err, output)
		_ = os.RemoveAll(testWorkDir)
		os.Exit(1)
	}

	exitCode := m.Run()
	_ = os.RemoveAll(testWorkDir)
	os.Exit(exitCode)
}

type commandResult struct {
	stdout string
	stderr string
	err    error
}

func runCLI(t *testing.T, input string, args ...string) commandResult {
	t.Helper()

	cmd := exec.Command(cliPath, args...)
	cmd.Stdin = strings.NewReader(input)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	return commandResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}

func testPath(t *testing.T, name string) string {
	t.Helper()

	path, err := os.MkdirTemp(testWorkDir, name+"-")
	if err != nil {
		t.Fatalf("create test directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(path) })
	return path
}

func writeConfig(t *testing.T, directory, content string) string {
	t.Helper()

	path := filepath.Join(directory, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func TestDefaultANSICleanupPreservesRawStandardOutput(t *testing.T) {
	directory := testPath(t, "ansi")
	output := filepath.Join(directory, "captured.log")
	raw := "\x1b[1;31mred\x1b[0m and \x1b[34mblue\x1b[0m\n"

	result := runCLI(t, raw, output)

	if result.err != nil {
		t.Fatalf("rrtee failed: %v\nstderr: %s", result.err, result.stderr)
	}
	if result.stdout != raw {
		t.Errorf("stdout = %q, want raw input %q", result.stdout, raw)
	}
	if got, want := readFile(t, output), "red and blue\n"; got != want {
		t.Errorf("captured output = %q, want %q", got, want)
	}
}

func TestRulesAreAppliedInConfigurationOrder(t *testing.T) {
	directory := testPath(t, "ordered-rules")
	config := writeConfig(t, directory, `
[[rules]]
name = "first-to-second"
from = "first"
to = "second"

[[rules]]
name = "second-to-third"
from = "second"
to = "third"
`)
	output := filepath.Join(directory, "captured.log")

	result := runCLI(t, "first\n", "--config", config, output)

	if result.err != nil {
		t.Fatalf("rrtee failed: %v\nstderr: %s", result.err, result.stderr)
	}
	if got, want := readFile(t, output), "third\n"; got != want {
		t.Errorf("captured output = %q, want %q", got, want)
	}
}

func TestMalformedRegexReturnsUsefulErrorWithoutWritingOutput(t *testing.T) {
	directory := testPath(t, "invalid-regex")
	config := writeConfig(t, directory, `
[[rules]]
name = "broken"
from = "["
to = ""
`)
	output := filepath.Join(directory, "captured.log")

	result := runCLI(t, "input\n", "--config", config, output)

	if result.err == nil {
		t.Fatal("rrtee succeeded with a malformed regular expression")
	}
	if !strings.Contains(result.stderr, "invalid regex") {
		t.Errorf("stderr = %q, want an invalid regex error", result.stderr)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("output file exists or could not be checked after invalid config: %v", err)
	}
}

func TestPreviewKeepsRawStandardOutputAndDoesNotCreateDestination(t *testing.T) {
	directory := testPath(t, "preview")
	output := filepath.Join(directory, "captured.log")
	raw := "\x1b[32mready\x1b[0m\n"

	result := runCLI(t, raw, "--preview", output)

	if result.err != nil {
		t.Fatalf("preview failed: %v\nstderr: %s", result.err, result.stderr)
	}
	if result.stdout != raw {
		t.Errorf("preview stdout = %q, want raw input %q", result.stdout, raw)
	}
	if !strings.Contains(result.stderr, "rrtee: preview: ready\n") {
		t.Errorf("preview stderr = %q, want transformed preview", result.stderr)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("preview created destination or could not be checked: %v", err)
	}
}

func TestExistingOutputRequiresForceAndForceReplacesIt(t *testing.T) {
	directory := testPath(t, "overwrite")
	output := filepath.Join(directory, "captured.log")
	if err := os.WriteFile(output, []byte("existing\n"), 0o600); err != nil {
		t.Fatalf("create existing output: %v", err)
	}

	result := runCLI(t, "new\n", output)
	if result.err == nil {
		t.Fatal("rrtee overwrote an existing output without --force")
	}
	if got, want := readFile(t, output), "existing\n"; got != want {
		t.Errorf("output after rejected overwrite = %q, want %q", got, want)
	}

	result = runCLI(t, "\x1b[33mnew\x1b[0m\n", "--overwrite", "--force", output)
	if result.err != nil {
		t.Fatalf("forced overwrite failed: %v\nstderr: %s", result.err, result.stderr)
	}
	if got, want := readFile(t, output), "new\n"; got != want {
		t.Errorf("output after forced overwrite = %q, want %q", got, want)
	}
}
