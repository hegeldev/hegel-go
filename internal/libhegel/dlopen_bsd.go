//go:build darwin || freebsd || netbsd

package libhegel

import "github.com/ebitengine/purego"

func dlopen(path string) (dlhandle, error) {
	// RTLD_GLOBAL keeps libhegel's symbols available for relocation;
	// RTLD_NOW resolves them eagerly so a missing symbol fails here rather than at
	// first call.
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}
