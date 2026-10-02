package hegel

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

// tempGoProject runs tests in a separate Go process to inspect output, exit
// codes, and testing.T log routing.
type tempGoProject struct {
	t      *testing.T
	dir    string
	expect string // regex; non-empty => expect non-zero exit and match
}

type runOutput struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func newTempGoProject(t *testing.T) *tempGoProject {
	t.Helper()
	moduleDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	p := &tempGoProject{
		t:   t,
		dir: t.TempDir(),
	}
	// Build the temp go.mod by reusing the parent's transitive require lines.
	// This avoids a `go mod tidy` round-trip per project (which would also
	// drop unused requires before main.go exists).
	parentMod, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		t.Fatalf("read parent go.mod: %v", err)
	}
	goMod := regexp.MustCompile(`(?m)^module .*$`).ReplaceAllString(string(parentMod), "module temptest")
	goMod += fmt.Sprintf("\nrequire hegel.dev/go/hegel v0.0.0\n\nreplace hegel.dev/go/hegel => %s\n", moduleDir)
	if err := os.WriteFile(filepath.Join(p.dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	// Reuse the parent module's go.sum so `go test` doesn't need network access
	// to verify the transitive dependency graph.
	if sum, err := os.ReadFile(filepath.Join(moduleDir, "go.sum")); err == nil {
		_ = os.WriteFile(filepath.Join(p.dir, "go.sum"), sum, 0o644)
	}
	return p
}

// writeFile writes name relative to the project dir.
func (p *tempGoProject) writeFile(name, content string) *tempGoProject {
	p.t.Helper()
	if err := os.WriteFile(filepath.Join(p.dir, name), []byte(content), 0o644); err != nil {
		p.t.Fatalf("write %s: %v", name, err)
	}
	return p
}

// expectFailure asserts the command exits non-zero and the combined
// stdout+stderr matches pattern. Without this, a non-zero exit fails the test.
func (p *tempGoProject) expectFailure(pattern string) *tempGoProject {
	p.expect = pattern
	return p
}

func (p *tempGoProject) goTest(args ...string) runOutput {
	p.t.Helper()
	return p.run(append([]string{"test"}, args...))
}

func (p *tempGoProject) run(args []string) runOutput {
	p.t.Helper()
	cmd := exec.Command("go", args...)
	cmd.Dir = p.dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()

	out := runOutput{Stdout: stdout.String(), Stderr: stderr.String()}
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		out.ExitCode = ee.ExitCode()
	} else if err != nil {
		p.t.Fatalf("go %v: spawn failed: %v", args, err)
	}

	if p.expect == "" {
		if err != nil {
			p.t.Fatalf("expected success, got exit %d.\nstdout:\n%s\nstderr:\n%s",
				out.ExitCode, out.Stdout, out.Stderr)
		}
	} else {
		if err == nil {
			p.t.Fatalf("expected failure but command succeeded.\nstdout:\n%s\nstderr:\n%s",
				out.Stdout, out.Stderr)
		}
		combined := out.Stdout + "\n" + out.Stderr
		matched, mErr := regexp.MatchString(p.expect, combined)
		if mErr != nil {
			p.t.Fatalf("bad expectFailure regex %q: %v", p.expect, mErr)
		}
		if !matched {
			p.t.Fatalf("output didn't match %q.\ncombined:\n%s", p.expect, combined)
		}
	}
	return out
}
