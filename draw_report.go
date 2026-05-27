package hegel

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var drawReportSource = newSourceCache()

type fileLine struct {
	file string
	line int
}

type cachedFile struct {
	file *ast.File
	err  error
}

// sourceCache resolves source positions to printed-statement text. Parse
// results — both successes and errors — are cached, so a missing or
// unparseable file is read at most once per process.
type sourceCache struct {
	mu         sync.RWMutex
	fset       *token.FileSet
	files      map[string]cachedFile
	statements map[fileLine]string
}

func newSourceCache() *sourceCache {
	return &sourceCache{
		fset:       token.NewFileSet(),
		files:      make(map[string]cachedFile),
		statements: make(map[fileLine]string),
	}
}

// statementAt returns the source text of the smallest AST statement whose
// position range covers line in file. Returns "" with err == nil when the
// file parsed but no statement matches; returns "" with err != nil when the
// file could not be parsed.
func (c *sourceCache) statementAt(file string, line int) (string, error) {
	key := fileLine{file: file, line: line}

	c.mu.RLock()
	stmt, ok := c.statements[key]
	c.mu.RUnlock()
	if ok {
		return stmt, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.extractLocked(key)
}

// extractLocked re-checks the cache (a racing goroutine may have populated it
// between statementAt's RLock and Lock) and otherwise parses the source and
// records the smallest statement covering the target line. Caller must hold
// c.mu for writing.
func (c *sourceCache) extractLocked(key fileLine) (string, error) {
	if stmt, ok := c.statements[key]; ok {
		return stmt, nil
	}

	f, err := c.loadLocked(key.file)
	if err != nil {
		return "", err
	}

	stmt := ""
	var best ast.Node
	tokFile := c.fset.File(f.FileStart)
	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		pos := tokFile.Line(n.Pos())
		end := tokFile.Line(n.End())
		if pos > key.line || end < key.line {
			return false
		}
		// Prefer the innermost enclosing statement, but stop at expression
		// granularity so we report whole assignments, not sub-expressions.
		if _, isStmt := n.(ast.Stmt); isStmt {
			best = n
		}
		return true
	})
	if best != nil {
		var buf strings.Builder
		cfg := printer.Config{Mode: printer.UseSpaces, Tabwidth: 4}
		if perr := cfg.Fprint(&buf, c.fset, best); perr == nil {
			stmt = buf.String()
		}
	}
	c.statements[key] = stmt
	return stmt, nil
}

// loadLocked returns the parsed AST for file, parsing it on first request.
//
// Errors are cached. Caller must hold c.mu for writing.
func (c *sourceCache) loadLocked(file string) (*ast.File, error) {
	if f, ok := c.files[file]; ok {
		return f.file, f.err
	}
	f, err := parser.ParseFile(c.fset, file, nil, parser.SkipObjectResolution)
	c.files[file] = cachedFile{f, err}
	return f, err
}

// formatDrawReport resolves the caller's source position via runtime.Caller
// and returns the file:line location and a printed "statement = value" line
// for the originating Draw call. skip is the number of frames above this one
// to skip (Draw passes 1 to point at the user's call site).
func formatDrawReport(skip int, value any) (location, statement string) {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok { // coverage-ignore
		panic(fmt.Errorf("runtime.Caller(%d) failed", skip+1))
	}
	stmt, _ := drawReportSource.statementAt(file, line)
	location, stmt = formatDrawLine(file, line, stmt, value)
	return location, stmt
}

func formatDrawLine(file string, line int, stmt string, value any) (location, statement string) {
	if stmt == "" {
		stmt = fmt.Sprintf("hegel.Draw[%T](...) = %#v", value, value)
	} else {
		stmt = fmt.Sprintf("%s = %#v", stmt, value)
	}
	return fmt.Sprintf("%s:%d", filepath.Base(file), line), stmt
}
