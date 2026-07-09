package hegel

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
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

const bindingSource = `package x

func f() int {
	x := g()
	_ = g()
	a, b := g2()
	var y = g()
	var z int = g()
	var (
		p = g()
		q = g()
	)
	var noval int
	type localT int
	m[0] = g()
	return x + y + z
}
`

func TestSourceCacheNameAt(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, bindingSource)
	c := newSourceCache()

	cases := []struct {
		line int
		want string
	}{
		{4, "x"},  // x := g()
		{5, ""},   // blank identifier
		{6, ""},   // two names on the left-hand side
		{7, "y"},  // var y = g()
		{8, "z"},  // var z int = g()
		{10, ""},  // grouped var block declares several names
		{13, ""},  // var noval int has no value
		{14, ""},  // type declaration binds no value
		{15, ""},  // assignment target is not an identifier
		{16, ""},  // return statement binds nothing
		{999, ""}, // past the end of the file
	}
	for _, tc := range cases {
		got, err := c.nameAt(path, tc.line)
		if err != nil {
			t.Fatalf("nameAt(line=%d): unexpected err %v", tc.line, err)
		}
		if got != tc.want {
			t.Fatalf("nameAt(line=%d): got %q, want %q", tc.line, got, tc.want)
		}
	}
}

const multiLineBindingSource = `package x

func f() []int {
	xs := []int{
		1,
		2,
	}
	n := g(func() int {
		return 3
	})
	return xs
}
`

// Asking for an interior line of a multi-line assignment should still find
// the binding — exercises the pos <= line <= end span check.
func TestSourceCacheMultiLineBinding(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, multiLineBindingSource)
	c := newSourceCache()

	got, err := c.nameAt(path, 5)
	if err != nil {
		t.Fatalf("nameAt: unexpected err %v", err)
	}
	if got != "xs" {
		t.Fatalf("nameAt: got %q, want %q", got, "xs")
	}

	// A closure argument opens its block on the binding's own line; the
	// block must not shadow the binding.
	got, err = c.nameAt(path, 8)
	if err != nil {
		t.Fatalf("nameAt: unexpected err %v", err)
	}
	if got != "n" {
		t.Fatalf("nameAt: got %q, want %q", got, "n")
	}
}

func TestSourceCacheMissingFile(t *testing.T) {
	t.Parallel()
	c := newSourceCache()
	missing := filepath.Join(t.TempDir(), "nope.go")

	got, err := c.nameAt(missing, 1)
	if err == nil {
		t.Fatalf("nameAt(missing): expected err, got nil")
	}
	if got != "" {
		t.Fatalf("nameAt(missing): got %q, want empty", got)
	}
	// Failures are cached so we don't re-read a file we already know is
	// broken. The file entry is recorded; no name is recorded.
	if len(c.files) != 1 || len(c.names) != 0 {
		t.Fatalf("unexpected cache after failure: files=%d names=%d", len(c.files), len(c.names))
	}
	// Repeat the lookup; the second call hits the file cache and returns
	// the same error without re-parsing.
	got2, err2 := c.nameAt(missing, 1)
	if err2 == nil {
		t.Fatal("nameAt(missing) repeat: expected err, got nil")
	}
	if got2 != "" {
		t.Fatalf("nameAt(missing) repeat: got %q, want empty", got2)
	}
}

func TestSourceCacheParseError(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, "this is not go\n")
	c := newSourceCache()

	got, err := c.nameAt(path, 1)
	if err == nil {
		t.Fatalf("nameAt(invalid): expected err, got nil")
	}
	if got != "" {
		t.Fatalf("nameAt(invalid): got %q, want empty", got)
	}
}

func TestSourceCacheNameHit(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, bindingSource)
	c := newSourceCache()

	first, err := c.nameAt(path, 4)
	if err != nil || first != "x" {
		t.Fatalf("first lookup: %q err=%v", first, err)
	}

	// Delete the file; the second call with the same key must hit the
	// name cache without touching disk.
	if err := os.Remove(path); err != nil { // coverage-ignore
		t.Fatalf("remove: %v", err)
	}
	second, err := c.nameAt(path, 4)
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
	// Simulate another goroutine populating the name cache between nameAt's
	// read-lock miss and the write-lock acquisition. extractLocked must
	// re-check and return the existing value instead of re-extracting.
	c.mu.Lock()
	c.names[key] = "racing_winner"
	got, err := c.extractLocked(key)
	c.mu.Unlock()

	if err != nil {
		t.Fatalf("extractLocked: unexpected err %v", err)
	}
	if got != "racing_winner" {
		t.Fatalf("extractLocked: got %q, want %q (lock-held re-check failed)", got, "racing_winner")
	}
}

