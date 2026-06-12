package libhegel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testHomeOverride redirects os.UserCacheDir into a fresh tempdir for the
// test, isolating the cache so downloads don't pollute the developer's
// actual ~/.cache.
func testHomeOverride(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("HOME", dir) // macOS fallback when XDG_CACHE_HOME is unset
	return dir
}

// startReleaseServer launches an httptest.Server that serves a fake libhegel
// asset at /v<version>/<asset>. asset is the body returned for the library
// file. Checksums are now baked into the library, so no .sha256 is served;
// callers pass the expected digest directly to downloadLibhegel.
func startReleaseServer(t *testing.T, version string, asset []byte) *httptest.Server {
	t.Helper()
	name := libhegelAssetName()
	libPath := fmt.Sprintf("/v%s/%s", version, name)
	mux := http.NewServeMux()
	mux.HandleFunc(libPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(asset)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// --- libhegelAssetName / Ext / URL ---

func TestLibhegelDownloadURLOverride(t *testing.T) {
	t.Setenv(hegelDownloadBaseURLEnv, "https://example.test/staging")
	got := libhegelDownloadURL("9.9.9")
	want := "https://example.test/staging/v9.9.9/" + libhegelAssetName()
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLibhegelDownloadURLDefault(t *testing.T) {
	t.Setenv(hegelDownloadBaseURLEnv, "")
	got := libhegelDownloadURL("1.2.3")
	if !strings.Contains(got, "1.2.3") {
		t.Errorf("epected URL to contain version, got %q", got)
	}
}

// --- downloadLibhegel: happy path ---

func TestDownloadLibhegelSuccess(t *testing.T) {
	testHomeOverride(t)
	asset := []byte("fake libhegel binary")
	sum := sha256Hex(asset)
	srv := startReleaseServer(t, "0.0.test", asset)
	t.Setenv(hegelDownloadBaseURLEnv, srv.URL)

	path, err := downloadLibhegel("0.0.test", sum)
	if err != nil {
		t.Fatalf("downloadLibhegel: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected destination to exist: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil { // coverage-ignore
		t.Fatalf("read: %v", err)
	}
	if string(got) != string(asset) {
		t.Errorf("body mismatch")
	}
}

// --- downloadLibhegel: cache hit ---

func TestDownloadLibhegelCacheHit(t *testing.T) {
	testHomeOverride(t)
	asset := []byte("first")
	sum := sha256Hex(asset)
	srv := startReleaseServer(t, "1.0.test", asset)
	t.Setenv(hegelDownloadBaseURLEnv, srv.URL)

	// First call downloads.
	path1, err := downloadLibhegel("1.0.test", sum)
	if err != nil {
		t.Fatalf("first download: %v", err)
	}

	// Close the server: a second download attempt would fail. The cache hit
	// must not touch the network.
	srv.Close()

	path2, err := downloadLibhegel("1.0.test", sum)
	if err != nil {
		t.Fatalf("second (cached) call: %v", err)
	}
	if path1 != path2 {
		t.Errorf("expected same path on cache hit; got %q vs %q", path1, path2)
	}
}

// --- downloadLibhegel: 404 ---

func TestDownloadLibhegelMissingAsset(t *testing.T) {
	testHomeOverride(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	t.Setenv(hegelDownloadBaseURLEnv, srv.URL)

	_, err := downloadLibhegel("404.test", sha256Hex([]byte("anything")))
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("expected 'HTTP 404' in error; got %q", err)
	}
}

// --- downloadLibhegel: checksum mismatch ---

func TestDownloadLibhegelChecksumMismatch(t *testing.T) {
	testHomeOverride(t)
	asset := []byte("real")
	wrong := sha256Hex([]byte("imposter"))
	srv := startReleaseServer(t, "bad.test", asset)
	t.Setenv(hegelDownloadBaseURLEnv, srv.URL)

	dest, err := downloadLibhegel("bad.test", wrong)
	if err == nil {
		t.Fatalf("expected checksum mismatch error; got dest=%q", dest)
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("expected 'checksum mismatch' in error; got %q", err)
	}
	// File should have been cleaned up.
	if _, statErr := os.Stat(filepath.Join(libhegelCacheDir("bad.test"), libhegelAssetName())); !os.IsNotExist(statErr) {
		t.Errorf("expected partial file to be removed: %v", statErr)
	}
}

// --- downloadLibhegel: disable env ---

func TestDownloadLibhegelDisabled(t *testing.T) {
	testHomeOverride(t)
	t.Setenv(hegelDownloadDisableEnv, "1")
	_, err := downloadLibhegel("disabled.test", sha256Hex([]byte("x")))
	if err == nil {
		t.Fatal("expected error when disable env is set")
	}
	if !strings.Contains(err.Error(), hegelDownloadDisableEnv) {
		t.Errorf("expected env name in error; got %q", err)
	}
}

// --- downloadCandidate: integration with openLibhegel ---

// TestDownloadToFileHTTPError covers the non-200 status path in downloadToFile.
func TestDownloadToFileHTTPError(t *testing.T) {
	dir := t.TempDir()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := downloadToFile(srv.URL+"/", filepath.Join(dir, "out"))
	if err == nil || !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("expected HTTP 403 error; got %v", err)
	}
}

// TestDownloadToFileConnectionError covers the http.Get error path.
func TestDownloadToFileConnectionError(t *testing.T) {
	dir := t.TempDir()
	_, err := downloadToFile("http://127.0.0.1:1/unreachable", filepath.Join(dir, "out"))
	if err == nil {
		t.Fatal("expected connection error")
	}
}

// --- downloadVerifiedLibhegel: baked-in checksum lookup ---

// TestDownloadVerifiedLibhegelUnknownAsset covers the unsupported-platform
// path: an asset with no baked-in checksum fails before any network access.
func TestDownloadVerifiedLibhegelUnknownAsset(t *testing.T) {
	_, err := downloadVerifiedLibhegel("9.9.9", "libhegel-plan9-mips.so", libhegelChecksums)
	if err == nil {
		t.Fatal("expected error for asset with no baked-in checksum")
	}
	if !strings.Contains(err.Error(), "no baked-in libhegel checksum") {
		t.Errorf("expected 'no baked-in libhegel checksum' in error; got %q", err)
	}
}

// TestDownloadVerifiedLibhegelKnownAsset covers the success path: an asset
// present in the supplied checksum table is downloaded and verified against
// the baked-in digest.
func TestDownloadVerifiedLibhegelKnownAsset(t *testing.T) {
	testHomeOverride(t)
	asset := []byte("verified payload")
	name := libhegelAssetName()
	srv := startReleaseServer(t, "2.0.test", asset)
	t.Setenv(hegelDownloadBaseURLEnv, srv.URL)

	checksums := map[string]string{name: sha256Hex(asset)}
	path, err := downloadVerifiedLibhegel("2.0.test", name, checksums)
	if err != nil {
		t.Fatalf("downloadVerifiedLibhegel: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected destination to exist: %v", err)
	}
}

// TestLibhegelChecksumsCoverCurrentPlatform asserts the baked-in table has an
// entry for the host platform's asset, so the auto-downloader can verify a
// release on the platforms this build runs on.
func TestLibhegelChecksumsCoverCurrentPlatform(t *testing.T) {
	name := libhegelAssetName()
	sum, ok := libhegelChecksums[name]
	if !ok {
		t.Fatalf("no baked-in checksum for current platform asset %q", name)
	}
	if len(sum) != 64 {
		t.Errorf("checksum for %q is not a 64-char sha256 hex digest: %q", name, sum)
	}
	if _, err := hex.DecodeString(sum); err != nil {
		t.Errorf("checksum for %q is not valid hex: %v", name, err)
	}
}
