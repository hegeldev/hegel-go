package libhegel

import (
	"errors"
	"testing"
)

func TestProfileSettingsRoundTrip(t *testing.T) {
	ctx := NewContext()
	s, err := ctx.SettingsNewForProfile("base")
	if err != nil {
		t.Fatal(err)
	}
	if db, err := s.GetDatabase(ctx); err != nil || db != nil {
		t.Fatalf("default database = %v, %v", db, err)
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
	if got, err := s.GetDatabase(ctx); got == nil || *got != "" || err != nil {
		t.Fatalf("database = %v, %v", got, err)
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
	s := &Settings{syms: ctx.syms, raw: 2}
	if err := s.RegisterProfile(ctx, "unit-profile"); err != nil {
		t.Fatal(err)
	}
	name := "unit-profile"
	if err := ctx.SetDefaultProfile(&name); err != nil {
		t.Fatal(err)
	}
	if err := ctx.SetDefaultProfile(nil); err != nil {
		t.Fatal(err)
	}
	name = "invalid\x00name"
	if err := ctx.SetDefaultProfile(&name); err == nil {
		t.Fatal("accepted interior NUL")
	}
}

func TestProfileBindingErrors(t *testing.T) {
	ctx := Stub(t, uintptr(0), E_INVALID_ARG, "unknown profile")
	if got, err := ctx.SettingsNewForProfile("missing"); got != nil || !errors.Is(err, E_INVALID_ARG) {
		t.Fatalf("settings = %v, %v", got, err)
	}
	ctx = Stub(t, "", E_INVALID_HANDLE, "missing settings")
	s := &Settings{syms: ctx.syms, raw: 2}
	if got, err := s.GetDatabase(ctx); got != nil || !errors.Is(err, E_INVALID_HANDLE) {
		t.Fatalf("database = %v, %v", got, err)
	}
	ctx = Stub(t, uintptr(0), E_INVALID_HANDLE, "missing test case")
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: ctx.syms, raw: 2}}
	if got, err := tc.Block(ctx, 2); got != nil || !errors.Is(err, E_INVALID_HANDLE) {
		t.Fatalf("block = %v, %v", got, err)
	}
}

func TestSettingsEnumOutParameters(t *testing.T) {
	ctx := Stub(t, int32(VERBOSITY_QUIET), OK, uint32(PHASE_ALL), OK, int32(BACKEND_URANDOM), OK)
	s := &Settings{syms: ctx.syms, raw: 2}
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
