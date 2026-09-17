package libhegel

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testHomeOverride redirects os.UserCacheDir into a fresh tempdir for the
// test, isolating the cache so the materialized library doesn't pollute the
// developer's actual ~/.cache.
func testHomeOverride(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir) // macOS fallback when XDG_CACHE_HOME is unset
	t.Setenv("LocalAppData", dir)
	return dir
}

// stubVar replaces the package-level stub *p with v for the duration of the
// test, restoring the original on cleanup.
func stubVar[T any](t *testing.T, p *T, v T) {
	t.Helper()
	orig := *p
	*p = v
	t.Cleanup(func() { *p = orig })
}

// TestEmbeddedLibPresent asserts that a vendored libhegel binary is embedded
// for the platform the test suite runs on. CI runs this on linux/amd64,
// darwin/arm64, and windows/amd64 — the published, vendored assets.
func TestEmbeddedLibPresent(t *testing.T) {
	if len(embeddedLib) == 0 {
		t.Fatalf("no embedded libhegel for this platform; libs/%s missing or not git-lfs-pulled", libhegelAssetName())
	}
}

// TestMaterializeEmbeddedSuccess covers the happy path: the bytes are written
// to the per-version cache dir, marked executable, and returned by path.
func TestMaterializeEmbeddedSuccess(t *testing.T) {
	testHomeOverride(t)
	payload := []byte("fake libhegel binary")

	path, err := writeDynamicLibrary(payload)
	if err != nil {
		t.Fatalf("materializeEmbedded: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil { // coverage-ignore
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("body mismatch: got %q", got)
	}
	fi, err := os.Stat(path)
	if err != nil { // coverage-ignore
		t.Fatalf("stat: %v", err)
	}
	// Windows exposes file attributes rather than Unix executable bits, so
	// os.Chmod(path, 0755) is not reflected as an owner-execute permission.
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("expected executable bit set, got mode %v", fi.Mode())
	}
	if filepath.Base(path) != libhegelAssetName() {
		t.Errorf("expected file named %q, got %q", libhegelAssetName(), filepath.Base(path))
	}
}

// TestMaterializeEmbeddedCacheHit verifies an existing destination is
// authoritative even when the supplied bytes differ.
func TestMaterializeEmbeddedCacheHit(t *testing.T) {
	testHomeOverride(t)
	payload := []byte("cached payload")

	path1, err := writeDynamicLibrary(payload)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}

	path2, err := writeDynamicLibrary([]byte("bogus! payload"))
	if err != nil {
		t.Fatalf("second (cached) call: %v", err)
	}
	if path1 != path2 {
		t.Errorf("expected same path on cache hit; got %q vs %q", path1, path2)
	}
	got, err := os.ReadFile(path2)
	if err != nil { // coverage-ignore
		t.Fatalf("read cached file: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("cache hit rewrote destination: got %q, want %q", got, payload)
	}
}

// TestMaterializeEmbeddedRepairsCacheDirMode verifies a cache hit still
// restores the version directory to its private mode.
func TestMaterializeEmbeddedRepairsCacheDirMode(t *testing.T) {
	if runtime.GOOS == "windows" { // coverage-ignore (Windows modes map to file attributes, not Unix permission bits)
		t.Skip("Windows cache privacy is inherited from the LocalAppData ACL")
	}
	testHomeOverride(t)

	path, err := writeDynamicLibrary([]byte("cached payload"))
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o755); err != nil { // coverage-ignore
		t.Fatalf("make cache dir permissive: %v", err)
	}

	if _, err := writeDynamicLibrary([]byte("cached payload")); err != nil {
		t.Fatalf("cache hit: %v", err)
	}
	fi, err := os.Stat(dir)
	if err != nil { // coverage-ignore
		t.Fatalf("stat cache dir: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0o700 {
		t.Errorf("cache dir mode: got %#o, want 0700", got)
	}
}

