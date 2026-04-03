package hegel

import (
	"fmt"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const docTestHeader = `package doccheck_test

import (
	"fmt"
	"math"
	"testing"

	"hegel.dev/go/hegel"
)

var _ = fmt.Sprintf
var _ = math.MinInt

`

var markdownGoBlock = regexp.MustCompile("(?ms)^```go\n(.*?)^```")

func TestDocExamplesCompile(t *testing.T) {
	moduleDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	var files []string

	// README.md — full-file ```go blocks
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, match := range markdownGoBlock.FindAllSubmatch(readme, -1) {
		block := strings.TrimSpace(string(match[1]))
		if strings.Contains(block, "package ") {
			patched := regexp.MustCompile(`(?m)^package \S+`).ReplaceAllString(block, "package doccheck_test")
			files = append(files, patched+"\n")
		}
	}

	// hegel.go — code blocks from package doc comment
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "hegel.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	var p comment.Parser
	doc := p.Parse(f.Doc.Text())
	for _, block := range doc.Content {
		code, ok := block.(*comment.Code)
		if !ok {
			continue
		}
		text := strings.TrimSpace(code.Text)
		if strings.HasPrefix(text, "func Test") {
			files = append(files, docTestHeader+text+"\n")
		}
	}

	if len(files) == 0 {
		t.Fatal("no compilable code examples found")
	}

	// Write to temp dir and compile
	tmpdir := t.TempDir()

	goMod := fmt.Sprintf(
		"module doccheck\n\ngo 1.23\n\nrequire hegel.dev/go/hegel v0.0.0\n\nreplace hegel.dev/go/hegel => %s\n",
		moduleDir,
	)
	os.WriteFile(filepath.Join(tmpdir, "go.mod"), []byte(goMod), 0o644)

	for i, content := range files {
		os.WriteFile(filepath.Join(tmpdir, fmt.Sprintf("ex%d_test.go", i)), []byte(content), 0o644)
	}

	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = tmpdir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed:\n%s", out)
	}

	cmd = exec.Command("go", "test", "-c", "-o", os.DevNull)
	cmd.Dir = tmpdir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("doc examples failed to compile:\n%s", out)
	}

	cmd = exec.Command("go", "vet", "./...")
	cmd.Dir = tmpdir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go vet failed on doc examples:\n%s", out)
	}

	for i, content := range files {
		filename := filepath.Join(tmpdir, fmt.Sprintf("ex%d_test.go", i))
		cmd = exec.Command("gofmt", "-l", filename)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("gofmt failed on doc example %d:\n%s", i, out)
		} else if len(strings.TrimSpace(string(out))) > 0 {
			formatted, _ := exec.Command("gofmt", filename).CombinedOutput()
			t.Errorf("doc example %d is not gofmt-formatted.\nOriginal:\n%s\nFormatted:\n%s", i, content, formatted)
		}
	}
}
