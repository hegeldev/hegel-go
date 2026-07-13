package libhegel

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

var chmodCacheDir = os.Chmod
var lockCacheDir = lockLibraryCache

// writeDynamicLibrary writes lib to the per-version cache directory atomically
// (temp file + rename), marked executable, and returns the destination path.
// An existing destination of the expected size is authoritative: because this
// function installs it by atomic rename, its presence short-circuits the write.
// On Windows, an inter-process lock serializes the cache check and rename so the
// rename never has to replace a DLL another process has already loaded.
// An empty lib (unsupported platform) is reported as an error before touching
// the disk.
func writeDynamicLibrary(lib []byte) (path string, err error) {
	if len(lib) == 0 {
		return "", fmt.Errorf("no vendored libhegel for %s/%s; build libhegel yourself and set %s to its path",
			runtime.GOOS, runtime.GOARCH, LibraryPathEnv)
	}

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
