This package vendors `src/runtime/cgo/handle.go` and `handle_test.go` from
Go 1.27.0, unchanged, under the accompanying BSD license.

Source: https://github.com/golang/go/tree/go1.27.0/src/runtime/cgo

Keeping handles separate from `runtime/cgo` avoids linking its runtime support,
which conflicts with purego when cgo is disabled on macOS (issue #133).
