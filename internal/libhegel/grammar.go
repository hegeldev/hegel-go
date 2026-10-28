package libhegel

import "runtime"

// GrammarBuilder gathers a Lark grammar and the files it imports.
//
// Builders are released automatically via the GC.
type GrammarBuilder pointer[grammarBuilderT]

// GrammarBuilderNew parses a Lark grammar and starts gathering its imports.
//
// text must contain UTF-8 grammar source.
//
// Invalid source returns a nil builder and an error.
func (c *Context) GrammarBuilderNew(text string) (*GrammarBuilder, error) {
	ptr, err := allocate(c, "hegel_grammar_builder_new", func(ctx ctxT, raw *grammarBuilderT) Error {
		return c.syms.GrammarBuilderNew(ctx, cString(text), raw)
	}, c.syms.GrammarBuilderFree)
	if ptr == nil {
		return nil, err
	}
	return (*GrammarBuilder)(ptr), nil
}

// File returns the path of an imported grammar file.
//
// index counts from zero; providing files may add more paths to the list.
//
// E_NOT_FOUND signals the end of the current list.
func (b *GrammarBuilder) File(ctx *Context, index uint64) (string, error) {
	var path stringT
	err := ctx.invoke("hegel_grammar_builder_file", func(ctx ctxT) Error {
		e := b.syms.GrammarBuilderFile(ctx, b.raw, index, &path)
		runtime.KeepAlive(b)
		return e
	})
	if err != nil {
		return "", err
	}
	value := goString(path)
	runtime.KeepAlive(b)
	return value, nil
}

// Provide supplies the contents of an imported grammar file.
//
// index identifies a path from File; present distinguishes an existing empty
// file from a missing file, which may fall back to a bundled grammar.
//
// Invalid source or an unavailable required import returns an error.
func (b *GrammarBuilder) Provide(ctx *Context, index uint64, text string, present bool) error {
	var view *stringT
	if present {
		view = stringPointer(text)
	}
	return ctx.invoke("hegel_grammar_builder_provide", func(ctx ctxT) Error {
		e := b.syms.GrammarBuilderProvide(ctx, b.raw, index, view)
		runtime.KeepAlive(b)
		return e
	})
}

// Grammar is an immutable compiled Lark grammar.
//
// It may be shared across test cases and goroutines and is released by the GC.
type Grammar pointer[grammarT]

// GrammarNew compiles a builder for TestCase.GenerateGrammar.
//
// All imports must be provided first. Empty start selects the start rule;
// zero maxSize and maxDepth select libhegel's defaults. alphabet optionally
// constrains characters, and explicitNames names placeholder terminals.
//
// Invalid grammar options return a nil grammar and an error.
func (c *Context) GrammarNew(builder *GrammarBuilder, start string, maxSize uint64, maxDepth uint32, alphabet *StringGenerator, explicitNames []string) (*Grammar, error) {
	var startPtr *stringT
	if start != "" {
		startPtr = stringPointer(start)
	}
	names, count := cStringArrayArg(explicitNames)
	var alphabetRaw stringGenT
	if alphabet != nil {
		alphabetRaw = alphabet.raw
	}
	ptr, err := allocate(c, "hegel_grammar_new", func(ctx ctxT, raw *grammarT) Error {
		e := c.syms.GrammarNew(ctx, builder.raw, startPtr, maxSize, maxDepth, alphabetRaw, names, count, raw)
		runtime.KeepAlive(builder)
		runtime.KeepAlive(alphabet)
		return e
	}, c.syms.GrammarFree)
	if ptr == nil {
		return nil, err
	}
	return (*Grammar)(ptr), nil
}

// Placeholder returns the marker for an explicit terminal.
//
// name must be one of the grammar's explicit terminals.
//
// An unknown terminal returns an error.
func (g *Grammar) Placeholder(ctx *Context, name string) (string, error) {
	var marker stringT
	err := ctx.invoke("hegel_grammar_placeholder", func(ctx ctxT) Error {
		e := g.syms.GrammarPlaceholder(ctx, g.raw, cString(name), &marker)
		runtime.KeepAlive(g)
		return e
	})
	if err != nil {
		return "", err
	}
	value := goString(marker)
	runtime.KeepAlive(g)
	return value, nil
}

