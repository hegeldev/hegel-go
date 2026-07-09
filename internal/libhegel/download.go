package libhegel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// hegelDownloadBaseURL is the GitHub release base URL libhegel artifacts are
// fetched from. Overridable via [hegelDownloadBaseURLEnv] for testing.
const hegelDownloadBaseURL = "https://github.com/hegeldev/hegel-rust/releases/download"

// hegelDownloadBaseURLEnv lets tests (and CI staging) point downloads at an
// alternate base URL. The path layout under the override URL must match the
// production layout: `<base>/v<version>/<asset>` for the library.
const hegelDownloadBaseURLEnv = "HEGEL_DOWNLOAD_BASE_URL"

// hegelDownloadDisableEnv, when set to a non-empty value, disables the
// on-demand downloader entirely. Useful for offline development and CI
// environments that must rely on a pre-staged library.
const hegelDownloadDisableEnv = "HEGEL_LIBHEGEL_NO_DOWNLOAD"

// libhegelAssetName returns the platform-specific filename of the libhegel
// artifact published in hegel-rust GitHub releases. Format:
// `libhegel-<goos>-<goarch>.<ext>`, e.g. `libhegel-linux-amd64.so`.
func libhegelAssetName() string {
	return fmt.Sprintf("libhegel-%s-%s.%s", runtime.GOOS, runtime.GOARCH, libhegelExt())
}

// libhegelExt returns the dynamic-library extension for the host OS
// ("so"/"dylib"/"dll"), defined per-platform in the libext_*.go files.
func libhegelExt() string {
	switch runtime.GOOS {
	case "darwin": // coverage-ignore (unreachable on the linux CI runner)
		return "dylib"
	case "windows": // coverage-ignore (unreachable on the linux CI runner)
		return "dll"
	default:
		return "so"
	}
}

// libhegelCacheDir returns the per-version cache directory under the user's
// cache root, creating parent dirs as needed (but not the leaf — the caller
// creates it). Falls back to a temp-dir-based location if UserCacheDir fails.
func libhegelCacheDir(version string) string {
	root, err := os.UserCacheDir()
	if err != nil { // coverage-ignore (UserCacheDir is robust on supported platforms)
		root = os.TempDir()
	}
	return filepath.Join(root, "hegel-go", "libhegel", version)
}

// libhegelDownloadURL returns the URL for the libhegel artifact at version.
func libhegelDownloadURL(version string) string {
	base := os.Getenv(hegelDownloadBaseURLEnv)
	if base == "" {
		base = hegelDownloadBaseURL
	}
	base = strings.TrimSuffix(base, "/")
	return fmt.Sprintf("%s/v%s/%s", base, version, libhegelAssetName())
}

// downloadLibhegel fetches the libhegel artifact for the given version from
// GitHub releases (or the override base URL) into the per-version cache dir,
// verifying its SHA-256 against wantSum. Returns the path of the downloaded
// library on success.
//
// If the cache already contains a verified copy, the network is not touched.
//
// Disabled when HEGEL_LIBHEGEL_NO_DOWNLOAD is set; returns a descriptive
// error in that case.
func downloadLibhegel(version, wantSum string) (string, error) {
	if os.Getenv(hegelDownloadDisableEnv) != "" {
		return "", fmt.Errorf("libhegel auto-download disabled by %s", hegelDownloadDisableEnv)
	}

	dir := libhegelCacheDir(version)
	dest := filepath.Join(dir, libhegelAssetName())

	if _, err := os.Stat(dest); err == nil {
		return dest, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil { // coverage-ignore (mkdir under user cache rarely fails)
		return "", fmt.Errorf("create cache dir %s: %w", dir, err)
	}

	libURL := libhegelDownloadURL(version)
	gotSum, err := downloadToFile(libURL, dest)
	if err != nil {
		_ = os.Remove(dest) // best-effort cleanup of a partial file
		return "", fmt.Errorf("download %s: %w", libURL, err)
	}
	if gotSum != wantSum {
		_ = os.Remove(dest)
		return "", fmt.Errorf("checksum mismatch for %s: got %s, want %s", libURL, gotSum, wantSum)
	}
	return dest, nil
}

// downloadToFile streams url's body into dest atomically (via a temp file +
// rename) and returns the SHA-256 of the streamed bytes.
func downloadToFile(url, dest string) (string, error) {
	resp, err := http.Get(url) //nolint:gosec,noctx // user-supplied URL, deliberate
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".libhegel-*.partial")
	if err != nil { // coverage-ignore (rare under user cache dir)
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // best-effort cleanup of leftover partials

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), resp.Body); err != nil { // coverage-ignore (network/disk error mid-stream)
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil { // coverage-ignore
		return "", err
	}
	// Mark executable so dlopen can use it without further chmod.
	if err := os.Chmod(tmpName, 0o755); err != nil { // coverage-ignore
		return "", err
	}
	// atomicRename replaces dest even when a concurrently-running test binary
	// has already downloaded and loaded it. On Windows that requires POSIX
	// rename semantics (a plain MoveFileEx fails with "Access is denied"
	// against a LoadLibrary-locked DLL); on other platforms it is os.Rename.
	if err := atomicRename(tmpName, dest); err != nil { // coverage-ignore
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// downloadCandidate triggers an on-demand download to the per-version cache
// dir and returns the path of the downloaded library. Used by [load] when no
// HEGEL_LIBHEGEL_PATH override is set. The download is verified against the
// baked-in checksum for the host platform's asset.
func downloadCandidate() (string, error) { // coverage-ignore (exercised only against the network, not in unit tests)
	return downloadVerifiedLibhegel(hegelVersion, libhegelAssetName(), libhegelChecksums)
}

// downloadVerifiedLibhegel resolves the expected checksum for asset from
// checksums, then downloads and verifies version's artifact. It errors before
// touching the network if no checksum is baked in for asset — i.e. the host
// platform has no published, pinned artifact.
func downloadVerifiedLibhegel(version, asset string, checksums map[string]string) (string, error) {
	wantSum, ok := checksums[asset]
	if !ok {
		return "", fmt.Errorf("no baked-in libhegel checksum for %s at version %s (unsupported platform)", asset, version)
	}
	return downloadLibhegel(version, wantSum)
}
