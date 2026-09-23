package hegel

import (
	"errors"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

func renderGoSyntax(t *testing.T, width uint64, source string) string {
	t.Helper()
	ctx := libhegel.NewContext()
	options, err := ctx.PrinterOptionsNew()
	if err != nil {
		t.Fatal(err)
	}
	if err := options.MaxWidth(ctx, width); err != nil {
		t.Fatal(err)
	}
	printer, err := ctx.PrinterNew(options)
	if err != nil {
		t.Fatal(err)
	}
	if err := printGoSyntax(ctx, printer, source); err != nil {
		t.Fatal(err)
	}
	got, err := printer.Value(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestPrintGoSyntaxFitsOnOneLine(t *testing.T) {
	t.Parallel()
	source := `Record{Name:"first", Values:[]int{1, 2}}`
	if got := renderGoSyntax(t, 79, source); got != source {
		t.Fatalf("rendered value = %q, want %q", got, source)
	}
}

func TestPrintGoSyntaxUsesNestedGroups(t *testing.T) {
	t.Parallel()
	source := `Record{Name:"first", Values:[]int{1, 2}, Enabled:true}`
	got := renderGoSyntax(t, 45, source)
	if !strings.Contains(got, "\n") {
		t.Fatalf("rendered value did not wrap: %q", got)
	}
	if !strings.Contains(got, "Values:[]int{1, 2}") {
		t.Fatalf("nested slice should remain on one line: %q", got)
	}
}

func TestPrintGoSyntaxIgnoresLiteralAndCommentCommas(t *testing.T) {
	t.Parallel()
	source := "Thing{Text:`raw,a`, Rune:',', Quoted:\"x,y\" /* z,w */}"
	got := renderGoSyntax(t, 1, source)
	for _, literal := range []string{"`raw,a`", "','", `"x,y"`, "/* z,w */"} {
		if !strings.Contains(got, literal) {
			t.Fatalf("rendered value lost %q: %q", literal, got)
		}
	}
	if strings.Count(got, "\n") != 2 {
		t.Fatalf("only the two field-separating commas should break: %q", got)
	}
}

func TestPrintGoSyntaxMalformedInputFallsBack(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`Thing{Text:"unterminated}`, `(]`} {
		if got := renderGoSyntax(t, 1, source); got != source {
			t.Fatalf("rendered value = %q, want literal fallback %q", got, source)
		}
	}
}

func TestPrintGoSyntaxLeavesTopLevelCommaLiteral(t *testing.T) {
	t.Parallel()
	source := "first, second"
	if got := renderGoSyntax(t, 1, source); got != source {
		t.Fatalf("rendered value = %q, want %q", got, source)
	}
}

func TestPrintGoSyntaxPreservesExistingCommaBreak(t *testing.T) {
	t.Parallel()
	source := "Thing{First:1,\nSecond:2}"
	got := renderGoSyntax(t, 1, source)
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("existing newline should not gain a preceding break: %q", got)
	}
}

func TestPrintGoSyntaxPropagatesPrinterErrors(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		returns []any
	}{
		{name: "fallback text", source: `(`, returns: []any{libhegel.E_BACKEND, "boom"}},
		{name: "text before token", source: `x{}`, returns: []any{libhegel.E_BACKEND, "boom"}},
		{name: "begin group", source: `{}`, returns: []any{libhegel.E_BACKEND, "boom"}},
		{name: "end group", source: `{}`, returns: []any{libhegel.OK, libhegel.E_BACKEND, "boom"}},
		{name: "comma", source: `{,}`, returns: []any{libhegel.OK, libhegel.E_BACKEND, "boom"}},
		{name: "breakable", source: `{,}`, returns: []any{libhegel.OK, libhegel.OK, libhegel.E_BACKEND, "boom"}},
		{name: "hard break", source: "\n", returns: []any{libhegel.E_BACKEND, "boom"}},
		{name: "text", source: `x`, returns: []any{libhegel.E_BACKEND, "boom"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returns := append([]any{uintptr(1), libhegel.OK}, tt.returns...)
			ctx := libhegel.Stub(t, returns...)
			printer, err := ctx.PrinterNew(nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := printGoSyntax(ctx, printer, tt.source); !errors.Is(err, libhegel.E_BACKEND) {
				t.Fatalf("printGoSyntax error = %v, want E_BACKEND", err)
			}
		})
	}
}
