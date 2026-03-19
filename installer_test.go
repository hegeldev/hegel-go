package hegel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureHegelInstalledCachedVersion(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Set up a fake cached installation.
	hegelDir := filepath.Join(tmp, ".hegel")
	venvDir := filepath.Join(hegelDir, "venv")
	binDir := filepath.Join(venvDir, "bin")
	os.MkdirAll(binDir, 0o755)                                                               //nolint:errcheck
	os.WriteFile(filepath.Join(binDir, "hegel"), []byte("#!/bin/sh\n"), 0o755)               //nolint:errcheck
	os.WriteFile(filepath.Join(venvDir, "hegel-version"), []byte(hegelServerVersion), 0o644) //nolint:errcheck

	result, err := ensureHegelInstalled()
	if err != nil {
		t.Fatalf("ensureHegelInstalled() error: %v", err)
	}
	expected := filepath.Join(binDir, "hegel")
	if result != expected {
		t.Errorf("ensureHegelInstalled() = %q, want %q", result, expected)
	}
}

func TestEnsureHegelInstalledVersionMismatch(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Set up a cached installation with wrong version.
	hegelDir := filepath.Join(tmp, ".hegel")
	venvDir := filepath.Join(hegelDir, "venv")
	binDir := filepath.Join(venvDir, "bin")
	os.MkdirAll(binDir, 0o755)                                                        //nolint:errcheck
	os.WriteFile(filepath.Join(binDir, "hegel"), []byte("#!/bin/sh\n"), 0o755)        //nolint:errcheck
	os.WriteFile(filepath.Join(venvDir, "hegel-version"), []byte("0.0.0-old"), 0o644) //nolint:errcheck

	// Mock uv to fail so we can verify the version check triggers reinstall.
	origUv := uvLookPathFn
	uvLookPathFn = func() (string, error) {
		return "", fmt.Errorf("uv not found")
	}
	defer func() { uvLookPathFn = origUv }()

	_, err := ensureHegelInstalled()
	if err == nil {
		t.Fatal("expected error when uv not found")
	}
	if !strings.Contains(err.Error(), "uv not found") {
		t.Errorf("expected uv error, got: %v", err)
	}
}

func TestEnsureHegelInstalledBinaryMissing(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Set up cached version file but no binary.
	hegelDir := filepath.Join(tmp, ".hegel")
	venvDir := filepath.Join(hegelDir, "venv")
	os.MkdirAll(venvDir, 0o755)                                                              //nolint:errcheck
	os.WriteFile(filepath.Join(venvDir, "hegel-version"), []byte(hegelServerVersion), 0o644) //nolint:errcheck

	// Mock uv to fail to verify reinstall is triggered.
	origUv := uvLookPathFn
	uvLookPathFn = func() (string, error) {
		return "", fmt.Errorf("uv not found")
	}
	defer func() { uvLookPathFn = origUv }()

	_, err := ensureHegelInstalled()
	if err == nil {
		t.Fatal("expected error when binary missing and uv unavailable")
	}
}

func TestEnsureHegelInstalledNoUv(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	origUv := uvLookPathFn
	uvLookPathFn = func() (string, error) {
		return "", fmt.Errorf("uv not found")
	}
	defer func() { uvLookPathFn = origUv }()

	_, err := ensureHegelInstalled()
	if err == nil {
		t.Fatal("expected error when uv not found")
	}
	if !strings.Contains(err.Error(), "uv not found") {
		t.Errorf("expected uv error, got: %v", err)
	}
	if !strings.Contains(err.Error(), hegelServerCommandEnv) {
		t.Errorf("expected env var hint in error, got: %v", err)
	}
}

