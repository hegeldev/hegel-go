package libhegel

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

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
		"allocate":   true,
		"NewContext": true,
		"newContext": true,
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
