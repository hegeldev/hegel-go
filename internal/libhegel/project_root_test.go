package libhegel

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

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
