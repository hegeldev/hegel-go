package libhegel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadLibVersion smoke-tests the loader against the real libhegel built
// by `just build-libhegel` (or in the sibling ../hegel-rust/ checkout, or
// auto-downloaded from the matching GitHub release). Asserts that
// hegel_version returns the version pinned in libhegel.go.
func TestLoadLibVersion(t *testing.T) {
	lib, _, err := load()
	if err != nil {
		t.Fatalf("loadLib: %v", err)
	}
	defer lib.Close()

	got := lib.version()
	if got != hegelVersion {
		t.Fatalf("hegel_version: got %q, want %q (rebuild libhegel.so if it's stale)", got, hegelVersion)
	}
}

// TestRegisterSymbolsMissing covers the dlsym / registerSymbols error path: a
// symbol that the library does not export must surface a wrapped error.
func TestRegisterSymbolsMissing(t *testing.T) {
	lib, _, err := load()
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

// TestLoadLibPathOverride verifies that HEGEL_LIBHEGEL_PATH is honored.
// Skipped when no sibling checkout has been built — the auto-download path
// has its own test against an httptest.Server.
func TestLoadLibPathOverride(t *testing.T) {
	resolved, err := filepath.Abs("../hegel-rust/target/release/" + libhegelFileName())
	if err != nil { // coverage-ignore
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(resolved); err != nil {
		t.Skipf("no libhegel at %s (run `just build-libhegel` for a sibling-checkout build)", resolved)
	}

	t.Setenv(LibraryPathEnv, resolved)

	lib, _, err := load()
	if err != nil {
		t.Fatalf("loadLib with override %q: %v", resolved, err)
	}
	_ = lib.Close()
}

// TestLoadLibMissing verifies the failure path when no library can be found.
func TestLoadLibMissing(t *testing.T) {
	t.Setenv(LibraryPathEnv, "/nonexistent/path/to/libhegel.so")

	_, _, err := load()
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

// TestCandidatePathsOverride verifies the override short-circuits the default search.
func TestCandidatePathsOverride(t *testing.T) {
	t.Setenv(LibraryPathEnv, "/some/explicit/path")
	got := candidatePaths()
	if len(got) != 1 || got[0] != "/some/explicit/path" {
		t.Errorf("override should return [/some/explicit/path], got %v", got)
	}
}

// TestCandidatePathsDefault verifies the default search returns both
// release and debug paths.
func TestCandidatePathsDefault(t *testing.T) {
	t.Setenv(LibraryPathEnv, "")
	got := candidatePaths()
	if len(got) != 2 {
		t.Fatalf("expected 2 default paths, got %d: %v", len(got), got)
	}
	if !strings.Contains(got[0], "release") {
		t.Errorf("expected first path to mention release; got %q", got[0])
	}
	if !strings.Contains(got[1], "debug") {
		t.Errorf("expected second path to mention debug; got %q", got[1])
	}
	expected := libhegelFileName()
	for _, p := range got {
		if filepath.Base(p) != expected {
			t.Errorf("expected file name %q in %q", expected, p)
		}
	}
}
