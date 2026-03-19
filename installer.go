package hegel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// hegelServerVersion is the version of hegel-core that this SDK requires.
const hegelServerVersion = "0.2.1"

// hegelServerCommandEnv is the environment variable that overrides automatic installation.
const hegelServerCommandEnv = "HEGEL_SERVER_COMMAND"

// installMu serializes installation attempts within a single process.
var installMu sync.Mutex

// fileLockTimeoutVal is how long to wait for the cross-process file lock.
// Overridable in tests.
var fileLockTimeoutVal = 5 * time.Minute

// fileLockPollIntervalVal is how often to retry acquiring the file lock.
// Overridable in tests.
var fileLockPollIntervalVal = 500 * time.Millisecond

// isInstalled checks whether the correct version is cached and the binary exists.
func isInstalled(versionFile, hegelBin string) bool {
	cached, err := os.ReadFile(versionFile)
	if err != nil {
		return false
	}
	if strings.TrimSpace(string(cached)) != hegelServerVersion {
		return false
	}
	_, err = os.Stat(hegelBin)
	return err == nil
}

// ensureHegelInstalled checks if the correct version of hegel is installed
// in the project's .hegel/venv directory, and installs it if not.
// It is safe to call concurrently from multiple goroutines and multiple processes.
// Returns the path to the hegel binary.
func ensureHegelInstalled() (string, error) {
	hegelDir := getHegelDirectory()
	venvDir := filepath.Join(hegelDir, "venv")
	versionFile := filepath.Join(venvDir, "hegel-version")
	hegelBin := filepath.Join(venvDir, "bin", "hegel")

	// Fast path (no locks): check cached version.
	if isInstalled(versionFile, hegelBin) {
		return hegelBin, nil
	}

	// Acquire in-process lock to serialize goroutines.
	installMu.Lock()
	defer installMu.Unlock()

	// Re-check after acquiring in-process lock.
	if isInstalled(versionFile, hegelBin) {
		return hegelBin, nil
	}

	// Create .hegel directory (needed for the file lock).
	if err := os.MkdirAll(hegelDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", hegelDir, err)
	}

	// Acquire cross-process file lock.
	lockDir := filepath.Join(hegelDir, ".install-lock")
	if err := acquireFileLock(lockDir); err != nil {
		return "", err
	}
	defer releaseFileLock(lockDir)

	// Re-check after acquiring file lock (another process may have installed).
	if isInstalled(versionFile, hegelBin) {
		return hegelBin, nil
	}

	return doInstall(hegelDir, venvDir, versionFile, hegelBin)
}

// doInstall performs the actual installation. Caller must hold both locks.
func doInstall(hegelDir, venvDir, versionFile, hegelBin string) (string, error) {
	installLog := filepath.Join(hegelDir, "install.log")

	logFile, err := os.Create(installLog)
	if err != nil {
		return "", fmt.Errorf("failed to create install log: %w", err)
	}
	defer logFile.Close()

	fmt.Fprintf(os.Stderr, "hegel: installing hegel-core %s into %s...\n", hegelServerVersion, venvDir)

	uvPath, err := uvLookPathFn()
	if err != nil {
		return "", fmt.Errorf("hegel: uv not found on PATH. Install uv (https://docs.astral.sh/uv/) to auto-install the hegel server, or set %s to a hegel binary path", hegelServerCommandEnv)
	}

	// Create venv.
	cmd := exec.Command(uvPath, "venv", "--clear", venvDir)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Run(); err != nil {
		log, _ := os.ReadFile(installLog)
		return "", fmt.Errorf("uv venv failed: %w\nInstall log:\n%s", err, string(log))
	}

	// Install hegel-core.
	pythonPath := filepath.Join(venvDir, "bin", "python")
	cmd = exec.Command(uvPath, "pip", "install", "--python", pythonPath,
		fmt.Sprintf("hegel-core==%s", hegelServerVersion))
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Run(); err != nil {
		log, _ := os.ReadFile(installLog)
		return "", fmt.Errorf("failed to install hegel-core %s. Set %s to a hegel binary path to skip installation.\nInstall log:\n%s",
			hegelServerVersion, hegelServerCommandEnv, string(log))
	}

	// Verify binary exists.
	if _, err := os.Stat(hegelBin); err != nil {
		return "", fmt.Errorf("hegel not found at %s after installation", hegelBin)
	}

	// Write version file (signals completion to other waiters).
	if err := os.WriteFile(versionFile, []byte(hegelServerVersion), 0o644); err != nil {
		return "", fmt.Errorf("failed to write version file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "hegel: installation complete.\n")
	return hegelBin, nil
}

// acquireFileLock uses mkdir as an atomic lock primitive (works on macOS and Linux).
func acquireFileLock(lockDir string) error {
	deadline := time.Now().Add(fileLockTimeoutVal)
	for {
		if err := os.Mkdir(lockDir, 0o755); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("hegel: timed out waiting for install lock %s (another process may be installing; remove manually if stale)", lockDir)
		}
		time.Sleep(fileLockPollIntervalVal)
	}
}

// releaseFileLock removes the lock directory.
func releaseFileLock(lockDir string) {
	os.Remove(lockDir) //nolint:errcheck
}

// uvLookPathFn is the function used to find uv. Overridable in tests.
var uvLookPathFn = func() (string, error) {
	return exec.LookPath("uv")
}
