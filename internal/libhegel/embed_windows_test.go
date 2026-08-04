package libhegel

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTempLibhegelCacheDirIsStableAndUserScoped(t *testing.T) {
	root := t.TempDir()
	stubVar(t, &windowsTempDir, func() string { return root })
	stubVar(t, &currentWindowsUsername, func() (string, error) { return `DOMAIN\alice`, nil })

	first, err := tempLibhegelCacheDir("test-version")
	if err != nil {
		t.Fatalf("first tempLibhegelCacheDir: %v", err)
	}
	second, err := tempLibhegelCacheDir("test-version")
	if err != nil {
		t.Fatalf("second tempLibhegelCacheDir: %v", err)
	}
	want := filepath.Join(root, "hegel-go-DOMAIN%5Calice", "libhegel", "test-version")
	if first != want {
		t.Errorf("cache dir: got %q, want %q", first, want)
	}
	if second != first {
		t.Errorf("cache dir is not stable: first %q, second %q", first, second)
	}
	if info, err := os.Stat(first); err != nil || !info.IsDir() {
		t.Errorf("cache dir was not created: info=%v err=%v", info, err)
	}
}

func TestTempLibhegelCacheDirUsernameError(t *testing.T) {
	want := errors.New("lookup failed")
	stubVar(t, &currentWindowsUsername, func() (string, error) { return "", want })
	if _, err := tempLibhegelCacheDir("test-version"); !errors.Is(err, want) {
		t.Fatalf("got %v, want username lookup error", err)
	}
}

func TestTempLibhegelCacheDirEmptyUsername(t *testing.T) {
	stubVar(t, &currentWindowsUsername, func() (string, error) { return "", nil })
	if _, err := tempLibhegelCacheDir("test-version"); err == nil {
		t.Fatal("expected empty username error")
	}
}

func TestTempLibhegelCacheDirMkdirError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, nil, 0o600); err != nil {
		t.Fatalf("write temp-root file: %v", err)
	}
	stubVar(t, &windowsTempDir, func() string { return root })
	stubVar(t, &currentWindowsUsername, func() (string, error) { return "alice", nil })
	if _, err := tempLibhegelCacheDir("test-version"); err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestLockLibraryCacheSerializes(t *testing.T) {
	dir := t.TempDir()
	unlockFirst, err := lockLibraryCache(dir)
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}

	type result struct {
		unlock func() error
		err    error
	}
	acquired := make(chan result, 1)
	go func() {
		unlock, err := lockLibraryCache(dir)
		acquired <- result{unlock, err}
	}()

	select {
	case second := <-acquired:
		if second.err == nil {
			_ = second.unlock()
		}
		_ = unlockFirst()
		t.Fatalf("second lock completed while first lock was held: %v", second.err)
	case <-time.After(100 * time.Millisecond):
		// The second caller is blocked in LockFileEx, as intended.
	}

	if err := unlockFirst(); err != nil {
		t.Fatalf("release first lock: %v", err)
	}

	select {
	case second := <-acquired:
		if second.err != nil {
			t.Fatalf("acquire second lock: %v", second.err)
		}
		if err := second.unlock(); err != nil {
			t.Fatalf("release second lock: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second lock remained blocked after first lock was released")
	}
}
