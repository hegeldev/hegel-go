//go:build (amd64 && windows) || (arm64 && (darwin || ios)) || (!amd64 && !arm64)

package libhegel

// testLocationFn uses the native C signature on platforms unaffected by the
// purego register-spill bugs worked around in the architecture-specific files.
type testLocationFn func(ctxT, settingsT, stringT, uint32, stringT, stringT) Error

func (fn testLocationFn) call(ctx ctxT, settings settingsT, file stringT, line uint32, class, function stringT) Error {
	return fn(ctx, settings, file, line, class, function)
}
