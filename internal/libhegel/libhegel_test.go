package libhegel

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// TestLoadLibVersion smoke-tests the loader against the real libhegel built
// by `just build-libhegel` (or in the sibling ../hegel-rust/ checkout, or the
// vendored binary embedded at build time). Asserts that hegel_version returns
// the version pinned in version.go.
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
	if !strings.Contains(err.Error(), "load libhegel") {
		t.Errorf("expected error to mention libhegel; got %q", err)
	}
	if !strings.Contains(err.Error(), "/nonexistent/path/to/libhegel.so") {
		t.Errorf("expected error to mention the bogus path; got %q", err)
	}
}

// TestLoadEmbeddedWriteFails covers the fallback path in load: with no path
// override, writing the embedded library must fail when both materialization
// locations are unavailable.
func TestLoadEmbeddedWriteFails(t *testing.T) {
	t.Setenv(LibraryPathEnv, "")
	testHomeOverride(t)
	stubVar(t, &chmodCacheDir, func(string, os.FileMode) error { return errors.New("cache denied") })
	stubVar(t, &resolveTempCacheDir, func(string) (string, error) { return "", errors.New("temp denied") })

	_, err := load()
	if err == nil {
		t.Fatal("expected load to fail when the embedded library cannot be written")
	}
	if !strings.Contains(err.Error(), "write libhegel") {
		t.Errorf("expected error to mention write libhegel; got %q", err)
	}
}
