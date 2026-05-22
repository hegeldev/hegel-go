package hegel

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempSource(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "src.go")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil { // coverage-ignore
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

const happyPathSource = `package x

func f() int {
	return 42
}
`

func TestSourceCacheStatementAt(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, happyPathSource)
	c := newSourceCache()

	got, err := c.statementAt(path, 4)
	if err != nil {
		t.Fatalf("statementAt: unexpected err %v", err)
	}
	if !strings.Contains(got, "return 42") {
		t.Fatalf("statementAt: got %q, want substring %q", got, "return 42")
	}
}

func TestSourceCacheMissingFile(t *testing.T) {
	t.Parallel()
	c := newSourceCache()
	missing := filepath.Join(t.TempDir(), "nope.go")

	got, err := c.statementAt(missing, 1)
	if err == nil {
		t.Fatalf("statementAt(missing): expected err, got nil")
	}
	if got != "" {
		t.Fatalf("statementAt(missing): got %q, want empty", got)
	}
	// Failures are cached so we don't re-read a file we already know is
	// broken. The file entry is recorded; no statement is recorded.
	if len(c.files) != 1 || len(c.statements) != 0 {
		t.Fatalf("unexpected cache after failure: files=%d statements=%d", len(c.files), len(c.statements))
	}
	// Repeat the lookup; the second call hits the file cache and returns
	// the same error without re-parsing.
	got2, err2 := c.statementAt(missing, 1)
	if err2 == nil {
		t.Fatal("statementAt(missing) repeat: expected err, got nil")
	}
	if got2 != "" {
		t.Fatalf("statementAt(missing) repeat: got %q, want empty", got2)
	}
}

func TestSourceCacheParseError(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, "this is not go\n")
	c := newSourceCache()

	got, err := c.statementAt(path, 1)
	if err == nil {
		t.Fatalf("statementAt(invalid): expected err, got nil")
	}
	if got != "" {
		t.Fatalf("statementAt(invalid): got %q, want empty", got)
	}
}

func TestSourceCacheLineOutOfRange(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, happyPathSource)
	c := newSourceCache()

	got, err := c.statementAt(path, 999)
	if err != nil {
		t.Fatalf("statementAt(line=999): unexpected err %v", err)
	}
	if got != "" {
		t.Fatalf("statementAt(line=999): got %q, want empty", got)
	}
}

func TestSourceCacheStatementHit(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, happyPathSource)
	c := newSourceCache()

	first, err := c.statementAt(path, 4)
	if err != nil || first == "" {
		t.Fatalf("first lookup: %q err=%v", first, err)
	}

	// Delete the file; the second call with the same key must hit the
	// statement cache without touching disk.
	if err := os.Remove(path); err != nil { // coverage-ignore
		t.Fatalf("remove: %v", err)
	}
	second, err := c.statementAt(path, 4)
	if err != nil {
		t.Fatalf("second lookup: unexpected err %v", err)
	}
	if second != first {
		t.Fatalf("cache miss after delete: got %q, want %q", second, first)
	}
}

func TestSourceCacheExtractLockedRevalidates(t *testing.T) {
	t.Parallel()
	c := newSourceCache()
	key := fileLine{file: "preempted.go", line: 1}
	// Simulate another goroutine populating the statement cache between
	// statementAt's read-lock miss and the write-lock acquisition. extractLocked
	// must re-check and return the existing value instead of re-extracting.
	c.mu.Lock()
	c.statements[key] = "racing winner"
	got, err := c.extractLocked(key)
	c.mu.Unlock()

	if err != nil {
		t.Fatalf("extractLocked: unexpected err %v", err)
	}
	if got != "racing winner" {
		t.Fatalf("extractLocked: got %q, want %q (lock-held re-check failed)", got, "racing winner")
	}
}

func TestSourceCacheFileReuse(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, happyPathSource)
	c := newSourceCache()

	// Prime the file cache for path; the statement cache only holds (path, 4).
	if got, err := c.statementAt(path, 4); err != nil || !strings.Contains(got, "return 42") {
		t.Fatalf("first lookup: %q err=%v", got, err)
	}
	// Delete the source on disk. A second lookup at a different line must
	// bypass the statement cache and reuse the parsed AST — proves load's
	// cache-hit branch is reached.
	if err := os.Remove(path); err != nil { // coverage-ignore
		t.Fatalf("remove: %v", err)
	}
	got, err := c.statementAt(path, 3)
	if err != nil {
		t.Fatalf("second lookup: unexpected err %v", err)
	}
	if got == "" {
		t.Fatalf("second lookup at different line returned empty; AST cache not reused")
	}
}

const multiLineSource = `package x

func f() int {
	return 1 +
		2 +
		3
}
`

