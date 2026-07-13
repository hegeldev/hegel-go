//go:build darwin || freebsd || linux || netbsd

package libhegel

import (
	"fmt"

	"github.com/ebitengine/purego"
)

type dlhandle = uintptr

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
