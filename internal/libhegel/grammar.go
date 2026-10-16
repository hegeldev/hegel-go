package libhegel

import (
	"errors"
	"fmt"
	"runtime"
)

// GrammarBuilder gathers a Lark grammar and the files it imports.
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

// GrammarImport resolves an imported grammar path to UTF-8 source.
// Paths are relative, slash-separated names ending in .lark. Return ok=false
// for a missing file to allow libhegel's bundled-grammar fallback.
type GrammarImport func(path string) (text string, ok bool)

// ResolveImports supplies all imported files through a Go resolver.
//
// A nil resolver uses only libhegel's bundled grammars. Call this before
// compiling a builder whose files have not yet been provided.
//
// Invalid or missing required imports return an error.
func (b *GrammarBuilder) ResolveImports(ctx *Context, resolve GrammarImport) error {
	for index := uint64(0); ; index++ {
		path, err := b.File(ctx, index)
		if errors.Is(err, E_NOT_FOUND) {
			return nil
		}
		if err != nil {
			return err
		}
		var text string
		var present bool
		if resolve != nil {
			text, present = resolve(path)
		}
		if err := b.Provide(ctx, index, text, present); err != nil {
			return fmt.Errorf("import %q: %w", path, err)
		}
	}
}

// Grammar is an immutable compiled Lark grammar.
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
