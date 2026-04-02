package hegel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheDirFromXDG(t *testing.T) {
	t.Parallel()
	result := cacheDirFrom("/tmp/xdg", "")
	expected := filepath.Join("/tmp/xdg", "hegel")
	if result != expected {
		t.Errorf("cacheDirFrom with XDG = %q, want %q", result, expected)
	}
}

func TestCacheDirFromHome(t *testing.T) {
	t.Parallel()
	result := cacheDirFrom("", "/home/test")
	expected := filepath.Join("/home/test", ".cache", "hegel")
	if result != expected {
		t.Errorf("cacheDirFrom with home = %q, want %q", result, expected)
	}
}

func TestCacheDirFromXDGTakesPrecedence(t *testing.T) {
	t.Parallel()
	result := cacheDirFrom("/tmp/xdg", "/home/test")
	expected := filepath.Join("/tmp/xdg", "hegel")
	if result != expected {
		t.Errorf("cacheDirFrom should prefer XDG, got %q, want %q", result, expected)
	}
}

func TestFindUVImplUsesPathUV(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	fakeUV := filepath.Join(tmp, "uv")
	os.WriteFile(fakeUV, []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck

	result, err := findUVImpl(fakeUV, "/nonexistent")
	if err != nil {
		t.Fatalf("findUVImpl: %v", err)
	}
	if result != fakeUV {
		t.Errorf("findUVImpl with path uv = %q, want %q", result, fakeUV)
	}
}

func TestFindUVImplReturnsCachedWhenNotInPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cachedUV := filepath.Join(tmp, "uv")
	os.WriteFile(cachedUV, []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck

	result, err := findUVImpl("", tmp)
	if err != nil {
		t.Fatalf("findUVImpl: %v", err)
	}
	if result != cachedUV {
		t.Errorf("findUVImpl with cached = %q, want %q", result, cachedUV)
	}
}

func TestFindInPathFindsKnownBinary(t *testing.T) {
	t.Parallel()
	result := findInPath("sh")
	if result == "" {
		t.Error("findInPath('sh') should find sh")
	}
}

func TestFindInPathReturnsEmptyForMissing(t *testing.T) {
	t.Parallel()
	result := findInPath("definitely_not_a_real_binary_xyz")
	if result != "" {
		t.Errorf("findInPath should return empty for missing binary, got %q", result)
	}
}

func TestInstallUVFailsWithBadShell(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	err := installUVWithSh(tmp, "definitely_not_a_real_shell_xyz")
	if err == nil {
		t.Fatal("expected error from installUVWithSh with bad shell")
	}
}
