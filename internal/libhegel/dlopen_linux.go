//go:build linux

package libhegel

import (
	"fmt"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

func dlopen(path string) (dlhandle, error) {
	// RTLD_GLOBAL keeps libhegel's symbols available for relocation;
	// RTLD_NOW resolves them eagerly so a missing symbol fails here rather than at
	// first call.
	h, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, withNoexecHint(path, err)
	}
	return h, nil
}

// withNoexecHint augments a dlopen failure with an actionable hint when path
// lives on a filesystem mounted noexec — the usual cause of an otherwise
// inexplicable load failure. The exec bit is mere metadata that chmod sets
// happily; the mount flag is what makes the kernel refuse the PROT_EXEC mmap
// dlopen needs. Returns err unchanged when the mount is exec-capable or its
// flags can't be read.
func withNoexecHint(path string, err error) error {
	var st unix.Statfs_t
	if statErr := unix.Statfs(path, &st); statErr != nil {
		return err
	}
	return noexecWrap(int64(st.Flags), err)
}

// noexecWrap is the pure flag-inspection half of [withNoexecHint], split out so
// both outcomes are testable without a (privileged) noexec mount.
func noexecWrap(flags int64, err error) error {
	if flags&unix.ST_NOEXEC == 0 {
		return err
	}
	return fmt.Errorf("%w (the library is on a noexec filesystem; set %s "+
		"to a copy on an exec-capable mount)", err, LibraryPathEnv)
}
