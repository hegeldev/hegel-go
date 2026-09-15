package libhegel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadLibVersion smoke-tests the loader against the real libhegel built
// by `just build-libhegel` (or in the sibling ../hegel-rust/ checkout, or the
// vendored binary embedded at build time). Asserts that hegel_version returns
// the version pinned in version.go.
func TestLoadLibVersion(t *testing.T) {
	lib, err := load()
	if err != nil {
		t.Fatalf("loadLib: %v", err)
	}
	defer lib.Close()

	got := lib.versionString()
	if got != hegelVersion {
		t.Fatalf("hegel_version: got %q, want %q (rebuild libhegel.so if it's stale)", got, hegelVersion)
	}
}

// TestRegisterSymbolsMissing covers the dlsym / registerSymbols error path: a
// symbol that the library does not export must surface a wrapped error.
func TestRegisterSymbolsMissing(t *testing.T) {
	lib, err := load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	defer lib.Close()

	var fn func()
	err = registerSymbols(lib.handle, []symbol{
		{name: "hegel_definitely_missing_symbol", dst: &fn},
	})
	if err == nil {
		t.Fatal("expected error resolving a missing symbol")
	}
}

// TestLoadLibMissing verifies the failure path when no library can be found.
func TestLoadLibMissing(t *testing.T) {
	t.Setenv(LibraryPathEnv, "/nonexistent/path/to/libhegel.so")

	_, err := load()
	if err == nil {
		t.Fatal("expected loadLib to fail with a bogus path")
	}
	if !strings.Contains(err.Error(), "load libhegel") {
		t.Errorf("expected error to mention libhegel; got %q", err)
	}
	if !strings.Contains(err.Error(), "/nonexistent/path/to/libhegel.so") {
		t.Errorf("expected error to mention the bogus path; got %q", err)
	}
}

// TestLoadEmbeddedWriteFails covers the fallback path in load: with no path
// override, writing the embedded library out to the cache must fail when the
// cache root cannot be created. We point the cache dir at a path beneath a
// regular file so MkdirAll fails with ENOTDIR.
func TestLoadEmbeddedWriteFails(t *testing.T) {
	t.Setenv(LibraryPathEnv, "")

	dir := t.TempDir()
	notADir := filepath.Join(dir, "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil { // coverage-ignore
		t.Fatalf("write file: %v", err)
	}
	t.Setenv("XDG_CACHE_HOME", notADir)
	t.Setenv("HOME", notADir) // macOS fallback when XDG_CACHE_HOME is unset
	t.Setenv("LocalAppData", notADir)

	_, err := load()
	if err == nil {
		t.Fatal("expected load to fail when the embedded library cannot be written")
	}
	if !strings.Contains(err.Error(), "write libhegel") {
		t.Errorf("expected error to mention write libhegel; got %q", err)
	}
}

