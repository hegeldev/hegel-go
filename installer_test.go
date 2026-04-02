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

func TestHegelCommandVenvPath(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// Create .venv/bin/hegel in the project root.
	venvBin := filepath.Join(tmp, ".venv", "bin")
	os.MkdirAll(venvBin, 0o755)                                                 //nolint:errcheck
	os.WriteFile(filepath.Join(venvBin, "hegel"), []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck

	cmd, err := hegelCommand()
	if err != nil {
		t.Fatalf("hegelCommand: %v", err)
	}
	expected := filepath.Join(venvBin, "hegel")
	if cmd.Args[0] != expected {
		t.Errorf("hegelCommand() = %v, want first arg %q", cmd.Args, expected)
	}
}

func TestHegelCommandUVToolRun(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// No .venv, uv is on PATH → should use uv tool run.
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

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck
	t.Chdir(tmp)

	// No .venv, no uv on PATH.
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("HOME", "/nonexistent")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmp, "cache"))

	_, err := hegelCommand()
	if err == nil {
		t.Fatal("expected error when no uv available")
	}
}

func TestFindHegelInDir(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "bin")
	os.MkdirAll(binDir, 0o755)                                                 //nolint:errcheck
	os.WriteFile(filepath.Join(binDir, "hegel"), []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck

	result := findHegelInDir(tmp)
	expected := filepath.Join(binDir, "hegel")
	if result != expected {
		t.Errorf("findHegelInDir(%q) = %q, want %q", tmp, result, expected)
	}
}

func TestFindHegelInDirMissing(t *testing.T) {
	t.Parallel()
	result := findHegelInDir("/nonexistent/path")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}
