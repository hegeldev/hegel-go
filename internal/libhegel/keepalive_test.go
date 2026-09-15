package libhegel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestTestCaseFreeStopsCleanup(t *testing.T) {
	var frees atomic.Int32
	syms := &symbols{TestCaseFree: func(ctx ctxT, raw testCaseT) Error {
		if ctx != 0 || raw != 7 {
			t.Errorf("TestCaseFree(%d, %d), want (0, 7)", ctx, raw)
		}
		frees.Add(1)
		return OK
	}}
	ptr := &pointer[testCaseT]{syms: syms, raw: 7, free: syms.TestCaseFree}
	ptr.cleanup = runtime.AddCleanup(ptr, func(raw testCaseT) {
		_ = syms.TestCaseFree(0, raw)
	}, ptr.raw)
	tc := &TestCase{pointer: ptr}

	tc.Free()
	tc.Free()
	if got := frees.Load(); got != 1 {
		t.Fatalf("free calls = %d, want 1", got)
	}
	if tc.raw != 0 {
		t.Fatalf("raw handle = %d after Free, want 0", tc.raw)
	}

	tc = nil
	ptr = nil
	runtime.GC()
	runtime.Gosched()
	runtime.GC()
	if got := frees.Load(); got != 1 {
		t.Fatalf("free calls after GC = %d, want 1", got)
	}

	(&TestCase{}).Free()
}

func TestInPlaceAllocatedWrappersExposeFree(t *testing.T) {
	ctx := Stub(t,
		uintptr(1), OK, // settings_new
		uintptr(2), OK, // run_start
	)
	settings, err := ctx.SettingsNew()
	if err != nil {
		t.Fatal(err)
	}
	run, err := settings.RunStart(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	run.Free()
	settings.Free()
	if run.raw != 0 || settings.raw != 0 {
		t.Fatalf("handles after Free = run %d, settings %d; want zero", run.raw, settings.raw)
	}
}

// TestNativeCallsKeepHandleOwnersAlive enforces the binding's lifetime rule:
// every function that reads a wrapper's raw handle must also pass the wrapper
// to runtime.KeepAlive, so the wrapper stays reachable for the duration of
// the native call that uses the handle.
//
// The raw handle is a plain uintptr — the collector cannot see it — and each
// wrapper's GC cleanup frees the native handle. Per runtime.AddCleanup, "a
// function argument or receiver may become unreachable at the last point
// where the object is used", which for a wrapper method is before the native
// call returns: the closure has already loaded x.syms and x.raw. Without the
// KeepAlive, a collection during the call can free the handle out from under
// the still-executing native code (observed in practice as a one-off SIGSEGV
// inside hegel_mark_complete: the *TestCase wrapper's last use was the
// MarkComplete call itself, and the cleanup's hegel_test_case_free ran while
// the native call held references into the freed handle).
func TestNativeCallsKeepHandleOwnersAlive(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "libhegel.go", nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse libhegel.go: %v", err)
	}

	// Constructors that only touch raw before registering the cleanup: no
	// cleanup exists yet, so nothing can free the handle mid-call.
	exempt := map[string]bool{
		"allocate":     true,
		"allocateInto": true,
		"NewContext":   true,
		"newContext":   true,
	}

	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || exempt[fd.Name.Name] {
			continue
		}
		rawOwners := map[string]bool{}
		kept := map[string]bool{}
		ast.Inspect(fd, func(n ast.Node) bool {
			switch e := n.(type) {
			case *ast.SelectorExpr:
				if e.Sel.Name == "raw" {
					if id, ok := e.X.(*ast.Ident); ok {
						rawOwners[id.Name] = true
					}
				}
			case *ast.CallExpr:
				sel, ok := e.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "runtime" || sel.Sel.Name != "KeepAlive" {
					return true
				}
				if len(e.Args) == 1 {
					if id, ok := e.Args[0].(*ast.Ident); ok {
						kept[id.Name] = true
					}
				}
			}
			return true
		})
		for owner := range rawOwners {
			if !kept[owner] {
				pos := fset.Position(fd.Pos())
				t.Errorf("%s (%s): %s.raw is passed to a native call without runtime.KeepAlive(%s)",
					fd.Name.Name, pos, owner, owner)
			}
		}
	}
}