func TestProfileSettingsRoundTrip(t *testing.T) {
	ctx := NewContext()
	s, err := ctx.SettingsNewForProfile("base")
	if err != nil {
		t.Fatal(err)
	}
	if db, hasDB, err := s.GetDatabase(ctx); err != nil || hasDB || db != "" {
		t.Fatalf("default database = %v, %q, %v", hasDB, db, err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.TestCases(ctx, 17))
	must(s.Verbosity(ctx, VERBOSITY_DEBUG))
	must(s.Seed(ctx, 123, true))
	must(s.Derandomize(ctx, true))
	must(s.Database(ctx, ""))
	must(s.Phases(ctx, PHASE_GENERATE|PHASE_SHRINK))
	must(s.SuppressHealthCheck(ctx, HC_TOO_SLOW))
	must(s.ReportMultipleFailures(ctx, false))
	must(s.ShowStatistics(ctx, true))
	must(s.PrintBlob(ctx, false))
	must(s.Backend(ctx, BACKEND_DEFAULT))
	must(s.TestLocation(ctx, "example_test.go", 42, "Example", "TestProperty"))
	if got, err := s.GetTestCases(ctx); got != 17 || err != nil {
		t.Fatalf("test cases = %v, %v", got, err)
	}
	if got, err := s.GetVerbosity(ctx); got != VERBOSITY_DEBUG || err != nil {
		t.Fatalf("verbosity = %v, %v", got, err)
	}
	if seed, has, err := s.GetSeed(ctx); seed != 123 || !has || err != nil {
		t.Fatalf("seed = %v, %v, %v", seed, has, err)
	}
	if got, err := s.GetDerandomize(ctx); !got || err != nil {
		t.Fatalf("derandomize = %v, %v", got, err)
	}
	if db, hasDB, err := s.GetDatabase(ctx); !hasDB || db != "" || err != nil {
		t.Fatalf("database = %v, %q, %v", hasDB, db, err)
	}
	if got, err := s.GetPhases(ctx); got != PHASE_GENERATE|PHASE_SHRINK || err != nil {
		t.Fatalf("phases = %v, %v", got, err)
	}
	if got, err := s.GetSuppressHealthCheck(ctx); got != HC_TOO_SLOW || err != nil {
		t.Fatalf("health checks = %v, %v", got, err)
	}
	if got, err := s.GetReportMultipleFailures(ctx); got || err != nil {
		t.Fatalf("multiple failures = %v, %v", got, err)
	}
	if got, err := s.GetShowStatistics(ctx); !got || err != nil {
		t.Fatalf("statistics = %v, %v", got, err)
	}
	if got, err := s.GetPrintBlob(ctx); got || err != nil {
		t.Fatalf("print blob = %v, %v", got, err)
	}
	if got, err := s.GetBackend(ctx); got != BACKEND_DEFAULT || err != nil {
		t.Fatalf("backend = %v, %v", got, err)
	}
}

func TestProfileRegistrationBindings(t *testing.T) {
	// Process-global mutations stay stubbed so unrelated tests retain their configuration.
	ctx := Stub(t, OK, OK, OK)
	s := &Settings{pointer: pointer[settingsT]{syms: ctx.syms, raw: 2}}
	if err := s.RegisterProfile(ctx, "unit-profile"); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetDefaultProfile("unit-profile"); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetDefaultProfile(""); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetDefaultProfile("invalid\x00name"); err == nil {
		t.Fatal("accepted interior NUL")
	}
}

func TestProfileBindingErrors(t *testing.T) {
	ctx := Stub(t, uintptr(0), E_INVALID_ARG, "unknown profile")
	if got, err := ctx.SettingsNewForProfile("missing"); got != nil || !errors.Is(err, E_INVALID_ARG) {
		t.Fatalf("settings = %v, %v", got, err)
	}
	ctx = Stub(t, "", E_INVALID_HANDLE, "missing settings")
	s := &Settings{pointer: pointer[settingsT]{syms: ctx.syms, raw: 2}}
	if db, hasDB, err := s.GetDatabase(ctx); hasDB || db != "" || !errors.Is(err, E_INVALID_HANDLE) {
		t.Fatalf("database = %v, %q, %v", hasDB, db, err)
	}
	ctx = Stub(t, uintptr(0), E_INVALID_HANDLE, "missing test case")
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: ctx.syms, raw: 2}}
	if got, err := tc.Block(ctx, 2); got != nil || !errors.Is(err, E_INVALID_HANDLE) {
		t.Fatalf("block = %v, %v", got, err)
	}
}

func TestSettingsEnumOutParameters(t *testing.T) {
	ctx := Stub(t, int32(VERBOSITY_QUIET), OK, uint32(PHASE_ALL), OK, int32(BACKEND_URANDOM), OK)
	s := &Settings{pointer: pointer[settingsT]{syms: ctx.syms, raw: 2}}
	if got, err := s.GetVerbosity(ctx); got != VERBOSITY_QUIET || err != nil {
		t.Fatalf("verbosity = %v, %v", got, err)
	}
	if got, err := s.GetPhases(ctx); got != PHASE_ALL || err != nil {
		t.Fatalf("phases = %v, %v", got, err)
	}
	if got, err := s.GetBackend(ctx); got != BACKEND_URANDOM || err != nil {
		t.Fatalf("backend = %v, %v", got, err)
	}
}

func TestNativeBlockSharesChoiceSequence(t *testing.T) {
	ctx := NewContext()
	settings, err := ctx.SettingsNewForProfile("base")
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Database(ctx, ""); err != nil {
		t.Fatal(err)
	}
	run, err := settings.RunStart(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc, err := run.NextTestCase(ctx)
	if err != nil || tc == nil {
		t.Fatalf("case = %v, %v", tc, err)
	}
	block, err := tc.Block(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := block.Note(ctx, "inside"); err != nil {
		t.Fatal(err)
	}
	if got, err := block.GenerateInteger(ctx, 7, 7); got != 7 || err != nil {
		t.Fatalf("draw = %v, %v", got, err)
	}
	if err := tc.Note(ctx, "outside"); err != nil {
		t.Fatal(err)
	}
	printer, err := tc.Printer(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := printer.Resolve(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := printer.Value(ctx); got != "  inside\noutside\n" || err != nil {
		t.Fatalf("document = %q, %v", got, err)
	}
	if err := tc.SetWorker(ctx, 3); err != nil {
		t.Fatal(err)
	}
	if err := tc.MarkComplete(ctx, STATUS_VALID, ""); err != nil {
		t.Fatal(err)
	}
}

func TestDerivedLabels(t *testing.T) {
	ctx := NewContext()
	list, err := ctx.LabelFromName("hegel.go.list")
	if err != nil || list != LABEL_LIST {
		t.Fatalf("list label = %v, %v", list, err)
	}
	a, err := ctx.LabelCombine([]Label{LABEL_LIST, LABEL_INTEGER})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ctx.LabelCombine([]Label{LABEL_INTEGER, LABEL_LIST})
	if err != nil || a == b {
		t.Fatalf("order-sensitive label = %v, %v", b, err)
	}
	if _, err := ctx.LabelCombine(nil); err != nil {
		t.Fatal(err)
	}
}
