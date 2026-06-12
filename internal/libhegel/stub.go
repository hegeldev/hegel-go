package libhegel

import "fmt"

// Stub creates a Handle where libhegel return values are controlled by the caller.
//
// returns supplies one value per libhegel call, consumed in strict call order.
// The caller is responsible to provide the correct number of values with the
// correct dynamic types (e.g. uintptr for opaque handles, Error for op results).
func Stub(returns ...any) *Handle {
	var i int
	retval := func() any {
		if i >= len(returns) {
			panic(fmt.Sprintf("libhegel stub: missing %d'th return value", i+1))
		}
		v := returns[i]
		i++
		return v
	}

	// The opaque handle types (settingsT, runT, …) are unexported, so callers
	// in other packages cannot name them. Handle-returning closures therefore
	// assert a plain uintptr and convert: pass uintptr(0) to simulate a NULL
	// handle (allocation failure), any non-zero value otherwise.
	return &Handle{
		0,
		func() string { return retval().(string) },
		func() string { return retval().(string) }, // coverage-ignore (version: not exercised by the runner)
		func() settingsT { return settingsT(retval().(uintptr)) },
		func(st settingsT) {},
		func(st settingsT, m Mode) {},
		func(st settingsT, u uint64) {},
		func(st settingsT, v Verbosity) {},
		func(st settingsT, u uint64, b bool) {},
		func(st settingsT, b bool) {},
		func(st settingsT, b bool) {},
		func(st settingsT, s string) {},
		func(st settingsT, s string) {},
		func(st settingsT, p Phase) {},
		func(st settingsT, hc HealthCheck) {},
		func(st settingsT) runT { return runT(retval().(uintptr)) },
		func(rt runT) {},
		func(rt runT) testCaseT { return testCaseT(retval().(uintptr)) },
		func(rt runT) resultT { return resultT(retval().(uintptr)) },
		func(tct testCaseT, b1 *byte, u1 uint64, b2 **byte, u2 *uint64) Error { return retval().(Error) },
		func(tct testCaseT, l Label) Error { return retval().(Error) },
		func(tct testCaseT, b bool) Error { return retval().(Error) },
		func(tct testCaseT, u1, u2 uint64, c *Collection) Error { return retval().(Error) },
		func(tct testCaseT, c Collection, b *bool) Error { return retval().(Error) },
		func(tct testCaseT, c Collection, s string) Error { return retval().(Error) },
		func(tct testCaseT, f float64, s string) Error { return retval().(Error) },
		func(tct testCaseT, s1 Status, s2 string) Error { return retval().(Error) },
		func(tct testCaseT) bool { return retval().(bool) },
		func(rt resultT) bool { return retval().(bool) },
		func(rt resultT) uint64 { return retval().(uint64) },
		func(rt resultT, u uint64) failureT { return failureT(retval().(uintptr)) },
		func(ft failureT) string { return retval().(string) }, // coverage-ignore (panic_message: not wired into collectFailures)
		func(ft failureT) string { return retval().(string) }, // coverage-ignore (diagnostic: not wired into collectFailures)
		func(ft failureT) string { return retval().(string) },
	}
}
