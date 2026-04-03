package hegel

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHegelCommandServerCommandEnv(t *testing.T) {
	resetProjectRoot(t)
	t.Setenv(hegelServerCommandEnv, "/custom/hegel/binary")

	cmd, err := hegelCommand()
	if err != nil {
		t.Fatalf("hegelCommand: %v", err)
	}
	if cmd.Path != "/custom/hegel/binary" && cmd.Args[0] != "/custom/hegel/binary" {
		t.Errorf("expected /custom/hegel/binary, got %v", cmd.Args)
	}
}

func TestHegelCommandUVToolRun(t *testing.T) {
	resetProjectRoot(t)
	t.Setenv(hegelServerCommandEnv, "")

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// uv is on PATH → should use uv tool run.
	uvPath, err := exec.LookPath("uv")
	if err != nil {
		t.Skip("uv not found on PATH")
	}

	cmd, err := hegelCommand()
	if err != nil {
		t.Fatalf("hegelCommand: %v", err)
	}
	if cmd.Args[0] != uvPath {
		t.Errorf("expected uv path %q, got %v", uvPath, cmd.Args)
	}
	// Should contain "tool", "run", "--from", "hegel-core==..."
	found := false
	for _, arg := range cmd.Args {
		if arg == "tool" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'tool' in args, got %v", cmd.Args)
	}
}

func TestHegelCommandNoUV(t *testing.T) {
	resetProjectRoot(t)
	t.Setenv(hegelServerCommandEnv, "")

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// No uv on PATH.
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("HOME", "/nonexistent")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	_, err := hegelCommand()
	if err == nil {
		t.Fatal("expected error when no uv available")
	}
}
