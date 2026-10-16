//go:build amd64 && !windows

package libhegel

// testLocationFn works around purego splitting a struct across registers and
// the stack, which violates the System V ABI. The dummy argument reserves the
// unused sixth register so both trailing strings go entirely on the stack.
// https://github.com/ebitengine/purego/issues/525
//
// TODO: Remove the padding when this bug is fixed in the pinned purego version.
type testLocationFn func(ctxT, settingsT, stringT, uint32, uintptr, stringT, stringT) Error

func (fn testLocationFn) call(ctx ctxT, settings settingsT, file stringT, line uint32, class, function stringT) Error {
	return fn(ctx, settings, file, line, 0, class, function)
}
