package hegel

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

const (
	uvInstallURL   = "https://astral.sh/uv/install.sh"
	hegelPipSource = "git+ssh://git@github.com/antithesishq/hegel.git"
)

var (
	ensureOnce sync.Once
	ensureErr  error
	cachedPath string
)

// ensureHegel ensures the hegel binary is available, installing it if necessary.
// Returns the path to the hegel binary.
func ensureHegel() (string, error) {
	ensureOnce.Do(func() {
		cachedPath, ensureErr = doEnsureHegel()
	})
	return cachedPath, ensureErr
}

func doEnsureHegel() (string, error) {
	// First check if hegel is already on PATH
	if path, err := exec.LookPath("hegel"); err == nil {
		return path, nil
	}

	// Check if we already have it installed in .hegel
	hegelDir := ".hegel"
	hegelBin := filepath.Join(hegelDir, "venv", "bin", "hegel")
	if _, err := os.Stat(hegelBin); err == nil {
		return hegelBin, nil
	}

	// Need to install - create .hegel directory
	if err := os.MkdirAll(hegelDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create .hegel directory: %w", err)
	}

	// Find or install uv
	uvPath, err := ensureUV(hegelDir)
	if err != nil {
		return "", fmt.Errorf("failed to ensure uv: %w", err)
	}

	// Create venv and install hegel
	venvDir := filepath.Join(hegelDir, "venv")
	if err := createVenvAndInstallHegel(uvPath, venvDir); err != nil {
		return "", fmt.Errorf("failed to install hegel: %w", err)
	}

	return hegelBin, nil
}

func ensureUV(hegelDir string) (string, error) {
	// Check if uv is on PATH
	if path, err := exec.LookPath("uv"); err == nil {
		return path, nil
	}

	// Check if we already installed it
	uvBin := filepath.Join(hegelDir, "uv", "uv")
	if _, err := os.Stat(uvBin); err == nil {
		return uvBin, nil
	}

	// Download and run the install script
	resp, err := http.Get(uvInstallURL)
	if err != nil {
		return "", fmt.Errorf("failed to download uv install script: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to download uv install script: status %d", resp.StatusCode)
	}

	script, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read uv install script: %w", err)
	}

	uvInstallDir := filepath.Join(hegelDir, "uv")
	cmd := exec.Command("sh")
	cmd.Stdin = bytes.NewReader(script)
	cmd.Env = append(os.Environ(), "UV_INSTALL_DIR="+uvInstallDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to run uv install script: %w", err)
	}

	return uvBin, nil
}

func createVenvAndInstallHegel(uvPath, venvDir string) error {
	// Create venv with Python 3.13
	cmd := exec.Command(uvPath, "venv", "--python", "3.13", venvDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create venv: %w", err)
	}

	// Install hegel into the venv
	cmd = exec.Command(uvPath, "pip", "install", "--python", filepath.Join(venvDir, "bin", "python"), hegelPipSource)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install hegel: %w", err)
	}

	return nil
}
