package hegel

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// projectRootMarkers are filenames whose presence indicates a project root directory.
var projectRootMarkers = []string{
	"go.mod",
	".git",
	"go.sum",
	"Makefile",
	"justfile",
	"Justfile",
	".hegel",
}

// getwdFn is the function used to get the current working directory.
// Overridable in tests to simulate failures.
var getwdFn = os.Getwd

var (
	hegelDirOnce     sync.Once
	hegelDirResult   string
	hegelDirOverride string
	hegelDirMu       sync.Mutex
)

// SetHegelDirectory overrides the automatically detected hegel data directory.
// Call this before any hegel tests run (e.g. in TestMain) if automatic
// detection does not find the correct project root.
func SetHegelDirectory(dir string) {
	hegelDirMu.Lock()
	defer hegelDirMu.Unlock()
	hegelDirOverride = dir
}

// getHegelDirectory returns the path to the .hegel data directory.
// It is calculated once on first call by walking up from the working directory
// to find the project root, then appending ".hegel". If SetHegelDirectory was
// called, that value is used instead.
func getHegelDirectory() string {
	hegelDirMu.Lock()
	override := hegelDirOverride
	hegelDirMu.Unlock()
	if override != "" {
		return override
	}

	hegelDirOnce.Do(func() {
		hegelDirResult = detectHegelDirectory()
	})
	return hegelDirResult
}

// getProjectRoot returns the project root (parent of the .hegel directory).
func getProjectRoot() string {
	d := getHegelDirectory()
	return filepath.Dir(d)
}

func detectHegelDirectory() string {
	root := findProjectRoot()
	if root != "" {
		return filepath.Join(root, ".hegel")
	}
	fmt.Fprintf(os.Stderr,
		"hegel: warning: could not detect project root (no go.mod, .git, etc. found in parent directories). "+
			"The .hegel data directory will be created in the current working directory. "+
			"Call hegel.SetHegelDirectory() to set an explicit path.\n")
	cwd, err := getwdFn()
	if err != nil {
		cwd = "."
	}
	return filepath.Join(cwd, ".hegel")
}

// findProjectRoot walks upward from the current working directory looking for
// project root markers (go.mod, .git, etc.). Returns the directory containing
// the marker, or "" if none is found.
func findProjectRoot() string {
	cwd, err := getwdFn()
	if err != nil {
		return ""
	}
	dir := cwd
	for {
		for _, marker := range projectRootMarkers {
			if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
