//go:build !windows

package libhegel

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var chmodTempDir = os.Chmod

// cachedLibraryMatches refuses symlinks, non-regular files, and files planted
// by another UID. The containing directory may only just have been repaired
// from a permissive mode, so size alone is not a sufficient trust check.
func cachedLibraryMatches(path string, size int64) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Getuid())
}

// lockLibraryCache is unnecessary on POSIX: rename replaces an open inode
// while existing users continue to refer to the old inode.
func lockLibraryCache(string) (func() error, error) {
	return func() error { return nil }, nil
}

// tempLibhegelCacheDir returns a stable per-user cache beneath the system temp
// directory. The UID is not itself a security boundary: an existing base must
// also be a real directory owned by that UID before its permissions are
// changed or a library is loaded from it.
func tempLibhegelCacheDir(version string) (string, error) {
	uid := os.Getuid()
	root, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		return "", fmt.Errorf("resolve system temp dir: %w", err)
	}
	if err := validateTempRoot(root, uid); err != nil {
		return "", err
	}

	base := filepath.Join(root, fmt.Sprintf("hegel-go-%d", uid))
	if err := ensurePrivateTempDir(base, uid); err != nil {
		return "", err
	}
	libhegelDir := filepath.Join(base, "libhegel")
	if err := ensurePrivateTempDir(libhegelDir, uid); err != nil {
		return "", err
	}
	dir := filepath.Join(libhegelDir, version)
	if err := ensurePrivateTempDir(dir, uid); err != nil {
		return "", err
	}
	return dir, nil
}

// ensurePrivateTempDir relies on path's parent already being protected: it is
// either a validated sticky temp root or an owner-controlled 0700 directory
// secured by the preceding call. Once Lstat confirms path is owned by uid, a
// different unprivileged UID cannot replace that directory entry before
// Chmod. Each child is inspected only after its parent has been secured.
func ensurePrivateTempDir(path string, uid int) error {
	if err := os.Mkdir(path, 0o700); err != nil && !os.IsExist(err) {
		return fmt.Errorf("create private temp dir %s: %w", path, err)
	}
	if err := validatePrivateTempDir(path, uid); err != nil {
		return err
	}
	if err := chmodTempDir(path, 0o700); err != nil {
		return fmt.Errorf("secure private temp dir %s: %w", path, err)
	}
	return nil
}

func validatePrivateTempDir(path string, uid int) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect per-user temp dir %s: %w", path, err)
	}
	return validatePrivateTempDirInfo(path, uid, info)
}

func validatePrivateTempDirInfo(path string, uid int, info os.FileInfo) error {
	if !info.IsDir() {
		return fmt.Errorf("refuse unsafe per-user temp path %s: not a directory", path)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("refuse unsafe per-user temp dir %s: cannot determine owner", path)
	}
	if stat.Uid != uint32(uid) {
		return fmt.Errorf("refuse unsafe per-user temp dir %s: owned by uid %d, want %d", path, stat.Uid, uid)
	}
	return nil
}

// validateTempRoot ensures another unprivileged user cannot rename the private
// directory between the ownership check above and later path-based operations.
// Shared roots such as /tmp are safe only when the sticky bit is set.
func validateTempRoot(root string, uid int) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("inspect system temp dir %s: %w", root, err)
	}
	return validateTempRootInfo(root, uid, info)
}

func validateTempRootInfo(root string, uid int, info os.FileInfo) error {
	if !info.IsDir() {
		return fmt.Errorf("refuse unsafe system temp path %s: not a directory", root)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("refuse unsafe system temp dir %s: cannot determine owner", root)
	}
	if stat.Uid != 0 && stat.Uid != uint32(uid) {
		return fmt.Errorf("refuse unsafe system temp dir %s: owned by untrusted uid %d", root, stat.Uid)
	}
	if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
		return fmt.Errorf("refuse unsafe system temp dir %s: writable by other users without sticky bit", root)
	}
	return nil
}
