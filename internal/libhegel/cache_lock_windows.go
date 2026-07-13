package libhegel

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

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
