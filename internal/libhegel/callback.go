package libhegel

import (
	"io"
	"runtime"
	"runtime/cgo"
	"unsafe"

	"github.com/ebitengine/purego"
)

func newOutputFn(w io.Writer) (outputCallbackT, cgo.Handle) {
	if w != nil {
		return outputCallbackPtr, cgo.NewHandle(w)
	}
	return 0, 0
}

func freeOutputFn[T ~uintptr](ptr *pointer[T], h cgo.Handle) {
	if h == 0 {
		return
	}

	if ptr == nil {
		// The C call failed, so there is no object whose GC will run the
		// cleanup: delete the handle now. Falling through to AddCleanup with a
		// nil ptr would panic.
		h.Delete()
		return
	}

	runtime.AddCleanup(ptr, cgo.Handle.Delete, h)
}

var outputCallbackPtr = outputCallbackT(purego.NewCallback(outputCallback))

// The return value is unused: hegel_output_callback_t is void-returning, so
// the engine ignores it. It exists only because purego.NewCallback maps to
// syscall.NewCallback on Windows, which requires exactly one uintptr result.
func outputCallback(userData uintptr, line *byte, length uint) uintptr {
	w := cgo.Handle(userData).Value().(io.Writer)
	// unsafe.String aliases the C buffer; io.WriteString copies the bytes into
	// the writer before this returns, so no separate copy is needed.
	_, _ = io.WriteString(w, unsafe.String(line, int(length)))
	return 0
}
