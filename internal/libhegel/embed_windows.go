package libhegel

import (
	"fmt"
	"net/url"
	"os"
	"os/user"
	"path/filepath"

	"golang.org/x/sys/windows"
)

var currentWindowsUsername = func() (string, error) {
	current, err := user.Current()
	if err != nil {
		return "", err
	}
	return current.Username, nil
}
var windowsTempDir = os.TempDir

// Windows cache privacy is provided by the containing directory's ACL.
func cachedLibraryMatches(path string, size int64) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() == size
}

// lockLibraryCache serializes publication into dir across processes. Windows
// cannot replace a DLL that another process has mapped with LoadLibrary, so the
// caller must recheck the destination while holding this lock and only rename
// when the destination is absent.
func lockLibraryCache(dir string) (func() error, error) {
	file, err := os.OpenFile(filepath.Join(dir, ".install.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	handle := windows.Handle(file.Fd())
	overlapped := new(windows.Overlapped)
	if err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		overlapped,
	); err != nil {
		_ = file.Close()
		return nil, err
	}

	return func() error {
		unlockErr := windows.UnlockFileEx(handle, 0, 1, 0, overlapped)
		closeErr := file.Close()
		if unlockErr != nil {
			return unlockErr
		}
		return closeErr
	}, nil
}

// Windows temp directories are normally scoped to the current user by ACL.
// Include an encoded username as well so accounts using a shared system temp
// directory do not contend for the same cache and lock file.
func tempLibhegelCacheDir(version string) (string, error) {
	username, err := currentWindowsUsername()
	if err != nil {
		return "", fmt.Errorf("resolve current username: %w", err)
	}
	if username == "" {
		return "", fmt.Errorf("resolve current username: empty username")
	}
	// QueryEscape is injective and escapes Windows path separators and reserved
	// filename characters while keeping ordinary usernames readable.
	userDir := "hegel-go-" + url.QueryEscape(username)
	dir := filepath.Join(windowsTempDir(), userDir, "libhegel", version)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create temporary cache dir %s: %w", dir, err)
	}
	return dir, nil
}
