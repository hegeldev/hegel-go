//go:build !windows

package libhegel

// These tests cover the POSIX-specific embedded-library cache path.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

type fileInfoWithSys struct {
	os.FileInfo
	sys any
}

func (f fileInfoWithSys) Sys() any { return f.sys }

func TestTempCacheDirUsesUIDAndPrivateMode(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)

	dir, err := tempLibhegelCacheDir("test-version")
	if err != nil {
		t.Fatalf("tempLibhegelCacheDir: %v", err)
	}
	canonicalTempRoot, err := filepath.EvalSymlinks(tempRoot)
	if err != nil {
		t.Fatalf("resolve temp root: %v", err)
	}
	base := filepath.Join(canonicalTempRoot, fmt.Sprintf("hegel-go-%d", os.Getuid()))
	if !strings.HasPrefix(dir, base+string(filepath.Separator)) {
		t.Fatalf("cache dir %q is not beneath per-user dir %q", dir, base)
	}
	for _, path := range []string{base, dir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Errorf("mode of %s: got %#o, want 0700", path, got)
		}
	}
}

func TestTempCacheDirRepairsOwnedMode(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)
	base := filepath.Join(tempRoot, fmt.Sprintf("hegel-go-%d", os.Getuid()))
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if _, err := tempLibhegelCacheDir("test-version"); err != nil {
		t.Fatalf("tempLibhegelCacheDir: %v", err)
	}
	info, err := os.Stat(base)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("mode: got %#o, want 0700", got)
	}
}

func TestTempCacheDirRejectsSymlink(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)
	base := filepath.Join(tempRoot, fmt.Sprintf("hegel-go-%d", os.Getuid()))
	if err := os.Symlink(t.TempDir(), base); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := tempLibhegelCacheDir("test-version")
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("got %v, want unsafe-path error", err)
	}
}

func TestTempCacheDirRejectsSymlinkInOwnedBase(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)
	base := filepath.Join(tempRoot, fmt.Sprintf("hegel-go-%d", os.Getuid()))
	if err := os.Mkdir(base, 0o700); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(base, "libhegel")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := tempLibhegelCacheDir("test-version")
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("got %v, want unsafe-path error", err)
	}
}

func TestTempCacheDirRejectsWrongOwner(t *testing.T) {
	err := validatePrivateTempDir(t.TempDir(), os.Getuid()+1)
	if err == nil || !strings.Contains(err.Error(), "owned by uid") {
		t.Fatalf("got %v, want ownership error", err)
	}
}

func TestTempCacheDirRejectsNonStickySharedRoot(t *testing.T) {
	tempRoot := t.TempDir()
	if err := os.Chmod(tempRoot, 0o777); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Setenv("TMPDIR", tempRoot)

	_, err := tempLibhegelCacheDir("test-version")
	if err == nil || !strings.Contains(err.Error(), "without sticky bit") {
		t.Fatalf("got %v, want unsafe-root error", err)
	}
}

func TestTempCacheDirMissingRoot(t *testing.T) {
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	if _, err := tempLibhegelCacheDir("test-version"); err == nil || !strings.Contains(err.Error(), "resolve system temp dir") {
		t.Fatalf("got %v, want temp-root resolution error", err)
	}
}

func TestTempCacheDirInvalidVersionPath(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("TMPDIR", tempRoot)
	if _, err := tempLibhegelCacheDir("missing/child"); err == nil || !strings.Contains(err.Error(), "create private temp dir") {
		t.Fatalf("got %v, want mkdir error", err)
	}
}

func TestEnsurePrivateTempDirChmodError(t *testing.T) {
	dir := t.TempDir()
	want := fmt.Errorf("chmod denied")
	stubVar(t, &chmodTempDir, func(string, os.FileMode) error { return want })
	if err := ensurePrivateTempDir(dir, os.Getuid()); err == nil || !strings.Contains(err.Error(), want.Error()) {
		t.Fatalf("got %v, want chmod error", err)
	}
}

func TestValidatePrivateTempDirErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := validatePrivateTempDir(missing, os.Getuid()); err == nil || !strings.Contains(err.Error(), "inspect per-user temp dir") {
		t.Fatalf("missing path: got %v", err)
	}
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := validatePrivateTempDirInfo("fake", os.Getuid(), fileInfoWithSys{FileInfo: info}); err == nil || !strings.Contains(err.Error(), "cannot determine owner") {
		t.Fatalf("missing owner metadata: got %v", err)
	}
}

func TestValidateTempRootErrors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := validateTempRoot(missing, os.Getuid()); err == nil || !strings.Contains(err.Error(), "inspect system temp dir") {
		t.Fatalf("missing path: got %v", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := validateTempRoot(file, os.Getuid()); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("regular file: got %v", err)
	}
	info, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := validateTempRootInfo("fake", os.Getuid(), fileInfoWithSys{FileInfo: info}); err == nil || !strings.Contains(err.Error(), "cannot determine owner") {
		t.Fatalf("missing owner metadata: got %v", err)
	}
	foreign := fileInfoWithSys{FileInfo: info, sys: &syscall.Stat_t{Uid: uint32(os.Getuid() + 1)}}
	if err := validateTempRootInfo("fake", os.Getuid(), foreign); err == nil || !strings.Contains(err.Error(), "untrusted uid") {
		t.Fatalf("foreign owner: got %v", err)
	}
}
