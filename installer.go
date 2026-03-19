package hegel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// hegelServerVersion is the version of hegel-core that this SDK requires.
const hegelServerVersion = "0.2.1"

// hegelServerCommandEnv is the environment variable that overrides automatic installation.
const hegelServerCommandEnv = "HEGEL_SERVER_COMMAND"

// ensureHegelInstalled checks if the correct version of hegel is installed
// in the project's .hegel/venv directory, and installs it if not.
// Returns the path to the hegel binary.
func ensureHegelInstalled() (string, error) {
	hegelDir := getHegelDirectory()
	venvDir := filepath.Join(hegelDir, "venv")
	versionFile := filepath.Join(venvDir, "hegel-version")
	hegelBin := filepath.Join(venvDir, "bin", "hegel")
	installLog := filepath.Join(hegelDir, "install.log")

	// Fast path: check cached version.
	if cached, err := os.ReadFile(versionFile); err == nil {
		if strings.TrimSpace(string(cached)) == hegelServerVersion {
			if _, err := os.Stat(hegelBin); err == nil {
				return hegelBin, nil
			}
		}
	}

	// Need to install. Create the .hegel directory.
	if err := os.MkdirAll(hegelDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", hegelDir, err)
	}

	logFile, err := os.Create(installLog)
	if err != nil {
		return "", fmt.Errorf("failed to create install log: %w", err)
	}
	defer logFile.Close()

	fmt.Fprintf(os.Stderr, "hegel: installing hegel-core %s into %s...\n", hegelServerVersion, venvDir)

	// Create venv.
	uvPath, err := uvLookPathFn()
	if err != nil {
		return "", fmt.Errorf("hegel: uv not found on PATH. Install uv (https://docs.astral.sh/uv/) to auto-install the hegel server, or set %s to a hegel binary path", hegelServerCommandEnv)
	}

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

	// Write version file.
	if err := os.WriteFile(versionFile, []byte(hegelServerVersion), 0o644); err != nil {
		return "", fmt.Errorf("failed to write version file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "hegel: installation complete.\n")
	return hegelBin, nil
}

// uvLookPathFn is the function used to find uv. Overridable in tests.
var uvLookPathFn = func() (string, error) {
	return exec.LookPath("uv")
}
