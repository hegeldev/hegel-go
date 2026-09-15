package libhegel

import (
	"runtime"
	"unsafe"
)

// PrinterOptions configures a printer's layout and is released automatically by the GC.
type PrinterOptions struct {
	pointer[printerOptionsT]
}

// Printer owns a reference to a document region, released automatically by the GC.
// Resolve or Value seals a document; subsequent writes fail.
type Printer struct {
	*pointer[printerT]
	outBool  bool
	outValue stringResult
}

// PrinterOptionsNew creates options with the engine's default line width (79).
func (ctx *Context) PrinterOptionsNew() (*PrinterOptions, error) {
	opts := new(PrinterOptions)
	ok, err := allocateInto(ctx, &opts.pointer, "hegel_printer_options_new", func(rawCtx ctxT, raw *printerOptionsT) Error {
		return ctx.syms.PrinterOptionsNew(rawCtx, raw)
	}, ctx.syms.PrinterOptionsFree)
	if !ok {
		return nil, err
	}
	return opts, err
}

// MaxWidth sets the document's line width, which must be positive.
func (o *PrinterOptions) MaxWidth(ctx *Context, width uint64) error {
	return ctx.invoke("hegel_printer_options_set_max_width", func(ctx ctxT) Error {
		e := o.syms.PrinterOptionsSetMaxWidth(ctx, o.raw, width)
		runtime.KeepAlive(o)
		return e
	})
}

// printerOptionsRaw maps nil options to the engine defaults.
func printerOptionsRaw(options *PrinterOptions) printerOptionsT {
	if options == nil {
		return 0
	}
	return options.raw
}

// PrinterNew creates a standalone document, using default options when nil.
func (ctx *Context) PrinterNew(options *PrinterOptions) (*Printer, error) {
	ptr, err := allocate(ctx, "hegel_printer_new", func(rawCtx ctxT, raw *printerT) Error {
		e := ctx.syms.PrinterNew(rawCtx, printerOptionsRaw(options), raw)
		runtime.KeepAlive(options)
		return e
	}, ctx.syms.PrinterFree)
	if ptr == nil {
		return nil, err
	}
	return &Printer{pointer: ptr}, err
}

// Printer returns this test-case handle's print region, using default options when nil.
func (tc *TestCase) Printer(ctx *Context, options *PrinterOptions) (*Printer, error) {
	ptr, err := allocate(ctx, "hegel_test_case_printer", func(ctx ctxT, raw *printerT) Error {
		e := tc.syms.TestCasePrinter(ctx, tc.raw, printerOptionsRaw(options), raw)
		runtime.KeepAlive(tc)
		runtime.KeepAlive(options)
		return e
	}, tc.syms.PrinterFree)
	if ptr == nil {
		return nil, err
	}
	return &Printer{pointer: ptr}, err
}

// Deferred opens a hole whose later content is spliced in when the root is resolved.
func (p *Printer) Deferred(ctx *Context) (*Printer, error) {
	ptr, err := allocate(ctx, "hegel_printer_deferred", func(ctx ctxT, raw *printerT) Error {
		e := p.syms.PrinterDeferred(ctx, p.raw, raw)
		runtime.KeepAlive(p)
		return e
	}, p.syms.PrinterFree)
	if ptr == nil {
		return nil, err
	}
	return &Printer{pointer: ptr}, err
}

// IsLive reports whether this region still accepts writes.
func (p *Printer) IsLive(ctx *Context) (bool, error) {
	err := ctx.invoke("hegel_printer_is_live", func(ctx ctxT) Error {
		e := p.syms.PrinterIsLive(ctx, p.raw, &p.outBool)
		runtime.KeepAlive(p)
		return e
	})
	return p.outBool, err
}

// Value seals the root document and copies its length-delimited UTF-8 output.
// Resolve outstanding deferred sessions first. Repeated reads return the same text.
func (p *Printer) Value(ctx *Context) (string, error) {
	err := ctx.invoke("hegel_printer_value", func(ctx ctxT) Error {
		e := p.syms.PrinterValue(ctx, p.raw, &p.outValue)
		runtime.KeepAlive(p)
		return e
	})
	if err != nil {
		return "", err
	}
	value := string(unsafe.Slice(p.outValue.data, p.outValue.len))
	_ = p.syms.PrinterValueFree(0, &p.outValue)
	return value, nil
}

