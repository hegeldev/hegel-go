package hegel

import (
	"fmt"
	"go/scanner"
	"go/token"
	"iter"
	"strings"

	"hegel.dev/go/hegel/internal/libhegel"
)

type goSyntaxToken struct {
	offset int
	token  token.Token
}

// printGoValue prints value in Go syntax with breakable comma spaces and
// independently nested delimiter groups.
func printGoValue(ctx *libhegel.Context, printer *libhegel.Printer, value any) error {
	return printGoSyntax(ctx, printer, fmt.Sprintf("%#v", value))
}

func printGoSyntax(ctx *libhegel.Context, printer *libhegel.Printer, source string) error {
	tokens, ok := scanGoSyntax(source)
	if !ok {
		return printText(ctx, printer, source)
	}

	cursor := 0
	trimSpace := false
	for item := range tokens {
		text := source[cursor:item.offset]
		if trimSpace {
			text = strings.TrimLeft(text, " \t")
			trimSpace = false
		}
		if err := printText(ctx, printer, text); err != nil {
			return err
		}

		delimiter := source[item.offset : item.offset+1]
		switch item.token {
		case token.LPAREN, token.LBRACK, token.LBRACE:
			if err := printer.BeginGroup(ctx, 1, delimiter); err != nil {
				return err
			}
		case token.RPAREN, token.RBRACK, token.RBRACE:
			if err := printer.EndGroup(ctx, delimiter); err != nil {
				return err
			}
		case token.COMMA:
			if err := printer.Text(ctx, delimiter); err != nil {
				return err
			}
			if !startsWithLineBreak(source[item.offset+1:]) {
				if err := printer.Breakable(ctx, " "); err != nil {
					return err
				}
			}
			trimSpace = true
		}
		cursor = item.offset + 1
	}

	return printText(ctx, printer, source[cursor:])
}

func startsWithLineBreak(text string) bool {
	text = strings.TrimLeft(text, " \t")
	return strings.HasPrefix(text, "\n") || strings.HasPrefix(text, "\r\n")
}

// scanGoSyntax validates the full input before yielding tokens so malformed
// GoString output can be printed literally.
func scanGoSyntax(source string) (iter.Seq[goSyntaxToken], bool) {
	if !walkGoSyntax(source, func(goSyntaxToken) bool { return true }) {
		return nil, false
	}
	return func(yield func(goSyntaxToken) bool) {
		walkGoSyntax(source, yield)
	}, true
}

func walkGoSyntax(source string, yield func(goSyntaxToken) bool) bool {
	fset := token.NewFileSet()
	file := fset.AddFile("", -1, len(source))
	valid := true
	var lexer scanner.Scanner
	lexer.Init(file, []byte(source), func(token.Position, string) {
		valid = false
	}, 0)

	var stack []token.Token
	for {
		pos, tok, _ := lexer.Scan()
		if tok == token.EOF {
			break
		}
		switch tok {
		case token.LPAREN, token.LBRACK, token.LBRACE:
			stack = append(stack, tok)
			if !yield(goSyntaxToken{offset: file.Offset(pos), token: tok}) {
				return true
			}
		case token.RPAREN, token.RBRACK, token.RBRACE:
			if len(stack) == 0 || !matchingDelimiters(stack[len(stack)-1], tok) {
				return false
			}
			stack = stack[:len(stack)-1]
			if !yield(goSyntaxToken{offset: file.Offset(pos), token: tok}) {
				return true
			}
		case token.COMMA:
			if len(stack) > 0 && !yield(goSyntaxToken{offset: file.Offset(pos), token: tok}) {
				return true
			}
		}
	}
	return valid && len(stack) == 0
}

func matchingDelimiters(open, close token.Token) bool {
	return open == token.LPAREN && close == token.RPAREN ||
		open == token.LBRACK && close == token.RBRACK ||
		open == token.LBRACE && close == token.RBRACE
}

func printText(ctx *libhegel.Context, printer *libhegel.Printer, text string) error {
	first := true
	for line := range strings.SplitSeq(text, "\n") {
		if !first {
			if err := printer.HardBreak(ctx); err != nil {
				return err
			}
		}
		first = false
		if line != "" {
			if err := printer.Text(ctx, line); err != nil {
				return err
			}
		}
	}
	return nil
}
