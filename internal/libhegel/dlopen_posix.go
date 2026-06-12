//go:build darwin || freebsd || linux || netbsd

package libhegel

import (
	"fmt"

	"github.com/ebitengine/purego"
)

type dlhandle = uintptr

func dlopen(path string) (dlhandle, error) {
	// RTLD_GLOBAL keeps libhegel's symbols available for relocation;
	// RTLD_NOW resolves them eagerly so a missing symbol fails here rather than at
	// first call.
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

func dlsym(handle dlhandle, name string) (uintptr, error) {
	fn, err := purego.Dlsym(handle, name)
	if err != nil {
		return fn, fmt.Errorf("dlsym: %s: %w", name, err)
	}
	return fn, nil
}

func dlclose(handle dlhandle) error {
	return purego.Dlclose(handle)
}