func TestEnsureHegelInstalledUvVenvFails(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Use "false" as uv — it exits with code 1.
	falseBin, err := exec.LookPath("false")
	if err != nil {
		t.Skip("false binary not available")
	}
	origUv := uvLookPathFn
	uvLookPathFn = func() (string, error) {
		return falseBin, nil
	}
	defer func() { uvLookPathFn = origUv }()

	_, err = ensureHegelInstalled()
	if err == nil {
		t.Fatal("expected error when uv venv fails")
	}
	if !strings.Contains(err.Error(), "uv venv failed") {
		t.Errorf("expected venv failure error, got: %v", err)
	}
}

func TestEnsureHegelInstalledPipInstallFails(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Create a script that succeeds on "venv" but fails on "pip".
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "fake_uv")
	script := `#!/bin/sh
if [ "$1" = "venv" ]; then
    mkdir -p "$3/bin"
    exit 0
fi
exit 1
`
	os.WriteFile(scriptPath, []byte(script), 0o755) //nolint:errcheck

	origUv := uvLookPathFn
	uvLookPathFn = func() (string, error) {
		return scriptPath, nil
	}
	defer func() { uvLookPathFn = origUv }()

	_, err := ensureHegelInstalled()
	if err == nil {
		t.Fatal("expected error when pip install fails")
	}
	if !strings.Contains(err.Error(), "failed to install") {
		t.Errorf("expected pip install failure error, got: %v", err)
	}
}

func TestEnsureHegelInstalledBinaryNotCreated(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Create a script that succeeds on both commands but doesn't create the binary.
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "fake_uv")
	script := `#!/bin/sh
if [ "$1" = "venv" ]; then
    mkdir -p "$3/bin"
    exit 0
fi
if [ "$1" = "pip" ]; then
    exit 0
fi
exit 1
`
	os.WriteFile(scriptPath, []byte(script), 0o755) //nolint:errcheck

	origUv := uvLookPathFn
	uvLookPathFn = func() (string, error) {
		return scriptPath, nil
	}
	defer func() { uvLookPathFn = origUv }()

	_, err := ensureHegelInstalled()
	if err == nil {
		t.Fatal("expected error when binary not created")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestEnsureHegelInstalledSuccess(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Create a fake uv that creates the venv and binary.
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "fake_uv")
	script := `#!/bin/sh
if [ "$1" = "venv" ]; then
    mkdir -p "$3/bin"
    touch "$3/bin/python"
    chmod +x "$3/bin/python"
    exit 0
fi
if [ "$1" = "pip" ]; then
    # Find the venv dir from --python arg and create the hegel binary.
    python_path="$4"
    venv_dir=$(dirname $(dirname "$python_path"))
    touch "$venv_dir/bin/hegel"
    chmod +x "$venv_dir/bin/hegel"
    exit 0
fi
exit 1
`
	os.WriteFile(scriptPath, []byte(script), 0o755) //nolint:errcheck

	origUv := uvLookPathFn
	uvLookPathFn = func() (string, error) {
		return scriptPath, nil
	}
	defer func() { uvLookPathFn = origUv }()

	result, err := ensureHegelInstalled()
	if err != nil {
		t.Fatalf("ensureHegelInstalled() error: %v", err)
	}

	hegelDir := filepath.Join(tmp, ".hegel")
	expected := filepath.Join(hegelDir, "venv", "bin", "hegel")
	if result != expected {
		t.Errorf("ensureHegelInstalled() = %q, want %q", result, expected)
	}

	// Verify version file was written.
	versionFile := filepath.Join(hegelDir, "venv", "hegel-version")
	cached, err := os.ReadFile(versionFile)
	if err != nil {
		t.Fatalf("reading version file: %v", err)
	}
	if string(cached) != hegelServerVersion {
		t.Errorf("version file = %q, want %q", string(cached), hegelServerVersion)
	}

	// Second call should use cached version (fast path).
	result2, err := ensureHegelInstalled()
	if err != nil {
		t.Fatalf("second ensureHegelInstalled() error: %v", err)
	}
	if result2 != expected {
		t.Errorf("second ensureHegelInstalled() = %q, want %q", result2, expected)
	}
}

