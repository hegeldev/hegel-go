package libhegel

import (
	"os"
	"path/filepath"
)

// projectRootMarkers are filenames whose presence indicates a project root directory.
var projectRootMarkers = []string{
	"go.mod",
	".git",
	"go.sum",
	"Makefile",
	"justfile",
	"Justfile",
}

// getwdFn is the function used to get the current working directory.
// Overridable in tests to simulate failures.
var getwdFn = os.Getwd

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
