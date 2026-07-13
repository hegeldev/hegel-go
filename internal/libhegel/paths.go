package libhegel

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// libhegelAssetName returns the platform-specific filename of the vendored
// libhegel artifact under libs/. Format: `libhegel-<goos>-<goarch>.<ext>`,
// e.g. `libhegel-linux-amd64.so`.
func libhegelAssetName() string {
	return fmt.Sprintf("libhegel-%s-%s.%s", runtime.GOOS, runtime.GOARCH, libhegelExt())
}

// libhegelExt returns the dynamic-library extension for the host OS
// ("so"/"dylib"/"dll").
func libhegelExt() string {
	switch runtime.GOOS {
	case "darwin": // coverage-ignore (unreachable on the linux CI runner)
		return "dylib"
	case "windows": // coverage-ignore (unreachable on the linux CI runner)
		return "dll"
	default:
		return "so"
	}
}

// libhegelCacheDir returns the per-version cache directory under the user's
// cache root. The caller creates it when needed.
func libhegelCacheDir(version string) (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	return filepath.Join(root, "hegel-go", "libhegel", version), nil
}
