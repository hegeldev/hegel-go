package hegel

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"runtime"
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

// sourceCache resolves source positions to the binding name receiving a
// draw. Parse results — both successes and errors — are cached, so a missing
// or unparseable file is read at most once per process.
type sourceCache struct {
	mu    sync.RWMutex
	fset  *token.FileSet
	files map[string]cachedFile
	names map[fileLine]string
}

func newSourceCache() *sourceCache {
	return &sourceCache{
		fset:  token.NewFileSet(),
		files: make(map[string]cachedFile),
		names: make(map[fileLine]string),
	}
}

// nameAt returns the binding name receiving the draw at line in file — the
// sole left-hand side of the assignment or declaration there — or "" when no
// single name is receiving it (a blank identifier, a multi-value assignment,
// a Draw nested inside a larger expression, or unparseable source). A ""
// name falls back to draw_N numbering at the report site.
func (c *sourceCache) nameAt(file string, line int) (string, error) {
	key := fileLine{file: file, line: line}

	c.mu.RLock()
	name, ok := c.names[key]
	c.mu.RUnlock()
	if ok {
		return name, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.extractLocked(key)
}

// extractLocked re-checks the cache (a racing goroutine may have populated it
// between nameAt's RLock and Lock) and otherwise parses the source and
// records the binding name of the innermost statement covering the target
// line. Caller must hold c.mu for writing.
func (c *sourceCache) extractLocked(key fileLine) (string, error) {
	if name, ok := c.names[key]; ok {
		return name, nil
	}

	f, err := c.loadLocked(key.file)
	if err != nil {
		return "", err
	}

	var best ast.Stmt
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
		// granularity so the whole assignment is considered, not
		// sub-expressions. Blocks don't count: a closure argument's body
		// opening on the binding's own line must not shadow the binding.
		if stmt, isStmt := n.(ast.Stmt); isStmt {
			if _, isBlock := n.(*ast.BlockStmt); !isBlock {
				best = stmt
			}
		}
		return true
	})
	name := bindingName(best)
	c.names[key] = name
	return name, nil
}

// bindingName extracts the single identifier a statement binds: the left-hand
// side of `x := …` / `x = …` or the name of `var x = …`. Returns "" for
// anything else — a blank identifier, several names, or no binding at all.
func bindingName(stmt ast.Stmt) string {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
			return ""
		}
		if ident, ok := s.Lhs[0].(*ast.Ident); ok && ident.Name != "_" {
			return ident.Name
		}
	case *ast.DeclStmt:
		decl, ok := s.Decl.(*ast.GenDecl)
		if !ok || len(decl.Specs) != 1 {
			return ""
		}
		spec, ok := decl.Specs[0].(*ast.ValueSpec)
		if ok && len(spec.Names) == 1 && len(spec.Values) == 1 && spec.Names[0].Name != "_" {
			return spec.Names[0].Name
		}
	}
	return ""
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

// callerFileLine resolves runtime.Caller relative to the calling function:
// skip = 0 names the caller itself, 1 its caller, and so on.
func callerFileLine(skip int) (file string, line int) {
	_, file, line, ok := runtime.Caller(skip + 1)
	if !ok { // coverage-ignore
		panic(fmt.Errorf("runtime.Caller(%d) failed", skip+1))
	}
	return file, line
}

// drawBindingName resolves the caller's source position and returns the
// binding name receiving the draw ("" when there is no unambiguous single
// name). skip is the number of frames above this one to skip (Draw passes 1
// to point at the user's call site).
func drawBindingName(skip int) string {
	file, line := callerFileLine(skip + 1)
	name, _ := drawReportSource.nameAt(file, line)
	return name
}
