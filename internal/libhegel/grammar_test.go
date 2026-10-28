package libhegel

import (
	"errors"
	"runtime"
	"testing"
)

func TestGrammarImports(t *testing.T) {
	ctx := NewContext()
	builder, err := ctx.GrammarBuilderNew("%import dir.custom.word\nstart: word\n")
	if err != nil {
		t.Fatal(err)
	}
	for index, file := range []struct{ path, text string }{
		{"dir/custom.lark", "%import .leaf.WORD\nword: WORD\n"},
		{"dir/leaf.lark", "WORD: \"hello\"\n"},
	} {
		runtime.GC()
		if path, err := builder.File(ctx, uint64(index)); err != nil || path != file.path {
			t.Fatalf("File(%d) = %q, %v; want %q", index, path, err, file.path)
		}
		if err := builder.Provide(ctx, uint64(index), file.text, true); err != nil {
			t.Fatal(err)
		}
	}
	if path, err := builder.File(ctx, 2); path != "" || !errors.Is(err, E_NOT_FOUND) {
		t.Fatalf("File past end = %q, %v", path, err)
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
	ctx := NewContext()
	builder, err := ctx.GrammarBuilderNew("%import common.INT\nstart: INT\n")
	if err != nil {
		t.Fatal(err)
	}
	if path, err := builder.File(ctx, 0); err != nil || path != "common.lark" {
		t.Fatalf("File = %q, %v", path, err)
	}
	if err := builder.Provide(ctx, 0, "", false); err != nil {
		t.Fatal(err)
	}
	if grammar, err := ctx.GrammarNew(builder, "", 0, 0, nil, nil); err != nil || grammar == nil {
		t.Fatalf("GrammarNew = %v, %v", grammar, err)
	}
}

func TestGrammarImportMissingAndEmpty(t *testing.T) {
	ctx := NewContext()
	builder, err := ctx.GrammarBuilderNew("%import .missing.WORD\nstart: WORD\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.Provide(ctx, 0, "", false); !errors.Is(err, E_INVALID_ARG) {
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
	if _, err := builder.File(ctx, 0); !errors.Is(err, E_INVALID_HANDLE) {
		t.Fatalf("File = %v", err)
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

// TestRepeat drives the repeat-loop protocol against the real library: the
// engine draws how many times the body should run, and the loop ends when More
// returns false.
func TestRepeat(t *testing.T) {
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
	if err != nil || tc == nil {
		t.Fatalf("NextTestCase = %v, %v", tc, err)
	}
	rep, err := tc.NewRepeat(ctx)
	if err != nil {
		t.Fatalf("NewRepeat: %v", err)
	}
	for i := 0; ; i++ {
		more, err := rep.More(ctx, tc)
		if err != nil {
			t.Fatalf("Repeat.More: %v", err)
		}
		if !more {
			break
		}
		if i > 1000 {
			t.Fatal("repeat loop did not terminate")
		}
	}
	if err := tc.MarkComplete(ctx, STATUS_VALID, ""); err != nil {
		t.Fatal(err)
	}
}

// TestGrammarDraw drives the grammar-draw protocol against the real library: a
// grammar with two explicit terminals yields a draw whose placeholders the
// caller fills, and the result is the string with the supplied values spliced
// in. Two distinct terminals (not one) let the test assert that Next reports
// each terminal's own index from explicitNames, not merely a constant.
func TestGrammarDraw(t *testing.T) {
	ctx := NewContext()
	builder, err := ctx.GrammarBuilderNew("start: FIRST \"-\" SECOND\nFIRST: /[a-z]+/\nSECOND: /[0-9]+/\n")
	if err != nil {
		t.Fatalf("GrammarBuilderNew: %v", err)
	}
	grammar, err := ctx.GrammarNew(builder, "", 0, 0, nil, []string{"FIRST", "SECOND"})
	if err != nil {
		t.Fatalf("GrammarNew: %v", err)
	}
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
	draw, err := grammar.DrawNew(ctx, 0)
	if err != nil {
		t.Fatalf("DrawNew: %v", err)
	}
	// One value per explicit terminal, indexed by the terminal index Next reports.
	values := []string{"aa", "99"}
	var seq []uint64
	for {
		term, err := draw.Next(ctx, tc)
		if errors.Is(err, E_NOT_FOUND) {
			break // the string is complete
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if term >= uint64(len(values)) {
			t.Fatalf("Next terminal = %d, out of range", term)
		}
		seq = append(seq, term)
		if err := draw.Provide(ctx, values[term]); err != nil {
			t.Fatalf("Provide: %v", err)
		}
	}
	// FIRST precedes SECOND in the string, so Next reports their explicitNames
	// indices in that order.
	if len(seq) != 2 || seq[0] != 0 || seq[1] != 1 {
		t.Fatalf("terminal index sequence = %v, want [0 1]", seq)
	}
	if got, err := draw.Result(ctx); err != nil || got != "aa-99" {
		t.Fatalf("Result = %q, %v; want \"aa-99\"", got, err)
	}
	if err := tc.MarkComplete(ctx, STATUS_VALID, ""); err != nil {
		t.Fatal(err)
	}
}

// TestStubGrammarDrawErrors covers the grammar-draw branches the real-library
// test does not reach: DrawNew's nil-handle path and Result's error path.
func TestStubGrammarDrawErrors(t *testing.T) {
	lib := Stub(t, uintptr(0), E_INVALID_HANDLE, "null grammar") // draw_new handle, result, diagnostic
	grammar := &Grammar{syms: lib.syms, raw: 0}
	if draw, err := grammar.DrawNew(lib, 0); draw != nil || !errors.Is(err, E_INVALID_HANDLE) {
		t.Fatalf("DrawNew = %v, %v", draw, err)
	}

	lib = Stub(t, "", E_NOT_COMPLETE, "not complete") // owned string, result, diagnostic
	draw := &GrammarDraw{syms: lib.syms, raw: 1}
	if got, err := draw.Result(lib); got != "" || !errors.Is(err, E_NOT_COMPLETE) {
		t.Fatalf("Result = %q, %v", got, err)
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

func TestGrammarOptions(t *testing.T) {
	ctx := NewContext()
	builder, err := ctx.GrammarBuilderNew("start: \"a\"\nother: \"b\"\n")
	if err != nil {
		t.Fatal(err)
	}
	grammar, err := ctx.GrammarNew(builder, "other", 1, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
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
	if got, err := tc.GenerateGrammar(ctx, grammar, 1); err != nil || got != "b" {
		t.Fatalf("alternate start = %q, %v", got, err)
	}
	alphabet, err := ctx.StringGeneratorText(1, 1, "utf-8", 0, 0x10ffff, []string{}, nil, "b", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.GrammarNew(builder, "", 0, 0, alphabet, nil); !errors.Is(err, E_INVALID_ARG) {
		t.Fatalf("grammar outside alphabet = %v", err)
	}
	if err := tc.MarkComplete(ctx, STATUS_VALID, ""); err != nil {
		t.Fatal(err)
	}
}
