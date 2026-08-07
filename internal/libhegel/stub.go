package libhegel

import (
	"reflect"
	"runtime"
	"strings"
	"sync"
	"unsafe"
)

type testingTB interface {
	Helper()
	Errorf(format string, args ...any)
}

// Stub creates a [Context] whose libhegel return values are controlled by the
// caller, backed by an in-memory symbol table.
//
// Pass a plain uintptr to return a handle type.
func Stub(tb testingTB, returns ...any) *Context {
	var i int
	retval := func() any {
		if i >= len(returns) {
			tb.Helper()
			tb.Errorf("libhegel stub: missing %d'th return value", i+1)
		}
		v := returns[i]
		i++
		return v
	}

	handles := newHandleTracker()
	syms := &symbols{}
	sv := reflect.ValueOf(syms).Elem()
	for i := range sv.NumField() {
		f := sv.Field(i)
		if f.Kind() != reflect.Func {
			continue
		}

		name := sv.Type().Field(i).Name
		dtor := strings.Contains(name, "Free")
		ft := f.Type()

		f.Set(reflect.MakeFunc(ft, func(args []reflect.Value) (results []reflect.Value) {
			tb.Helper()
			for i, a := range args {
				if handles.check(a.Interface()) {
					tb.Errorf("libhegel stub: %s: use-after-free: argument %d (%T) used after its owner was freed", name, i, a.Interface())
				}
			}

			// Force any now-unreachable wrapper to run its cleanup once we
			// return; the Gosched between collections lets the cleanup
			// goroutine actually run.
			defer func() {
				runtime.GC()
				runtime.Gosched()
				runtime.GC()
			}()

			var lastHandle any
			for i := range ft.NumIn() {
				if ft.In(i).Kind() == reflect.Uintptr {
					lastHandle = args[i].Interface()
				}
			}

			if dtor {
				handles.free(lastHandle)
			}

			// Fill every output parameter from the scripted return values. An
			// output is any out[T] argument — recognized by reflect.Type
			// identity, or, for a handle out-param, by the structural shape of
			// a pointer to a ~uintptr handle. Every other pointer (const input
			// buffers, `const char *const *` string arrays, and the
			// *bytesResult/*stringResult a Free takes to release its buffer) is
			// an input the stub leaves untouched. Nothing is matched by name.
			//
			// Destructors have no out[T] parameters, so this loop is a no-op for
			// them and consumes no scripted return value.
			for i := range ft.NumIn() {
				arg, in := args[i], ft.In(i)
				if in.Kind() != reflect.Pointer {
					continue // by-value input
				}
				switch {
				case in.Elem().Kind() == reflect.Uintptr:
					// out[handle]: pointer to a ~uintptr handle. No bare *handle
					// inputs exist, so this shape unambiguously marks an output.
					h := reflect.ValueOf(retval()).Convert(in.Elem())
					handles.track(h.Interface())
					arg.Elem().Set(h)
				case in == reflect.TypeFor[out[*byte]]():
					// **byte: the engine writes a NUL-terminated C string.
					buf := append([]byte(retval().(string)), 0)
					arg.Elem().Set(reflect.ValueOf(&buf[0]))
				case in == reflect.TypeFor[out[byte]]():
					// *byte: a fixed-width buffer the engine fills in place.
					b := retval().([]byte)
					copy(unsafe.Slice((*byte)(arg.Interface().(out[byte])), len(b)), b)
				case in == reflect.TypeFor[out[bytesResult]]():
					b := retval().([]byte)
					arg.Elem().Set(reflect.ValueOf(bytesResult{data: slicePtr(b), len: uint64(len(b))}))
				case in == reflect.TypeFor[out[stringResult]]():
					b := []byte(retval().(string))
					arg.Elem().Set(reflect.ValueOf(stringResult{data: slicePtr(b), len: uint64(len(b))}))
				case in == reflect.TypeFor[out[uint64]](), in == reflect.TypeFor[out[RunStatus]](),
					in == reflect.TypeFor[out[bool]](), in == reflect.TypeFor[out[int64]](),
					in == reflect.TypeFor[out[float64]](), in == reflect.TypeFor[out[Date]](),
					in == reflect.TypeFor[out[Time]](), in == reflect.TypeFor[out[Datetime]]():
					arg.Elem().Set(reflect.ValueOf(retval()))
				default:
					continue // input pointer
				}
			}

			ret := ft.Out(0)
			switch {
			case ret == reflect.TypeFor[Error]():
				// Destructors always succeed and script no return value.
				if dtor {
					return []reflect.Value{reflect.ValueOf(OK)}
				}
				return []reflect.Value{reflect.ValueOf(retval().(Error))}
			case ret == reflect.TypeFor[string]():
				return []reflect.Value{reflect.ValueOf(retval().(string))}
			case ret == reflect.TypeFor[ctxT]():
				// hegel_context_new: hand back a fixed non-NULL handle.
				return []reflect.Value{reflect.ValueOf(ctxT(1))}
			default: // coverage-ignore (every libhegel symbol returns Error, string or ctxT)
				tb.Errorf("libhegel stub: unsupported return value %s", ft)
				return nil
			}
		}))
	}

	return &Context{syms: syms, raw: 1}
}

type handleTracker struct {
	mu    sync.Mutex
	alive map[any]bool
}

func newHandleTracker() *handleTracker {
	return &handleTracker{alive: map[any]bool{}}
}

// A NULL handle (raw value 0) is libhegel's "no object" sentinel. The tracker
// treats it as nothing to track, so callers never have to special-case a failed
// or absent handle. Handles are stored boxed in the maps, keyed by their typed
// value, so handles that share a raw value but differ in type stay distinct.

// track records a produced handle, born alive. A NULL (zero) handle is ignored.
func (h *handleTracker) track(handle any) {
	if reflect.ValueOf(handle).Uint() == 0 {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.alive[handle] = true
}

// free marks a handle dead and cascades to everything borrowed from it.
func (h *handleTracker) free(handle any) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Mark dead rather than delete: a freed handle must stay distinguishable
	// from a never-tracked one so handleCheck can tell a use-after-free from an
	// untracked argument.
	h.alive[handle] = false
}

// check reports whether handle is a tracked handle that has been freed (a
// use-after-free). A never-tracked handle or any NULL is not, so the caller
// can pass every call argument without first classifying which are handles.
func (h *handleTracker) check(handle any) bool {
	h.mu.Lock()
	alive, tracked := h.alive[handle]
	h.mu.Unlock()
	return tracked && !alive && reflect.ValueOf(handle).Uint() != 0
}
