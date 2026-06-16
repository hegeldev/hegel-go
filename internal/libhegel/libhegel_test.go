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

// TestLoadLibPathOverride verifies that HEGEL_LIBHEGEL_PATH is honored. It
// first resolves a real library through the normal search (sibling checkout,
// an already-set override, or the auto-downloader), then pins that path via
// the override and confirms it is loaded from there. Skipped only when no
// library can be resolved at all (e.g. offline with an empty cache).
func TestLoadLibPathOverride(t *testing.T) {
	_, resolved, err := load()
	if err != nil {
		t.Skipf("no loadable libhegel available: %v", err)
	}

	t.Setenv(LibraryPathEnv, resolved)

	lib, gotPath, err := load()
	if err != nil {
		t.Fatalf("loadLib with override %q: %v", resolved, err)
	}
	defer lib.Close()
	if gotPath != resolved {
		t.Errorf("override path: got %q, want %q", gotPath, resolved)
	}
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

// TestCandidatePathsNoOverride verifies that with no HEGEL_LIBHEGEL_PATH set,
// candidatePaths returns nil so resolution falls through to the auto-downloader
// — the library does not search for a sibling hegel-rust checkout.
func TestCandidatePathsNoOverride(t *testing.T) {
	t.Setenv(LibraryPathEnv, "")
	if got := candidatePaths(); got != nil {
		t.Errorf("expected nil candidate paths with no override, got %v", got)
	}
}
