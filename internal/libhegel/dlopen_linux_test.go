//go:build linux

package libhegel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// TestNoexecWrap exercises both outcomes of the flag inspection without needing
// an actual (privileged) noexec mount.
func TestNoexecWrap(t *testing.T) {
	base := errors.New("dlopen boom")

	if got := noexecWrap(0, base); got != base {
		t.Errorf("exec-capable flags: got %v, want the original error", got)
	}

	got := noexecWrap(unix.ST_NOEXEC, base)
	if !errors.Is(got, base) {
		t.Errorf("noexec hint should wrap the original error; got %v", got)
	}
	if !strings.Contains(got.Error(), "noexec") {
		t.Errorf("noexec flags: got %q, want a noexec hint", got)
	}
	if !strings.Contains(got.Error(), LibraryPathEnv) {
		t.Errorf("noexec hint should name %s; got %q", LibraryPathEnv, got)
	}
}

// TestWithNoexecHintStatfs covers the syscall wrapper: a real, exec-capable
// directory leaves the error untouched, and an unstattable path is swallowed to
// the original error.
func TestWithNoexecHintStatfs(t *testing.T) {
	base := errors.New("dlopen boom")

	if got := withNoexecHint(t.TempDir(), base); got != base {
		t.Errorf("temp dir is exec-capable: got %v, want the original error", got)
	}
	if got := withNoexecHint("/nonexistent/path/for/statfs", base); got != base {
		t.Errorf("unstattable path: got %v, want the original error", got)
	}
}

// TestDlopenNoexecHint drives the full dlopen failure path: opening a non-library
// file on a normal (exec-capable) filesystem fails, and the error passes through
// withNoexecHint without a noexec hint being appended.
func TestDlopenNoexecHint(t *testing.T) {
	junk := filepath.Join(t.TempDir(), "not-a-library.so")
	if err := os.WriteFile(junk, []byte("definitely not ELF"), 0o755); err != nil { // coverage-ignore
		t.Fatalf("write junk: %v", err)
	}

	_, err := dlopen(junk)
	if err == nil {
		t.Fatal("expected dlopen of a non-library file to fail")
	}
	if strings.Contains(err.Error(), "noexec") {
		t.Errorf("temp dir is exec-capable; did not expect a noexec hint: %v", err)
	}
}
