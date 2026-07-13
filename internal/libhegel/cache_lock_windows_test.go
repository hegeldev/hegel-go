package libhegel

import (
	"testing"
	"time"
)

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
