package libhegel

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var chmodCacheDir = os.Chmod
var lockCacheDir = lockLibraryCache
var mkdirTemp = os.MkdirTemp

// writeDynamicLibrary materializes lib on disk and returns the path to dlopen.
//
// The per-version user cache is preferred. When it cannot be used, the bytes are
// extracted to a fresh directory under the system temp dir instead.
func writeDynamicLibrary(lib []byte) (string, error) {
	if len(lib) == 0 {
		return "", fmt.Errorf("no vendored libhegel for %s/%s; build libhegel yourself and set %s to its path",
			runtime.GOOS, runtime.GOARCH, LibraryPathEnv)
	}

	path, cacheErr := cachedLibrary(lib)
	if cacheErr == nil {
		return path, nil
	}
	path, tmpErr := tempLibrary(lib)
	if tmpErr != nil {
		return "", fmt.Errorf("%w (cache also unusable: %v)", tmpErr, cacheErr)
	}
	return path, nil
}

// cachedLibrary returns the per-version cached copy of lib, writing it on
// first use.
func cachedLibrary(lib []byte) (path string, err error) {
	dir, err := libhegelCacheDir(hegelVersion)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { // coverage-ignore (mkdir under user cache rarely fails)
		return "", fmt.Errorf("create cache dir %s: %w", dir, err)
	}
	if err := chmodCacheDir(dir, 0o700); err != nil {
		return "", fmt.Errorf("secure cache dir %s: %w", dir, err)
	}

	unlock, err := lockCacheDir(dir)
	if err != nil {
		return "", fmt.Errorf("lock cache dir %s: %w", dir, err)
	}
	defer func() {
		if unlockErr := unlock(); err == nil && unlockErr != nil {
			path = ""
			err = fmt.Errorf("unlock cache dir %s: %w", dir, unlockErr)
		}
	}()

	// Recheck while holding the publication lock. Another process may have
	// populated the cache between MkdirAll and acquiring the lock.
	dest := filepath.Join(dir, libhegelAssetName())
	if fi, err := os.Stat(dest); err == nil && fi.Size() == int64(len(lib)) {
		return dest, nil
	}

	return installLibrary(dir, lib)
}

// tempLibrary extracts lib to a fresh private directory under the system temp
// dir.
func tempLibrary(lib []byte) (string, error) {
	dir, err := mkdirTemp("", "hegel-go-libhegel-")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	return installLibrary(dir, lib)
}

// installLibrary writes lib into dir atomically (temp file + rename to dest),
// marked executable, and returns dest.
func installLibrary(dir string, lib []byte) (string, error) {
	tmp, err := os.CreateTemp(dir, ".libhegel-*.partial")
	if err != nil { // coverage-ignore (rare under a directory we just created)
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // best-effort cleanup of leftover partials

	if _, err := tmp.Write(lib); err != nil { // coverage-ignore (disk write error)
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
	dest := filepath.Join(dir, libhegelAssetName())
	if err := os.Rename(tmpName, dest); err != nil { // coverage-ignore
		return "", err
	}
	return dest, nil
}
