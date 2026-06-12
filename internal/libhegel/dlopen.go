package libhegel

import (
	"github.com/ebitengine/purego"
)

type symbol struct {
	name string
	dst  any // *func(...) — passed to purego.RegisterLibFunc
}

func registerSymbols(handle dlhandle, symbols []symbol) error {
	for _, sym := range symbols {
		fn, err := dlsym(handle, sym.name)
		if err != nil {
			return err
		}
		purego.RegisterFunc(sym.dst, fn)
	}
	return nil
}
