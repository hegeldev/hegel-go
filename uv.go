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
	pathUV := findInPath("uv")
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
	if err := installUVFn(cacheDir); err != nil {
		return "", err
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

// findInPath searches PATH for a named binary and returns its full path,
// or "" if not found.
func findInPath(name string) string {
	pathVar := os.Getenv("PATH")
	if pathVar == "" {
		return ""
	}
	for _, dir := range filepath.SplitList(pathVar) {
		p := filepath.Join(dir, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// cacheDirFrom returns the hegel cache directory based on XDG_CACHE_HOME or HOME.
func cacheDirFrom(xdgCacheHome, homeDir string) string {
	if xdgCacheHome != "" {
		return filepath.Join(xdgCacheHome, "hegel")
	}
	return filepath.Join(homeDir, ".cache", "hegel")
}