// TestMaterializeEmbeddedFallsBackToTemp verifies an unusable cache degrades
// to a fresh extraction under the system temp dir rather than an error: with
// no cached library, the library must still materialize.
func TestMaterializeEmbeddedFallsBackToTemp(t *testing.T) {
	testHomeOverride(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp) // unix temp root
	t.Setenv("TMP", tmp)    // windows temp root
	stubVar(t, &chmodCacheDir, func(string, os.FileMode) error { return errors.New("chmod denied") })

	payload := []byte("fallback payload")
	path, err := writeDynamicLibrary(payload)
	if err != nil {
		t.Fatalf("writeDynamicLibrary: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil { // coverage-ignore
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("body mismatch: got %q", got)
	}
	if filepath.Base(path) != libhegelAssetName() {
		t.Errorf("expected file named %q, got %q", libhegelAssetName(), filepath.Base(path))
	}
}

// TestMaterializeEmbeddedTempFallbackError covers the terminal case: both the
// cache and the temp-dir fallback are unusable. The error must surface both
// causes.
func TestMaterializeEmbeddedTempFallbackError(t *testing.T) {
	testHomeOverride(t)
	stubVar(t, &chmodCacheDir, func(string, os.FileMode) error { return errors.New("chmod denied") })
	want := errors.New("temp dir denied")
	stubVar(t, &mkdirTemp, func(string, string) (string, error) { return "", want })

	_, err := writeDynamicLibrary([]byte("libhegel"))
	if !errors.Is(err, want) {
		t.Fatalf("writeDynamicLibrary: got %v, want temp dir error", err)
	}
	if !strings.Contains(err.Error(), "cache also unusable") {
		t.Errorf("expected the cache failure to be surfaced too; got %q", err)
	}
	if !strings.Contains(err.Error(), "secure cache dir") {
		t.Errorf("expected the cache failure detail; got %q", err)
	}
}

// TestCachedLibraryLockError covers failure to acquire the cross-process
// publication lock.
func TestCachedLibraryLockError(t *testing.T) {
	testHomeOverride(t)
	want := errors.New("lock failed")
	stubVar(t, &lockCacheDir, func(string) (func() error, error) { return nil, want })

	_, err := cachedLibrary([]byte("libhegel"))
	if !errors.Is(err, want) {
		t.Fatalf("cachedLibrary: got %v, want lock error", err)
	}
	if !strings.Contains(err.Error(), "lock cache dir") {
		t.Errorf("expected cache lock context; got %q", err)
	}
}

// TestCachedLibraryUnlockError covers failure to release the cross-process
// publication lock after a successful write.
func TestCachedLibraryUnlockError(t *testing.T) {
	testHomeOverride(t)
	want := errors.New("unlock failed")
	stubVar(t, &lockCacheDir, func(string) (func() error, error) {
		return func() error { return want }, nil
	})

	_, err := cachedLibrary([]byte("libhegel"))
	if !errors.Is(err, want) {
		t.Fatalf("cachedLibrary: got %v, want unlock error", err)
	}
	if !strings.Contains(err.Error(), "unlock cache dir") {
		t.Errorf("expected cache unlock context; got %q", err)
	}
}

// TestMaterializeEmbeddedEmpty covers the unsupported-platform path: empty
// bytes are rejected before any disk access.
func TestMaterializeEmbeddedEmpty(t *testing.T) {
	_, err := writeDynamicLibrary(nil)
	if err == nil {
		t.Fatal("expected error for empty embedded lib")
	}
	// The message must point users at the env-var escape hatch so they can
	// supply a self-built libhegel on an arch with no vendored binary.
	if !strings.Contains(err.Error(), LibraryPathEnv) {
		t.Errorf("expected error to mention %s; got %q", LibraryPathEnv, err)
	}
}

// TestCachedLibraryUserCacheDirError verifies that failure to resolve a
// per-user cache is reported to the caller (which then falls back to the
// system temp dir).
func TestCachedLibraryUserCacheDirError(t *testing.T) {
	switch runtime.GOOS {
	case "darwin": // coverage-ignore (unreachable on the linux CI runner)
		t.Setenv("HOME", "")
	case "windows": // coverage-ignore (unreachable on the linux CI runner)
		t.Setenv("LocalAppData", "")
	default:
		t.Setenv("XDG_CACHE_HOME", "relative")
	}

	_, err := cachedLibrary([]byte("libhegel"))
	if err == nil {
		t.Fatal("expected user cache directory resolution to fail")
	}
	if !strings.Contains(err.Error(), "user cache dir") {
		t.Errorf("expected user cache directory error; got %q", err)
	}
}
