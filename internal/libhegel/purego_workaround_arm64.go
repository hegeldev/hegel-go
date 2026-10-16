//go:build arm64 && !darwin && !ios

package libhegel

// testLocationFn works around purego splitting the final string across x7 and
// the stack, which violates AAPCS64. The dummy argument reserves unused x7 so
// the function-name string goes entirely on the stack.
// https://github.com/ebitengine/purego/issues/544
// https://github.com/ebitengine/purego/pull/517
//
// TODO: Remove the padding when the fix is available in the pinned purego version.
type testLocationFn func(ctxT, settingsT, stringT, uint32, stringT, uintptr, stringT) Error

func (fn testLocationFn) call(ctx ctxT, settings settingsT, file stringT, line uint32, class, function stringT) Error {
	return fn(ctx, settings, file, line, class, 0, function)
}
