package libhegel

import (
	"fmt"
	"syscall"
)

type dlhandle = syscall.Handle

func dlopen(path string) (dlhandle, error) {
	return syscall.LoadLibrary(path)
}

func dlsym(handle dlhandle, name string) (uintptr, error) {
	fn, err := syscall.GetProcAddress(handle, name)
	if err != nil {
		return fn, fmt.Errorf("GetProcAddress: %s: %w", name, err)
	}
	return fn, nil
}

func dlclose(handle dlhandle) error {
	return syscall.FreeLibrary(handle)
}
