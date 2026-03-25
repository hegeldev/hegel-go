package hegel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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

	// Verify lock directory was released.
	lockDir := filepath.Join(hegelDir, ".install-lock")
	if _, err := os.Stat(lockDir); err == nil {
		t.Error("lock directory should have been removed after install")
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

	// Create .hegel directory so lock succeeds, but make install.log a directory so Create fails.
	hegelDir := filepath.Join(tmp, ".hegel")
	os.MkdirAll(hegelDir, 0o755)                               //nolint:errcheck
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

// TestEnsureHegelInstalledConcurrentGoroutines verifies that concurrent
// goroutines within the same process are serialized by the in-process mutex.
func TestEnsureHegelInstalledConcurrentGoroutines(t *testing.T) {
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

	expected := filepath.Join(tmp, ".hegel", "venv", "bin", "hegel")

	var wg sync.WaitGroup
	errs := make([]error, 5)
	results := make([]string, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = ensureHegelInstalled()
		}(i)
	}
	wg.Wait()

	for i := 0; i < 5; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: %v", i, errs[i])
		}
		if results[i] != expected {
			t.Errorf("goroutine %d: got %q, want %q", i, results[i], expected)
		}
	}
}

// TestIsInstalled covers the isInstalled helper directly.
func TestIsInstalled(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	versionFile := filepath.Join(tmp, "hegel-version")
	hegelBin := filepath.Join(tmp, "hegel")

	// No version file.
	if isInstalled(versionFile, hegelBin) {
		t.Error("expected false when version file missing")
	}

	// Wrong version.
	os.WriteFile(versionFile, []byte("0.0.0"), 0o644) //nolint:errcheck
	if isInstalled(versionFile, hegelBin) {
		t.Error("expected false with wrong version")
	}

	// Right version, no binary.
	os.WriteFile(versionFile, []byte(hegelServerVersion), 0o644) //nolint:errcheck
	if isInstalled(versionFile, hegelBin) {
		t.Error("expected false when binary missing")
	}

	// Right version + binary exists.
	os.WriteFile(hegelBin, []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck
	if !isInstalled(versionFile, hegelBin) {
		t.Error("expected true when version matches and binary exists")
	}
}

// TestAcquireFileLock verifies basic lock/unlock and timeout.
func TestAcquireFileLock(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	lockDir := filepath.Join(tmp, "test-lock")

	// Acquire and release.
	if err := acquireFileLock(lockDir); err != nil {
		t.Fatalf("acquireFileLock: %v", err)
	}
	if _, err := os.Stat(lockDir); err != nil {
		t.Error("lock directory should exist after acquire")
	}
	releaseFileLock(lockDir)
	if _, err := os.Stat(lockDir); err == nil {
		t.Error("lock directory should not exist after release")
	}
}

// TestReleaseFileLockIdempotent verifies releasing a non-existent lock is safe.
func TestReleaseFileLockIdempotent(t *testing.T) {
	t.Parallel()
	releaseFileLock(filepath.Join(t.TempDir(), "nonexistent"))
}

// TestAcquireFileLockTimeout verifies the lock times out when held by another party.
func TestAcquireFileLockTimeout(t *testing.T) {
	// Not parallel: mutates package-level fileLockTimeoutVal/fileLockPollIntervalVal.
	tmp := t.TempDir()
	lockDir := filepath.Join(tmp, "test-lock")

	// Pre-create the lock directory to simulate another holder.
	os.Mkdir(lockDir, 0o755) //nolint:errcheck

	// Use very short timeout and poll interval.
	origTimeout := fileLockTimeoutVal
	origPoll := fileLockPollIntervalVal
	fileLockTimeoutVal = 10 * time.Millisecond
	fileLockPollIntervalVal = 1 * time.Millisecond
	defer func() {
		fileLockTimeoutVal = origTimeout
		fileLockPollIntervalVal = origPoll
	}()

	err := acquireFileLock(lockDir)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

// TestAcquireFileLockWaitsAndSucceeds verifies the lock retries and succeeds
// when the holder releases.
func TestAcquireFileLockWaitsAndSucceeds(t *testing.T) {
	// Not parallel: mutates package-level fileLockPollIntervalVal.
	tmp := t.TempDir()
	lockDir := filepath.Join(tmp, "test-lock")

	// Pre-create the lock directory.
	os.Mkdir(lockDir, 0o755) //nolint:errcheck

	origPoll := fileLockPollIntervalVal
	fileLockPollIntervalVal = 1 * time.Millisecond
	defer func() { fileLockPollIntervalVal = origPoll }()

	// Release the lock after a short delay.
	go func() {
		time.Sleep(5 * time.Millisecond)
		os.Remove(lockDir) //nolint:errcheck
	}()

	err := acquireFileLock(lockDir)
	if err != nil {
		t.Fatalf("acquireFileLock: %v", err)
	}
	releaseFileLock(lockDir)
}

// TestEnsureHegelInstalledFileLockTimeout verifies ensureHegelInstalled returns
// an error when the file lock cannot be acquired.
func TestEnsureHegelInstalledFileLockTimeout(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Create the .hegel dir and hold the lock.
	hegelDir := filepath.Join(tmp, ".hegel")
	os.MkdirAll(hegelDir, 0o755)                              //nolint:errcheck
	os.Mkdir(filepath.Join(hegelDir, ".install-lock"), 0o755) //nolint:errcheck

	origTimeout := fileLockTimeoutVal
	origPoll := fileLockPollIntervalVal
	fileLockTimeoutVal = 10 * time.Millisecond
	fileLockPollIntervalVal = 1 * time.Millisecond
	defer func() {
		fileLockTimeoutVal = origTimeout
		fileLockPollIntervalVal = origPoll
	}()

	_, err := ensureHegelInstalled()
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

// TestEnsureHegelInstalledRecheckAfterFileLock verifies that if another process
// completes the install while we wait for the file lock, we use the cached result.
func TestEnsureHegelInstalledRecheckAfterFileLock(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	hegelDir := filepath.Join(tmp, ".hegel")
	venvDir := filepath.Join(hegelDir, "venv")
	binDir := filepath.Join(venvDir, "bin")
	versionFile := filepath.Join(venvDir, "hegel-version")
	hegelBin := filepath.Join(binDir, "hegel")

	// Create the .hegel dir and hold the lock.
	os.MkdirAll(hegelDir, 0o755)                              //nolint:errcheck
	os.Mkdir(filepath.Join(hegelDir, ".install-lock"), 0o755) //nolint:errcheck

	origPoll := fileLockPollIntervalVal
	fileLockPollIntervalVal = 1 * time.Millisecond
	defer func() { fileLockPollIntervalVal = origPoll }()

	// Simulate another process completing the install and releasing the lock.
	go func() {
		time.Sleep(5 * time.Millisecond)
		os.MkdirAll(binDir, 0o755)                                   //nolint:errcheck
		os.WriteFile(hegelBin, []byte("#!/bin/sh\n"), 0o755)         //nolint:errcheck
		os.WriteFile(versionFile, []byte(hegelServerVersion), 0o644) //nolint:errcheck
		os.Remove(filepath.Join(hegelDir, ".install-lock"))          //nolint:errcheck
	}()

	result, err := ensureHegelInstalled()
	if err != nil {
		t.Fatalf("ensureHegelInstalled: %v", err)
	}
	if result != hegelBin {
		t.Errorf("got %q, want %q", result, hegelBin)
	}
}
