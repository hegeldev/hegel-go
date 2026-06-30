package libhegel

import (
	"strings"
	"testing"
)

// TestLoadLibVersion smoke-tests the loader against the real libhegel built
// by `just build-libhegel` (or in the sibling ../hegel-rust/ checkout, or
// auto-downloaded from the matching GitHub release). Asserts that
// hegel_version returns the version pinned in libhegel.go.
func TestLoadLibVersion(t *testing.T) {
	lib, err := load()
	if err != nil {
		t.Fatalf("loadLib: %v", err)
	}
	defer lib.Close()

	got := lib.versionString()
	if got != hegelVersion {
		t.Fatalf("hegel_version: got %q, want %q (rebuild libhegel.so if it's stale)", got, hegelVersion)
	}
}

// TestRegisterSymbolsMissing covers the dlsym / registerSymbols error path: a
// symbol that the library does not export must surface a wrapped error.
func TestRegisterSymbolsMissing(t *testing.T) {
	lib, err := load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer lib.Close()

	var fn func()
	err = registerSymbols(lib.handle, []symbol{
		{name: "hegel_definitely_missing_symbol", dst: &fn},
	})
	if err == nil {
		t.Fatal("expected error resolving a missing symbol")
	}
}

// TestLoadLibMissing verifies the failure path when no library can be found.
func TestLoadLibMissing(t *testing.T) {
	t.Setenv(LibraryPathEnv, "/nonexistent/path/to/libhegel.so")

	_, err := load()
	if err == nil {
		t.Fatal("expected loadLib to fail with a bogus path")
	}
	if !strings.Contains(err.Error(), "could not load libhegel") {
		t.Errorf("expected error to mention libhegel; got %q", err)
	}
	if !strings.Contains(err.Error(), LibraryPathEnv) {
		t.Errorf("expected error to mention env var %s; got %q", LibraryPathEnv, err)
	}
	if !strings.Contains(err.Error(), "/nonexistent/path/to/libhegel.so") {
		t.Errorf("expected error to mention the bogus path; got %q", err)
	}
}