func TestSourceCacheFileReuse(t *testing.T) {
	t.Parallel()
	path := writeTempSource(t, bindingSource)
	c := newSourceCache()

	// Prime the file cache for path; the name cache only holds (path, 4).
	if got, err := c.nameAt(path, 4); err != nil || got != "x" {
		t.Fatalf("first lookup: %q err=%v", got, err)
	}
	// Delete the source on disk. A second lookup at a different line must
	// bypass the name cache and reuse the parsed AST — proves loadLocked's
	// cache-hit branch is reached.
	if err := os.Remove(path); err != nil { // coverage-ignore
		t.Fatalf("remove: %v", err)
	}
	got, err := c.nameAt(path, 7)
	if err != nil {
		t.Fatalf("second lookup: unexpected err %v", err)
	}
	if got != "y" {
		t.Fatalf("second lookup at different line: got %q, want %q; AST cache not reused", got, "y")
	}
}

func TestDrawReportInProcess(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := run(func(tc TestCase) {
		n := Draw(tc, Integers(0, 100))
		_ = Draw(tc, Integers(0, 100))
		_ = n
	}, WithSingleTestCase(), withOutput(&buf)); err != nil {
		t.Fatalf("runHegel: %v", err)
	}
	captured := buf.String()
	if !regexp.MustCompile(`(?m)^n := \d+$`).MatchString(captured) {
		t.Fatalf("expected a named draw line, got:\n%s", captured)
	}
	if !regexp.MustCompile(`(?m)^draw_1 := \d+$`).MatchString(captured) {
		t.Fatalf("expected a numbered draw line for the blank binding, got:\n%s", captured)
	}
}

// A binding name that draws more than once — a loop — prints bare the first
// time and numbered afterwards.
func TestDrawReportRepeatedName(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := run(func(tc TestCase) {
		for i := 0; i < 2; i++ {
			n := Draw(tc, Integers(0, 100))
			_ = n
		}
	}, WithSingleTestCase(), withOutput(&buf)); err != nil {
		t.Fatalf("runHegel: %v", err)
	}
	captured := buf.String()
	if !regexp.MustCompile(`(?m)^n := \d+$`).MatchString(captured) {
		t.Fatalf("expected a bare first draw line, got:\n%s", captured)
	}
	if !regexp.MustCompile(`(?m)^n_2 := \d+$`).MatchString(captured) {
		t.Fatalf("expected a numbered second draw line, got:\n%s", captured)
	}
}

// Inner Draws inside the Composite element fire at depth > 0 under the Lists
// span and must be suppressed.
func TestDrawReportSuppressedInsideSpan(t *testing.T) {
	t.Parallel()
	gen := Lists(Composite(func(tc TestCase) int {
		return Draw(tc, Integers(0, 100))
	})).MinSize(2).MaxSize(2)

	var buf bytes.Buffer
	if err := run(func(tc TestCase) {
		xs := Draw(tc, gen)
		_ = xs
	}, WithSingleTestCase(), withOutput(&buf)); err != nil {
		t.Fatalf("runHegel: %v", err)
	}
	captured := buf.String()
	got := strings.Count(captured, ":=")
	if got != 1 {
		t.Fatalf("expected exactly 1 draw line in output, got %d:\n%s", got, captured)
	}
	if !strings.Contains(captured, "xs := []int{") {
		t.Fatalf("expected the outer binding's draw line, got:\n%s", captured)
	}
}

// Isolates labelComposite from labelList: a top-level Composite must
// suppress its own inner Draws even without an enclosing Lists span.
func TestDrawReportSuppressedInsideComposite(t *testing.T) {
	t.Parallel()
	gen := Composite(func(tc TestCase) int {
		a := Draw(tc, Integers(0, 100))
		b := Draw(tc, Integers(0, 100))
		return a + b
	})

	var buf bytes.Buffer
	if err := run(func(tc TestCase) {
		total := Draw(tc, gen)
		_ = total
	}, WithSingleTestCase(), withOutput(&buf)); err != nil {
		t.Fatalf("runHegel: %v", err)
	}
	captured := buf.String()
	got := strings.Count(captured, ":=")
	if got != 1 {
		t.Fatalf("expected exactly 1 draw line in output, got %d:\n%s", got, captured)
	}
	if !regexp.MustCompile(`(?m)^total := \d+$`).MatchString(captured) {
		t.Fatalf("expected the outer binding's draw line, got:\n%s", captured)
	}
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
