package hegel

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed uv-install.sh
var uvInstaller string

// findUV returns the path to a uv binary.
//
// Lookup order:
//  1. uv found on PATH
//  2. Cached binary at ~/.cache/hegel/uv
//  3. Installs uv to ~/.cache/hegel/uv using the embedded installer script
func findUV() (string, error) {
	pathUV, _ := exec.LookPath("uv")
	cacheDir := cacheDirFrom(os.Getenv("XDG_CACHE_HOME"), os.Getenv("HOME"))
	return findUVImpl(pathUV, cacheDir)
}

func findUVImpl(pathUV, cacheDir string) (string, error) {
	if pathUV != "" {
		return pathUV, nil
	}
	cached := filepath.Join(cacheDir, "uv")
	if info, err := os.Stat(cached); err == nil && !info.IsDir() {
		return cached, nil
	}
	// Install to a temp dir inside cacheDir, then atomically rename.
	// This is safe for concurrent processes on the same filesystem.
	tmpDir, err := os.MkdirTemp(filepath.Dir(cacheDir), "hegel-uv-install-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir for uv install: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	if err := installUVFn(tmpDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil { // coverage-ignore
		return "", fmt.Errorf("failed to create cache directory %s: %w", cacheDir, err)
	}
	// Atomic rename; if another process already placed it, that's fine.
	if err := os.Rename(filepath.Join(tmpDir, "uv"), cached); err != nil && !os.IsExist(err) {
		// Another process may have beaten us; check if it's there now.
		if info, statErr := os.Stat(cached); statErr == nil && !info.IsDir() { // coverage-ignore
			return cached, nil
		}
		return "", fmt.Errorf("failed to install uv to %s: %w", cached, err)
	}
	return cached, nil
}

// installUVFn is the function used to install uv. Overridable in tests.
var installUVFn = func(cacheDir string) error {
	return installUVWithSh(cacheDir, "sh")
}

func installUVWithSh(cacheDir, sh string) error {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory %s: %w", cacheDir, err)
	}
	cmd := exec.Command(sh)
	cmd.Stdin = strings.NewReader(uvInstaller)
	cmd.Env = append(os.Environ(), "UV_UNMANAGED_INSTALL="+cacheDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("uv installer failed: %w\nOutput: %s\nInstall uv manually: https://docs.astral.sh/uv/getting-started/installation/", err, string(output))
	}
	return nil
}

// cacheDirFrom returns the hegel cache directory based on XDG_CACHE_HOME or HOME.
func cacheDirFrom(xdgCacheHome, homeDir string) string {
	if xdgCacheHome != "" {
		return filepath.Join(xdgCacheHome, "hegel")
	}
	return filepath.Join(homeDir, ".cache", "hegel")
}
