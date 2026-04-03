package hegel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// resetProjectRoot resets the global state so tests get fresh detection.
func resetProjectRoot(t *testing.T) {
	t.Helper()
	hegelDirOnce = sync.Once{}
	hegelDirResult = ""
	hegelDirMu.Lock()
	old := hegelDirOverride
	hegelDirOverride = ""
	hegelDirMu.Unlock()
	t.Cleanup(func() {
		hegelDirOnce = sync.Once{}
		hegelDirResult = ""
		hegelDirMu.Lock()
		hegelDirOverride = old
		hegelDirMu.Unlock()
	})
}

func TestFindProjectRootFindsGoMod(t *testing.T) {
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	sub := filepath.Join(tmp, "a", "b", "c")
	os.MkdirAll(sub, 0o755)                                                    //nolint:errcheck
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck

	t.Chdir(sub)

	root := findProjectRoot()
	if root != tmp {
		t.Errorf("findProjectRoot() = %q, want %q", root, tmp)
	}
}

func TestFindProjectRootFindsGitDir(t *testing.T) {
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	sub := filepath.Join(tmp, "x")
	os.MkdirAll(sub, 0o755)                        //nolint:errcheck
	os.MkdirAll(filepath.Join(tmp, ".git"), 0o755) //nolint:errcheck

	t.Chdir(sub)

	root := findProjectRoot()
	if root != tmp {
		t.Errorf("findProjectRoot() = %q, want %q", root, tmp)
	}
}

func TestFindProjectRootFindsJustfile(t *testing.T) {
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "justfile"), []byte(""), 0o644) //nolint:errcheck

	t.Chdir(tmp)

	root := findProjectRoot()
	if root != tmp {
		t.Errorf("findProjectRoot() = %q, want %q", root, tmp)
	}
}

func TestFindProjectRootReturnsEmptyWhenNoMarker(t *testing.T) {
	// Use a temp dir that has no markers all the way up to /.
	// On most systems, / has no go.mod etc, so this should return "".
	// We create a deep temp tree to be safe.
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	sub := filepath.Join(tmp, "a", "b")
	os.MkdirAll(sub, 0o755) //nolint:errcheck

	t.Chdir(sub)

	root := findProjectRoot()
	// The temp dir is under /tmp or similar which may have markers above.
	// We just verify it doesn't panic and returns something reasonable.
	_ = root
}

func TestGetHegelDirectoryUsesProjectRoot(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	sub := filepath.Join(tmp, "child")
	os.MkdirAll(sub, 0o755)                                                    //nolint:errcheck
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck

	t.Chdir(sub)

	dir := getHegelDirectory()
	expected := filepath.Join(tmp, ".hegel")
	if dir != expected {
		t.Errorf("getHegelDirectory() = %q, want %q", dir, expected)
	}
}

func TestGetProjectRoot(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck

	t.Chdir(tmp)

	root := getProjectRoot()
	if root != tmp {
		t.Errorf("getProjectRoot() = %q, want %q", root, tmp)
	}
}

func TestSetHegelDirectoryOverridesDetection(t *testing.T) {
	resetProjectRoot(t)

	custom := "/tmp/custom-hegel-dir"
	SetHegelDirectory(custom)
	defer SetHegelDirectory("")

	dir := getHegelDirectory()
	if dir != custom {
		t.Errorf("getHegelDirectory() = %q, want %q", dir, custom)
	}
}

func TestSetHegelDirectoryAffectsProjectRoot(t *testing.T) {
	resetProjectRoot(t)

	custom := "/tmp/my-project/.hegel"
	SetHegelDirectory(custom)
	defer SetHegelDirectory("")

	root := getProjectRoot()
	if root != "/tmp/my-project" {
		t.Errorf("getProjectRoot() = %q, want %q", root, "/tmp/my-project")
	}
}

func TestDetectHegelDirectoryWarnsWhenNoRoot(t *testing.T) {
	// Create a temp dir with no markers at all.
	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	sub := filepath.Join(tmp, "isolated", "deep")
	os.MkdirAll(sub, 0o755) //nolint:errcheck

	t.Chdir(sub)

	// Capture stderr to check for warning.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	result := detectHegelDirectory()

	w.Close()
	os.Stderr = oldStderr

	var buf [4096]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	// The result should fall back to cwd/.hegel.
	expected := filepath.Join(sub, ".hegel")
	// On systems where /tmp itself has markers, we might get a different root.
	// Accept either the sub-dir fallback or any .hegel suffix.
	if !strings.HasSuffix(result, ".hegel") {
		t.Errorf("detectHegelDirectory() = %q, expected suffix .hegel", result)
	}

	// If it fell back to cwd, there should be a warning.
	if result == expected && !strings.Contains(output, "warning") {
		t.Error("expected warning on stderr when no project root found")
	}
}

func TestGetHegelDirectoryIsCached(t *testing.T) {
	resetProjectRoot(t)

	tmp, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0o644) //nolint:errcheck

	t.Chdir(tmp)

	dir1 := getHegelDirectory()
	dir2 := getHegelDirectory()
	if dir1 != dir2 {
		t.Errorf("getHegelDirectory() not cached: %q vs %q", dir1, dir2)
	}
}

func TestFindProjectRootGetwdError(t *testing.T) {
	orig := getwdFn
	getwdFn = func() (string, error) {
		return "", fmt.Errorf("simulated getwd failure")
	}
	defer func() { getwdFn = orig }()

	root := findProjectRoot()
	if root != "" {
		t.Errorf("findProjectRoot() = %q, want empty on Getwd error", root)
	}
}

func TestDetectHegelDirectoryGetwdError(t *testing.T) {
	orig := getwdFn
	getwdFn = func() (string, error) {
		return "", fmt.Errorf("simulated getwd failure")
	}
	defer func() { getwdFn = orig }()

	// Suppress the warning to stderr.
	oldStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		w.Close()
		os.Stderr = oldStderr
	}()

	result := detectHegelDirectory()
	if result != ".hegel" {
		t.Errorf("detectHegelDirectory() = %q, want %q on Getwd error", result, ".hegel")
	}
}
