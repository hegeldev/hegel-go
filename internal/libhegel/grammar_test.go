package libhegel

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestGrammarImports(t *testing.T) {
	ctx := NewContext()
	builder, err := ctx.GrammarBuilderNew("%import dir.custom.word\nstart: word\n")
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	err = builder.ResolveImports(ctx, func(path string) (string, bool) {
		runtime.GC()
		calls = append(calls, path)
		switch path {
		case "dir/custom.lark":
			return "%import .leaf.WORD\nword: WORD\n", true
		case "dir/leaf.lark":
			return "WORD: \"hello\"\n", true
		default:
			t.Fatalf("unexpected import %q", path)
			return "", false
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(calls, ","); got != "dir/custom.lark,dir/leaf.lark" {
		t.Fatalf("imports = %s", got)
	}
	grammar, err := ctx.GrammarNew(builder, "", 0, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	(*pointer[grammarBuilderT])(builder).Free()
	settings, err := ctx.SettingsNewForProfile("base")
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Database(ctx, ""); err != nil {
		t.Fatal(err)
	}
	run, err := settings.RunStart(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc, err := run.NextTestCase(ctx)
	if err != nil || tc == nil {
		t.Fatalf("NextTestCase = %v, %v", tc, err)
	}
	if got, err := tc.GenerateGrammar(ctx, grammar, 0); err != nil || got != "hello" {
		t.Fatalf("GenerateGrammar = %q, %v", got, err)
	}
	if err := tc.MarkComplete(ctx, STATUS_VALID, ""); err != nil {
		t.Fatal(err)
	}
}

func TestGrammarImportFallback(t *testing.T) {
	for _, resolve := range []GrammarImport{nil, func(path string) (string, bool) {
		if path != "common.lark" {
			t.Fatalf("import = %q", path)
		}
		return "", false
	}} {
		ctx := NewContext()
		builder, err := ctx.GrammarBuilderNew("%import common.INT\nstart: INT\n")
		if err != nil {
			t.Fatal(err)
		}
		if err := builder.ResolveImports(ctx, resolve); err != nil {
			t.Fatal(err)
		}
		if grammar, err := ctx.GrammarNew(builder, "", 0, 0, nil, nil); err != nil || grammar == nil {
			t.Fatalf("GrammarNew = %v, %v", grammar, err)
		}
	}
}

func TestGrammarImportMissingAndEmpty(t *testing.T) {
	ctx := NewContext()
	builder, err := ctx.GrammarBuilderNew("%import .missing.WORD\nstart: WORD\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.ResolveImports(ctx, nil); !errors.Is(err, E_INVALID_ARG) || !strings.Contains(err.Error(), "missing.lark") {
		t.Fatalf("missing import: %v", err)
	}
	// Existing empty files must not fall back to bundled grammars.
	builder, err = ctx.GrammarBuilderNew("%import common.INT\nstart: INT\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.Provide(ctx, 0, "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.GrammarNew(builder, "", 0, 0, nil, nil); !errors.Is(err, E_INVALID_ARG) {
		t.Fatalf("empty file: %v", err)
	}
}

func TestStubGrammar(t *testing.T) {
	ctx := Stub(t, uintptr(1), OK, uintptr(1), OK, "<<EXPLICIT_FOO_0>>", OK, "a\x00b", OK)
	builder, err := ctx.GrammarBuilderNew("start: FOO")
	if err != nil {
		t.Fatal(err)
	}
	alphabet := &StringGenerator{syms: ctx.syms, raw: 9}
	grammar, err := ctx.GrammarNew(builder, "start", 16, 4, alphabet, []string{"FOO"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := grammar.Placeholder(ctx, "FOO"); err != nil || got != "<<EXPLICIT_FOO_0>>" {
		t.Fatalf("Placeholder = %q, %v", got, err)
	}
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: ctx.syms, raw: 1}}
	if got, err := tc.GenerateGrammar(ctx, grammar, 8); err != nil || got != "a\x00b" {
		t.Fatalf("GenerateGrammar = %q, %v", got, err)
	}
}

func TestStubGrammarErrors(t *testing.T) {
	for _, code := range []Error{OK, E_INVALID_ARG} {
		var diagnostics []any
		if code != OK {
			diagnostics = []any{"bad grammar"}
		}
		ctx := Stub(t, append([]any{uintptr(0), code}, diagnostics...)...)
		if builder, err := ctx.GrammarBuilderNew("???"); builder != nil || (err != nil) != (code != OK) {
			t.Fatalf("builder = %v, %v", builder, err)
		}
		ctx = Stub(t, append([]any{uintptr(0), code}, diagnostics...)...)
		builder := &GrammarBuilder{syms: ctx.syms, raw: 1}
		if grammar, err := ctx.GrammarNew(builder, "", 0, 0, nil, nil); grammar != nil || (err != nil) != (code != OK) {
			t.Fatalf("grammar = %v, %v", grammar, err)
		}
	}
	ctx := Stub(t, "", E_INVALID_ARG, "no terminal", "", E_ASSUME, "rejected")
	grammar := &Grammar{syms: ctx.syms, raw: 1}
	if got, err := grammar.Placeholder(ctx, "missing"); got != "" || !errors.Is(err, E_INVALID_ARG) {
		t.Fatalf("Placeholder = %q, %v", got, err)
	}
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: ctx.syms, raw: 1}}
	if got, err := tc.GenerateGrammar(ctx, grammar, 0); got != "" || !errors.Is(err, E_ASSUME) {
		t.Fatalf("GenerateGrammar = %q, %v", got, err)
	}
	ctx = Stub(t, "", E_INVALID_HANDLE, "invalid builder")
	builder := &GrammarBuilder{syms: ctx.syms, raw: 0}
	if err := builder.ResolveImports(ctx, nil); !errors.Is(err, E_INVALID_HANDLE) {
		t.Fatalf("ResolveImports = %v", err)
	}
}

func TestPoolRestore(t *testing.T) {
	ctx := NewContext()
	settings, err := ctx.SettingsNewForProfile("base")
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Database(ctx, ""); err != nil {
		t.Fatal(err)
	}
	run, err := settings.RunStart(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc, err := run.NextTestCase(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := tc.NewPool(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := pool.Add(ctx, tc)
	if err != nil {
		t.Fatal(err)
	}
	consumed, err := pool.Generate(ctx, tc, true)
	if err != nil || consumed != id {
		t.Fatalf("consume = %d, %v", consumed, err)
	}
	if err := pool.Restore(ctx, tc, id); err != nil {
		t.Fatal(err)
	}
	if got, err := pool.Generate(ctx, tc, false); err != nil || got != id {
		t.Fatalf("restored = %d, %v", got, err)
	}
	if err := tc.MarkComplete(ctx, STATUS_VALID, ""); err != nil {
		t.Fatal(err)
	}
}

func TestResultErrorKind(t *testing.T) {
	ctx := Stub(t, RUN_ERROR_HEALTH_CHECK, uint32(HC_TOO_SLOW), OK)
	result := &Result{pointer: &pointer[resultT]{syms: ctx.syms, raw: 1}}
	kind, check, err := result.ErrorKind(ctx)
	if err != nil || kind != RUN_ERROR_HEALTH_CHECK || check != HC_TOO_SLOW {
		t.Fatalf("ErrorKind = %v, %v, %v", kind, check, err)
	}
}
