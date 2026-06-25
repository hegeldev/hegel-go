//go:build !windows

package libhegel

import "os"

// atomicRename replaces newpath with oldpath. On POSIX systems rename(2) is
// already atomic and replaces the destination even while another process holds
// it open (the old inode lives on until the last handle closes), so a plain
// os.Rename is all that is needed.
func atomicRename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
