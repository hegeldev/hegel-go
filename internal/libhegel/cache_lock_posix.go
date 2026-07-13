//go:build !windows

package libhegel

// lockLibraryCache is unnecessary on POSIX: rename replaces an open inode
// while existing users continue to refer to the old inode.
func lockLibraryCache(string) (func() error, error) {
	return func() error { return nil }, nil
}