// Text emits literal UTF-8 text without newlines.
func (p *Printer) Text(ctx *Context, text string) error {
	data, n := cString(&text)
	return ctx.invoke("hegel_printer_text", func(ctx ctxT) Error {
		e := p.syms.PrinterText(ctx, p.raw, data, n)
		runtime.KeepAlive(p)
		return e
	})
}

// IfBreak emits text only when the enclosing group breaks.
func (p *Printer) IfBreak(ctx *Context, text string) error {
	data, n := cString(&text)
	return ctx.invoke("hegel_printer_if_break", func(ctx ctxT) Error {
		e := p.syms.PrinterIfBreak(ctx, p.raw, data, n)
		runtime.KeepAlive(p)
		return e
	})
}

// Breakable emits a separator or an indented newline when the group breaks.
func (p *Printer) Breakable(ctx *Context, text string) error {
	data, n := cString(&text)
	return ctx.invoke("hegel_printer_breakable", func(ctx ctxT) Error {
		e := p.syms.PrinterBreakable(ctx, p.raw, data, n)
		runtime.KeepAlive(p)
		return e
	})
}

// Comment appends a line comment and forces the enclosing groups to break.
func (p *Printer) Comment(ctx *Context, text string) error {
	data, n := cString(&text)
	return ctx.invoke("hegel_printer_comment", func(ctx ctxT) Error {
		e := p.syms.PrinterComment(ctx, p.raw, data, n)
		runtime.KeepAlive(p)
		return e
	})
}

// EndGroup closes the innermost group and emits its closing text.
func (p *Printer) EndGroup(ctx *Context, text string) error {
	data, n := cString(&text)
	return ctx.invoke("hegel_printer_end_group", func(ctx ctxT) Error {
		e := p.syms.PrinterEndGroup(ctx, p.raw, data, n)
		runtime.KeepAlive(p)
		return e
	})
}

// BeginGroup opens a group and increases indentation by indent.
func (p *Printer) BeginGroup(ctx *Context, indent uint64, text string) error {
	data, n := cString(&text)
	return ctx.invoke("hegel_printer_begin_group", func(ctx ctxT) Error {
		e := p.syms.PrinterBeginGroup(ctx, p.raw, indent, data, n)
		runtime.KeepAlive(p)
		return e
	})
}

// HardBreak emits an unconditional indented newline.
func (p *Printer) HardBreak(ctx *Context) error {
	return ctx.invoke("hegel_printer_hard_break", func(ctx ctxT) Error {
		e := p.syms.PrinterHardBreak(ctx, p.raw)
		runtime.KeepAlive(p)
		return e
	})
}

// BeginSpeculative buffers subsequent writes until committed or aborted.
func (p *Printer) BeginSpeculative(ctx *Context) error {
	return ctx.invoke("hegel_printer_begin_speculative", func(ctx ctxT) Error {
		e := p.syms.PrinterBeginSpeculative(ctx, p.raw)
		runtime.KeepAlive(p)
		return e
	})
}

// CommitSpeculative keeps the innermost speculative region.
func (p *Printer) CommitSpeculative(ctx *Context) error {
	return ctx.invoke("hegel_printer_commit_speculative", func(ctx ctxT) Error {
		e := p.syms.PrinterCommitSpeculative(ctx, p.raw)
		runtime.KeepAlive(p)
		return e
	})
}

// AbortSpeculative discards the innermost speculative region.
func (p *Printer) AbortSpeculative(ctx *Context) error {
	return ctx.invoke("hegel_printer_abort_speculative", func(ctx ctxT) Error {
		e := p.syms.PrinterAbortSpeculative(ctx, p.raw)
		runtime.KeepAlive(p)
		return e
	})
}

// Resolve splices deferred content into the root and seals the document.
func (p *Printer) Resolve(ctx *Context) error {
	return ctx.invoke("hegel_printer_resolve", func(ctx ctxT) Error {
		e := p.syms.PrinterResolve(ctx, p.raw)
		runtime.KeepAlive(p)
		return e
	})
}

// ShiftIndent adjusts indentation by a signed delta.
func (p *Printer) ShiftIndent(ctx *Context, delta int64) error {
	return ctx.invoke("hegel_printer_shift_indent", func(ctx ctxT) Error {
		e := p.syms.PrinterShiftIndent(ctx, p.raw, delta)
		runtime.KeepAlive(p)
		return e
	})
}