func TestSourceCacheMultiLineStatement(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, multiLineSource)
	c := newSourceCache()

	// Asking for the middle line of a multi-line return should return
	// the whole return statement — exercises the pos < line <= end branch.
	got, err := c.statementAt(path, 5)
	if err != nil {
		t.Fatalf("statementAt: unexpected err %v", err)
	}
	for _, want := range []string{"return 1 +", "2 +", "3"} {
		if !strings.Contains(got, want) {
			t.Fatalf("statementAt: got %q, want substring %q", got, want)
		}
	}
}

func TestFormatDrawLineWithStatement(t *testing.T) {
	t.Parallel()
	loc, stmt := formatDrawLine("/abs/path/example_test.go", 42, "x := hegel.Draw(...)", []int{0, 0})
	wantLoc := "example_test.go:42"
	wantStmt := "x := hegel.Draw(...) = []int{0, 0}"
	if loc != wantLoc {
		t.Fatalf("formatDrawLine location: got %q, want %q", loc, wantLoc)
	}
	if stmt != wantStmt {
		t.Fatalf("formatDrawLine statement: got %q, want %q", stmt, wantStmt)
	}
}

func TestFormatDrawLineWithoutStatement(t *testing.T) {
	t.Parallel()
	loc, stmt := formatDrawLine("/abs/path/example_test.go", 42, "", 7)
	wantLoc := "example_test.go:42"
	wantStmt := "hegel.Draw[int](...) = 7"
	if loc != wantLoc {
		t.Fatalf("formatDrawLine location: got %q, want %q", loc, wantLoc)
	}
	if stmt != wantStmt {
		t.Fatalf("formatDrawLine statement: got %q, want %q", stmt, wantStmt)
	}
}

func TestDrawReportInProcess(t *testing.T) {
	captured := captureStdout(t, func() {
		if err := Run(func(tc TestCase) {
			_ = Draw(tc, Integers(0, 100))
		}, WithSingleTestCase()); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})
	if !strings.Contains(captured, "draw_report_test.go:") {
		t.Fatalf("expected draw report in output, got:\n%s", captured)
	}
	if !strings.Contains(captured, "Draw(tc, Integers(0, 100))") {
		t.Fatalf("expected statement text in output, got:\n%s", captured)
	}
}

// Inner Draws inside the Composite element fire at depth > 0 under the Lists
// span and must be suppressed.
func TestDrawReportSuppressedInsideSpan(t *testing.T) {
	gen := Lists(Composite(func(tc TestCase) int {
		return Draw(tc, Integers(0, 100))
	})).MinSize(2).MaxSize(2)

	captured := captureStdout(t, func() {
		if err := Run(func(tc TestCase) {
			_ = Draw(tc, gen)
		}, WithSingleTestCase()); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	got := strings.Count(captured, "draw_report_test.go:")
	if got != 1 {
		t.Fatalf("expected exactly 1 draw line in output, got %d:\n%s", got, captured)
	}
	if !strings.Contains(captured, "Draw(tc, gen)") {
		t.Fatalf("expected outer Draw statement text in output, got:\n%s", captured)
	}
}

// Isolates labelComposite from labelList: a top-level Composite must
// suppress its own inner Draws even without an enclosing Lists span.
func TestDrawReportSuppressedInsideComposite(t *testing.T) {
	gen := Composite(func(tc TestCase) int {
		a := Draw(tc, Integers(0, 100))
		b := Draw(tc, Integers(0, 100))
		return a + b
	})

	captured := captureStdout(t, func() {
		if err := Run(func(tc TestCase) {
			_ = Draw(tc, gen)
		}, WithSingleTestCase()); err != nil {
			t.Fatalf("Run: %v", err)
		}
	})

	got := strings.Count(captured, "draw_report_test.go:")
	if got != 1 {
		t.Fatalf("expected exactly 1 draw line in output, got %d:\n%s", got, captured)
	}
	if !strings.Contains(captured, "Draw(tc, gen)") {
		t.Fatalf("expected outer Draw statement text in output, got:\n%s", captured)
	}
}

// captureStdout redirects os.Stdout for the duration of fn. Not safe for
// concurrent test execution — caller must not use t.Parallel.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	saved := os.Stdout
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		buf, _ := io.ReadAll(r)
		done <- string(buf)
	}()

	fn()
	w.Close()
	os.Stdout = saved
	return <-done
}

func TestIsHegelFrameExternalTestPackage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		fn   string
		want bool
	}{
		{"hegel.dev/go/hegel.Draw[...]", true},
		{"hegel.dev/go/hegel", true},
		{"hegel.dev/go/hegel/internal/foo.Bar", true},
		{"hegel.dev/go/hegel_test.TestSomething", false},
		{"hegel.dev/go/hegelfoo.Bar", false},
		{"testing.tRunner", false},
		{"short", false},
	}
	for _, tc := range cases {
		if got := isHegelFrame(tc.fn); got != tc.want {
			t.Errorf("isHegelFrame(%q) = %v, want %v", tc.fn, got, tc.want)
		}
	}
}
