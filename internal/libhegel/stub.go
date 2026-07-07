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

			// Destructors free their argument rather than producing output, so
			// they never consume a scripted return value for it — even now that
			// some (e.g. GenerateBytesFree) take a *bytesResult/*stringResult
			// pointer that would otherwise match an output-param case below.
			if !dtor {
				for i := range ft.NumIn() {
					arg, in := args[i], ft.In(i)
					switch {
					case in == reflect.TypeFor[**byte]():
						buf := append([]byte(retval().(string)), 0)
						arg.Elem().Set(reflect.ValueOf(&buf[0]))
					case in == reflect.TypeFor[*uint64](), in == reflect.TypeFor[*RunStatus](),
						in == reflect.TypeFor[*bool](), in == reflect.TypeFor[*int64](),
						in == reflect.TypeFor[*float64](), in == reflect.TypeFor[*Collection](),
						in == reflect.TypeFor[*StateMachine](), in == reflect.TypeFor[*Date](),
						in == reflect.TypeFor[*Time](), in == reflect.TypeFor[*Datetime]():
						arg.Elem().Set(reflect.ValueOf(retval()))
					case in == reflect.TypeFor[outBuf]():
						b := retval().([]byte)
						copy(unsafe.Slice((*byte)(arg.Interface().(outBuf)), len(b)), b)
					case in == reflect.TypeFor[*bytesResult]():
						b := retval().([]byte)
						arg.Elem().Set(reflect.ValueOf(bytesResult{data: slicePtr(b), len: uint64(len(b))}))
					case in == reflect.TypeFor[*stringResult]():
						b := []byte(retval().(string))
						arg.Elem().Set(reflect.ValueOf(stringResult{data: slicePtr(b), len: uint64(len(b))}))
					case in.ConvertibleTo(reflect.TypeFor[*uintptr]()):
						h := reflect.ValueOf(retval()).Convert(in.Elem())
						handles.track(h.Interface())
						arg.Elem().Set(h)
					default:
						continue
					}
				}
			}

			out := ft.Out(0)
			switch {
			case out == reflect.TypeFor[Error]():
				// Destructors always succeed and script no return value.
				if dtor {
					return []reflect.Value{reflect.ValueOf(OK)}
				}
				return []reflect.Value{reflect.ValueOf(retval().(Error))}
			case out == reflect.TypeFor[string]():
				return []reflect.Value{reflect.ValueOf(retval().(string))}
			case out == reflect.TypeFor[ctxT]():
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