// GenerateGrammar draws a string accepted by a compiled grammar.
//
// maxSize caps the number of characters, or selects the grammar's cap when zero.
// Explicit terminals appear as their placeholder markers.
//
// An unsuccessful draw returns an error, including E_ASSUME or E_STOP_TEST.
func (tc *TestCase) GenerateGrammar(ctx *Context, grammar *Grammar, maxSize uint64) (string, error) {
	err := ctx.invoke("hegel_generate_grammar", func(ctx ctxT) Error {
		e := tc.syms.GenerateGrammar(ctx, tc.raw, grammar.raw, maxSize, &tc.outStringRes)
		runtime.KeepAlive(tc)
		runtime.KeepAlive(grammar)
		return e
	})
	if err != nil {
		return "", err
	}
	value := goString(stringT(tc.outStringRes))
	_ = tc.syms.OwnedStringFree(0, &tc.outStringRes)
	return value, nil
}

// GrammarDraw is one draw of a grammar string whose explicit terminals take
// caller-supplied values.
//
// Driven by Next / Provide and read with Result; released automatically via the
// GC.
type GrammarDraw pointer[grammarDrawT]

// DrawNew starts a draw of this grammar whose explicit terminals take
// caller-supplied values.
//
// maxSize caps the characters (the supplied values included), or selects the
// grammar's cap when zero. Nothing is drawn until GrammarDraw.Next. The
// returned handle is owned by the caller and freed automatically via the GC.
//
// A nil grammar returns a nil draw and an error.
func (g *Grammar) DrawNew(ctx *Context, maxSize uint64) (*GrammarDraw, error) {
	ptr, err := allocate(ctx, "hegel_grammar_draw_new", func(ctx ctxT, raw *grammarDrawT) Error {
		e := g.syms.GrammarDrawNew(ctx, g.raw, maxSize, raw)
		runtime.KeepAlive(g)
		return e
	}, g.syms.GrammarDrawFree)
	if ptr == nil {
		return nil, err
	}
	return (*GrammarDraw)(ptr), nil
}

// Next advances the draw to the next explicit terminal that needs a value,
// drawing the string from tc when an attempt starts. tc must be the test case
// every earlier call on this draw was given.
//
// It returns the index, in the explicitNames given to Context.GrammarNew, of
// the terminal whose value is due; supply it with Provide before calling Next
// again. E_NOT_FOUND signals that no terminal is left and the string is
// complete (read it with Result); E_ASSUME means the test case was concluded
// invalid.
func (d *GrammarDraw) Next(ctx *Context, tc *TestCase) (uint64, error) {
	var terminal uint64
	err := ctx.invoke("hegel_grammar_draw_next", func(ctx ctxT) Error {
		e := d.syms.GrammarDrawNext(ctx, tc.raw, d.raw, &terminal)
		runtime.KeepAlive(tc)
		runtime.KeepAlive(d)
		return e
	})
	return terminal, err
}

// Provide supplies the value of the explicit terminal Next asked for. The value
// is copied and must be UTF-8.
//
// A value that is not valid UTF-8, or a call when no value is due, returns an error.
func (d *GrammarDraw) Provide(ctx *Context, value string) error {
	return ctx.invoke("hegel_grammar_draw_provide", func(ctx ctxT) Error {
		e := d.syms.GrammarDrawProvide(ctx, d.raw, cString(value))
		runtime.KeepAlive(d)
		return e
	})
}

// Result returns the string the draw produced, with each explicit terminal's
// placeholder marker replaced by the value supplied for it.
//
// Call it once Next has returned E_NOT_FOUND. A draw that is not complete yet
// returns an error wrapping E_NOT_COMPLETE.
func (d *GrammarDraw) Result(ctx *Context) (string, error) {
	var res ownedStringT
	err := ctx.invoke("hegel_grammar_draw_result", func(ctx ctxT) Error {
		e := d.syms.GrammarDrawResult(ctx, d.raw, &res)
		runtime.KeepAlive(d)
		return e
	})
	if err != nil {
		return "", err
	}
	value := goString(stringT(res))
	_ = d.syms.OwnedStringFree(0, &res)
	runtime.KeepAlive(d)
	return value, nil
}
