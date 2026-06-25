package libhegel

import "fmt"

// Stub creates a [Context] whose libhegel return values are controlled by the
// caller, backed by an in-memory symbol table (no dlopen).
//
// returns supplies one value per libhegel call that produces output, consumed
// in strict call order. The caller is responsible for providing the correct
// number of values with the correct dynamic types:
//
//   - handle constructors (settings_new, run_start, next_test_case, run_result,
//     test_case_from_blob, run_result_failure) pop either a uintptr (the
//     produced handle; 0 means "no object") or an [Error] (the call fails with
//     that code, after which the matching error message is read from the
//     context's last-error reader);
//   - fallible operations (the settings setters, generate, spans, collection,
//     pool, …) pop an [Error];
//   - readers (run_result_status, _failure_count) pop their value;
//   - string readers (context_last_error, run_result_error, the failure
//     getters) pop a Go string.
func Stub(returns ...any) *Context {
	var i int
	retval := func() any {
		if i >= len(returns) {
			panic(fmt.Sprintf("libhegel stub: missing %d'th return value", i+1))
		}
		v := returns[i]
		i++
		return v
	}

	// retHandle pops either an Error (the constructor fails) or a uintptr (the
	// produced handle). The opaque handle types are unexported, so callers pass
	// a plain uintptr — 0 simulates a NULL handle (e.g. a finished run).
	retHandle := func() (uintptr, Error) {
		v := retval()
		if e, ok := v.(Error); ok {
			return 0, e
		}
		return v.(uintptr), OK
	}

	// writeStr pops a Go string and writes it into a C `const char**`
	// out-parameter as a NUL-terminated buffer. The buffer stays alive through
	// the returned pointer until the immediate goString copy on the Go side.
	writeStr := func(out **byte) Error {
		buf := append([]byte(retval().(string)), 0)
		*out = &buf[0]
		return OK
	}

	syms := &symbols{
		0,
		func() ctxT { return 1 },
		func(ctxT) Error { return OK },
		func(ctxT) string { return retval().(string) },

		func(_ ctxT, out *settingsT) Error { h, e := retHandle(); *out = settingsT(h); return e },
		func(ctxT, settingsT) Error { return OK },
		func(ctxT, settingsT, Mode) Error { return retval().(Error) },
		func(ctxT, settingsT, Backend) Error { return retval().(Error) },
		func(ctxT, settingsT, uint64) Error { return retval().(Error) },
		func(ctxT, settingsT, Verbosity) Error { return retval().(Error) },
		func(ctxT, settingsT, uint64, bool) Error { return retval().(Error) },
		func(ctxT, settingsT, bool) Error { return retval().(Error) },
		func(ctxT, settingsT, bool) Error { return retval().(Error) },
		func(ctxT, settingsT, string) Error { return retval().(Error) },
		func(ctxT, settingsT, string) Error { return retval().(Error) },
		func(ctxT, settingsT, Phase) Error { return retval().(Error) },
		func(ctxT, settingsT, HealthCheck) Error { return retval().(Error) },

		func(_ ctxT, _ settingsT, out *runT) Error { h, e := retHandle(); *out = runT(h); return e },
		func(ctxT, runT) Error { return OK },
		func(_ ctxT, _ runT, out *testCaseT) Error { h, e := retHandle(); *out = testCaseT(h); return e },
		func(_ ctxT, _ runT, out *resultT) Error { h, e := retHandle(); *out = resultT(h); return e },

		func(_ ctxT, _ settingsT, _ string, out *testCaseT) Error {
			h, e := retHandle()
			*out = testCaseT(h)
			return e
		},
		func(ctxT, testCaseT) Error { return OK },

		func(ctxT, testCaseT, *byte, uint64, **byte, *uint64) Error { return retval().(Error) },
		func(ctxT, testCaseT, Label) Error { return retval().(Error) },
		func(ctxT, testCaseT, bool) Error { return retval().(Error) },
		func(ctxT, testCaseT, uint64, uint64, *Collection) Error { return retval().(Error) },
		func(ctxT, testCaseT, Collection, *bool) Error { return retval().(Error) },
		func(ctxT, testCaseT, Collection, string) Error { return retval().(Error) },
		func(ctxT, testCaseT, *int64) Error { return retval().(Error) },
		func(ctxT, testCaseT, int64, *int64) Error { return retval().(Error) },
		func(ctxT, testCaseT, int64, bool, *int64) Error { return retval().(Error) },
		func(ctxT, testCaseT, **byte, uint64, **byte, uint64, *StateMachine) Error { return retval().(Error) },
		func(ctxT, testCaseT, StateMachine, *int64) Error { return retval().(Error) },
		func(ctxT, testCaseT, float64, bool, bool, *bool) Error { return retval().(Error) },
		func(ctxT, testCaseT, float64, string) Error { return retval().(Error) },
		func(ctxT, testCaseT, Status, string) Error { return retval().(Error) },

		func(_ ctxT, _ resultT, out *RunStatus) Error { *out = retval().(RunStatus); return OK },
		func(_ ctxT, _ resultT, out **byte) Error { return writeStr(out) },
		func(_ ctxT, _ resultT, out *uint64) Error { *out = retval().(uint64); return OK },
		func(_ ctxT, _ resultT, _ uint64, out *failureT) Error {
			h, e := retHandle()
			*out = failureT(h)
			return e
		},

		func(_ ctxT, _ failureT, out **byte) Error { return writeStr(out) },
		func(_ ctxT, _ failureT, out **byte) Error { return writeStr(out) },

		func(_ ctxT, out **byte) Error { return writeStr(out) },
	}

	return &Context{syms: syms, raw: 1}
}
