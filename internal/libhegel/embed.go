package libhegel

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var chmodCacheDir = os.Chmod
var lockCacheDir = lockLibraryCache
var resolveTempCacheDir = tempLibhegelCacheDir

// writeDynamicLibrary materializes lib on disk and returns its path. The user
// cache is preferred, but sandboxes may allow neither changing nor writing it.
// In that case, fall back to a private location under the system temp dir.
// Environment variables selecting the cache and temp roots are trusted inputs:
// an attacker who can control the process environment can already load an
// arbitrary library through LibraryPathEnv. Filesystem protections within
// those roots are still validated where required.
func writeDynamicLibrary(lib []byte) (string, error) {
	if len(lib) == 0 {
		return "", fmt.Errorf("no vendored libhegel for %s/%s; build libhegel yourself and set %s to its path",
			runtime.GOOS, runtime.GOARCH, LibraryPathEnv)
	}

	path, cacheErr := cachedLibrary(lib)
	if cacheErr == nil {
		return path, nil
	}

	dir, err := resolveTempCacheDir(hegelVersion)
	if err != nil {
		return "", fmt.Errorf("prepare temporary libhegel cache: %w (user cache unavailable: %v)", err, cacheErr)
	}
	path, err = libraryInDir(dir, lib)
	if err != nil {
		return "", fmt.Errorf("materialize libhegel in temporary cache: %w (user cache unavailable: %v)", err, cacheErr)
	}
	return path, nil
}

// cachedLibrary returns the per-version copy in the user cache, installing it
// on first use.
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
	return libraryInDir(dir, lib)
}

func libraryInDir(dir string, lib []byte) (path string, err error) {
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
	if cachedLibraryMatches(dest, int64(len(lib))) {
		return dest, nil
	}

	tmp, err := os.CreateTemp(dir, ".libhegel-*.partial")
	if err != nil { // coverage-ignore (rare under user cache dir)
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
	// On Windows the publication lock guarantees dest does not exist, avoiding
	// the unsupported operation of replacing a DLL mapped by another process.
	if err := os.Rename(tmpName, dest); err != nil { // coverage-ignore
		return "", err
	}
	return dest, nil
}
