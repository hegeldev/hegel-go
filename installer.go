package hegel

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// hegelServerVersion is the version of hegel-core that this SDK requires.
const hegelServerVersion = "0.3.0"

// hegelServerCommandEnv is the environment variable that overrides automatic installation.
const hegelServerCommandEnv = "HEGEL_SERVER_COMMAND"

// hegelCommand returns an exec.Cmd that starts the hegel server with --stdio.
//
// Priority:
//  1. HEGEL_SERVER_COMMAND env var → direct binary
//  2. .venv/bin/hegel in project root → direct binary (for `just setup` users)
//  3. uv tool run → finds or auto-downloads uv, runs hegel-core via uv
func hegelCommand() (*exec.Cmd, error) {
	// 1. Environment variable override.
	if override := os.Getenv(hegelServerCommandEnv); override != "" {
		return exec.Command(override, "--stdio", "--verbosity", "normal"), nil
	}

	// 2. Check .venv in project root (e.g. from `just setup`).
	root := getProjectRoot()
	if p := findHegelInDir(filepath.Join(root, ".venv")); p != "" {
		return exec.Command(p, "--stdio", "--verbosity", "normal"), nil
	}

	// 3. Use uv tool run.
	uvPath, err := findUV()
	if err != nil {
		return nil, fmt.Errorf("could not find or install uv: %w\nSet %s to a hegel binary path to skip automatic installation", err, hegelServerCommandEnv)
	}
	cmd := exec.Command(uvPath, "tool", "run",
		"--from", fmt.Sprintf("hegel-core==%s", hegelServerVersion),
		"hegel", "--stdio", "--verbosity", "normal")
	return cmd, nil
}

// findHegelInDir looks for bin/hegel inside dir.
func findHegelInDir(dir string) string {
	p := filepath.Join(dir, "bin", "hegel")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}