func TestEnsureHegelInstalledLogCreationFails(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Create .hegel as a read-only directory so install.log creation fails.
	hegelDir := filepath.Join(tmp, ".hegel")
	os.MkdirAll(hegelDir, 0o755) //nolint:errcheck
	// Create install.log as a directory so File.Create fails.
	os.MkdirAll(filepath.Join(hegelDir, "install.log"), 0o755) //nolint:errcheck

	origUv := uvLookPathFn
	uvLookPathFn = func() (string, error) {
		return "/usr/bin/true", nil
	}
	defer func() { uvLookPathFn = origUv }()

	_, err := ensureHegelInstalled()
	if err == nil {
		t.Fatal("expected error when log creation fails")
	}
	if !strings.Contains(err.Error(), "install log") {
		t.Errorf("expected log creation error, got: %v", err)
	}
}

func TestEnsureHegelInstalledMkdirFails(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Create a file at .hegel so MkdirAll fails.
	os.WriteFile(filepath.Join(tmp, ".hegel"), []byte("not a dir"), 0o644) //nolint:errcheck

	origUv := uvLookPathFn
	uvLookPathFn = func() (string, error) {
		return "/usr/bin/true", nil
	}
	defer func() { uvLookPathFn = origUv }()

	_, err := ensureHegelInstalled()
	if err == nil {
		t.Fatal("expected error when mkdir fails")
	}
	if !strings.Contains(err.Error(), "failed to create") {
		t.Errorf("expected mkdir error, got: %v", err)
	}
}

func TestFindHegelServerCommandEnv(t *testing.T) {
	resetProjectRoot(t)
	t.Setenv(hegelServerCommandEnv, "/custom/hegel/binary")

	result := findHegel()
	if result != "/custom/hegel/binary" {
		t.Errorf("findHegel() = %q, want /custom/hegel/binary", result)
	}
}

func TestFindHegelAutoInstallPath(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// No .venv, no HEGEL_SERVER_COMMAND — set up a fake cached install.
	hegelDir := filepath.Join(tmp, ".hegel")
	venvDir := filepath.Join(hegelDir, "venv")
	binDir := filepath.Join(venvDir, "bin")
	os.MkdirAll(binDir, 0o755)                                                               //nolint:errcheck
	os.WriteFile(filepath.Join(binDir, "hegel"), []byte("#!/bin/sh\n"), 0o755)               //nolint:errcheck
	os.WriteFile(filepath.Join(venvDir, "hegel-version"), []byte(hegelServerVersion), 0o644) //nolint:errcheck

	result := findHegel()
	expected := filepath.Join(binDir, "hegel")
	if result != expected {
		t.Errorf("findHegel() = %q, want %q", result, expected)
	}
}

func TestEnsureHegelInstalledVersionFileWriteFails(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Create a fake uv that creates the binary but makes hegel-version a directory
	// so the write fails.
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "fake_uv")
	script := `#!/bin/sh
if [ "$1" = "venv" ]; then
    mkdir -p "$3/bin"
    touch "$3/bin/python"
    chmod +x "$3/bin/python"
    # Make hegel-version a directory so write fails.
    mkdir -p "$3/hegel-version"
    exit 0
fi
if [ "$1" = "pip" ]; then
    python_path="$4"
    venv_dir=$(dirname $(dirname "$python_path"))
    touch "$venv_dir/bin/hegel"
    chmod +x "$venv_dir/bin/hegel"
    exit 0
fi
exit 1
`
	os.WriteFile(scriptPath, []byte(script), 0o755) //nolint:errcheck

	origUv := uvLookPathFn
	uvLookPathFn = func() (string, error) {
		return scriptPath, nil
	}
	defer func() { uvLookPathFn = origUv }()

	_, err := ensureHegelInstalled()
	if err == nil {
		t.Fatal("expected error when version file write fails")
	}
	if !strings.Contains(err.Error(), "version file") {
		t.Errorf("expected version file error, got: %v", err)
	}
}
