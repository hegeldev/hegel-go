package hegel

import (
	"fmt"
	"os"
	"os/exec"
)

// hegelServerVersion is the version of hegel-core that this SDK requires.
const hegelServerVersion = "0.4.11"

// hegelServerCommandEnv is the environment variable that overrides automatic installation.
const hegelServerCommandEnv = "HEGEL_SERVER_COMMAND"

// hegelCommand returns an exec.Cmd that starts the hegel server. The server
// communicates with the client over stdin/stdout.
//
// Priority:
//  1. HEGEL_SERVER_COMMAND env var → direct binary
//  2. uv tool run → finds or auto-downloads uv, runs hegel-core via uv
func hegelCommand() (*exec.Cmd, error) {
	// 1. Environment variable override.
	if override := os.Getenv(hegelServerCommandEnv); override != "" {
		return exec.Command(override, "--verbosity", "normal"), nil
	}

	// 2. Use uv tool run.
	uvPath, err := findUV()
	if err != nil {
		return nil, fmt.Errorf("could not find or install uv: %w\nSet %s to a hegel binary path to skip automatic installation", err, hegelServerCommandEnv)
	}
	cmd := exec.Command(uvPath, "tool", "run",
		"--from", fmt.Sprintf("hegel-core==%s", hegelServerVersion),
		"hegel", "--verbosity", "normal")
	return cmd, nil
}
