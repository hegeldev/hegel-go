// Package libhegel provides low-level bindings to the Rust property-testing engine
// shipped as a C cdylib by hegel-rust.
//
// The exported surface of libhegel is described in hegel-rust/hegel-c/include/hegel.h.
// One Go func-typed field per C symbol lives on [*libhegel]; tests can
// substitute fakes without dlopen'ing anything.
//
// # Path resolution
//
// The following locations are searched for the dynamic library, first hit wins:
//
//  1. $HEGEL_LIBHEGEL_PATH if set (no fallback if it fails to open)
//  2. <projectRoot>/../hegel-rust/target/release/libhegel.<ext>
//  3. <projectRoot>/../hegel-rust/target/debug/libhegel.<ext>
//  4. ~/.cache/hegel-go/libhegel/<version>/libhegel-<goos>-<goarch>.<ext>
//     (auto-downloaded from the matching hegel-rust GitHub release; the
//     download is skipped when HEGEL_LIBHEGEL_PATH is set or when
//     HEGEL_LIBHEGEL_NO_DOWNLOAD is non-empty)
//
// where <ext> is "so" on Linux, "dylib" on macOS, and "dll" on Windows.

package libhegel

import (
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"unsafe"
)

//go:generate go tool stringer -type=Error,Status,Mode,Backend,Verbosity,RunStatus,HealthCheck,Phase,Label -linecomment -output=libhegel_string.go

// LibraryPathEnv names the env var that pins libhegel to an explicit path.
// When set, that path is loaded directly with no auto-download fallback; when
// unset, the pinned release is fetched by the auto-downloader. It is the only
// way to use a local build — the library does not search for a sibling
// hegel-rust checkout.
const LibraryPathEnv = "HEGEL_LIBHEGEL_PATH"

type Error int32

func (e Error) Error() string {
	return e.String()
}

const (
	OK Error = -iota
	E_STOP_TEST
	E_ASSUME
	E_BACKEND
	E_INVALID_HANDLE
	E_INVALID_ARG
	E_ALREADY_COMPLETE
	E_NOT_COMPLETE
	E_INTERNAL
)

type Status int32 // Equivalent of hegel_status_t

const (
	STATUS_VALID Status = iota
	STATUS_INVALID
	STATUS_OVERRUN
	STATUS_INTERESTING
)

type Mode int32 // Equivalent of hegel_mode_t

const (
	MODE_TEST_RUN Mode = iota
	MODE_SINGLE_TEST_CASE
)

type Backend int32 // Equivalent of hegel_backend_t

const (
	// Choose automatically (the default): urandom under Antithesis, otherwise
	// the default seeded PRNG.
	BACKEND_AUTO Backend = iota

	// Expand a single seeded PRNG; runs are reproducible from the seed and
	// shrinking / replay work as usual.
	BACKEND_DEFAULT

	// Read fresh entropy from /dev/urandom on every draw. Intended for running
	// under Antithesis; you almost certainly don't want it otherwise.
	BACKEND_URANDOM
)

type Verbosity int32 // Equivalent of hegel_verbosity_t

const (
	VERBOSITY_QUIET Verbosity = iota
	VERBOSITY_NORMAL
	VERBOSITY_VERBOSE
	VERBOSITY_DEBUG
)

type RunStatus int32 // Equivalent of hegel_run_status_t

const (
	// The property held across every generated test case.
	RUN_STATUS_PASSED RunStatus = iota

	// The property failed; inspect each distinct counterexample via
	// [Result.FailureCount] / [Result.Failure].
	RUN_STATUS_FAILED

	// The run itself failed — a failed health check, a nondeterministic test,
	// an engine panic — and produced no verdict on the property. There are no
	// failures to inspect; read the message via [Result.ErrorMessage].
	RUN_STATUS_ERROR
)

type HealthCheck uint32

// Health-check suppression bitmask values.
const (
	HC_FILTER_TOO_MUCH         HealthCheck = 1 << iota // filter_too_much
	HC_TOO_SLOW                                        // too_slow
	HC_TEST_CASES_TOO_LARGE                            // test_cases_too_large
	HC_LARGE_INITIAL_TEST_CASE                         // large_initial_test_case
)

type Phase uint32

const (
	PHASE_EXPLICIT Phase = 1 << iota
	PHASE_REUSE
	PHASE_GENERATE
	PHASE_TARGET
	PHASE_SHRINK
)

type Collection int64

// StateMachine identifies an engine-owned state machine registered via
// [TestCase.NewStateMachine] for the lifetime of a single test case.
type StateMachine int64

type Label uint64

const (
	// Outer span around a list / sequence.
	LABEL_LIST Label = iota + 1

	// One element of a list.
	LABEL_LIST_ELEMENT

	// Outer span around a set (unordered, no duplicates).
	LABEL_SET

	// One element of a set.
	LABEL_SET_ELEMENT

	// Outer span around a map / dictionary.
	LABEL_MAP

	// One (key, value) entry of a map.
	LABEL_MAP_ENTRY

	// Outer span around a tuple / fixed-arity record.
	LABEL_TUPLE

	// Outer span around a `one_of` / disjunction; useful so the shrinke/ can swap which branch is taken.
	LABEL_ONE_OF

	// Outer span around an `optional` (None vs Some(value)).
	LABEL_OPTIONAL

	// Outer span around a fixed-shape record (named fields know/ statically).
	LABEL_FIXED_DICT

	// Outer span around a `flat_map` / monadic dependent draw.
	LABEL_FLAT_MAP

	// Outer span around a `filter` / rejection-sampling wrapper.
	LABEL_FILTER

	// Outer span around a `map` / pure transformation.
	LABEL_MAPPED

	// Outer span around a `sampled_from` / pick-from-collection draw.
	LABEL_SAMPLED_FROM

	// Outer span around the variant discriminator of a sum-type draw.
	LABEL_ENUM_VARIANT

	// Span around one swarm-testing feature-flag draw. Emitted internally by
	// the engine's state-machine rule selection; callers normally never open
	// this span themselves.
	LABEL_FEATURE_FLAG

	// Binding-specific labels, beyond the upstream HEGEL_LABEL_* range. The
	// engine treats span labels as opaque shrinker hints, so hegel-go reserves
	// values past the last upstream constant for its own span structures.
	LABEL_COMPOSITE
	LABEL_STATEFUL
)

type pointer[T ~uintptr] struct {
	syms *symbols
	raw  T
}

type ctxT uintptr      // Equivalent of hegel_context_t
type settingsT uintptr // Equivalent of hegel_settings_t
type runT uintptr      // Equivalent of hegel_run_t
type testCaseT uintptr // Equivalent of hegel_test_case_t
type resultT uintptr   // Equivalent of hegel_run_result_t
type failureT uintptr  // Equivalent of hegel_failure_t

// symbols is the loaded libhegel shared library: the dlopen handle plus one Go
// func per C entry point. It is immutable after load and freely shared across
// goroutines; the per-call error-reporting context lives on [Context] instead.
//
// Every fallible C call takes a hegel_context_t as its first argument and
// returns a result code; any value it produces is written through a trailing
// out-parameter. The func signatures below mirror that convention exactly.
type symbols struct {
	handle dlhandle

	contextNew       func() ctxT
	contextFree      func(ctxT) Error
	contextLastError func(ctxT) string

	settingsNew                       func(ctxT, *settingsT) Error
	settingsFree                      func(ctxT, settingsT) Error
	settingsSetMode                   func(ctxT, settingsT, Mode) Error
	settingsSetBackend                func(ctxT, settingsT, Backend) Error
	settingsSetTestCases              func(ctxT, settingsT, uint64) Error
	settingsSetVerbosity              func(ctxT, settingsT, Verbosity) Error
	settingsSetSeed                   func(ctxT, settingsT, uint64, bool) Error
	settingsSetDerandomize            func(ctxT, settingsT, bool) Error
	settingsSetReportMultipleFailures func(ctxT, settingsT, bool) Error
	settingsSetDatabase               func(ctxT, settingsT, string) Error
	settingsSetDatabaseKey            func(ctxT, settingsT, string) Error
	settingsSetPhases                 func(ctxT, settingsT, Phase) Error
	settingsSetSuppressHealthCheck    func(ctxT, settingsT, HealthCheck) Error

	runStart     func(ctxT, settingsT, *runT) Error
	runFree      func(ctxT, runT) Error
	nextTestCase func(ctxT, runT, *testCaseT) Error
	runResult    func(ctxT, runT, *resultT) Error

	testCaseFromBlob func(ctxT, settingsT, string, *testCaseT) Error
	testCaseFree     func(ctxT, testCaseT) Error

	generate             func(ctxT, testCaseT, *byte, uint64, **byte, *uint64) Error
	startSpan            func(ctxT, testCaseT, Label) Error
	stopSpan             func(ctxT, testCaseT, bool) Error
	newCollection        func(ctxT, testCaseT, uint64, uint64, *Collection) Error
	collectionMore       func(ctxT, testCaseT, Collection, *bool) Error
	collectionReject     func(ctxT, testCaseT, Collection, string) Error
	newPool              func(ctxT, testCaseT, *int64) Error
	poolAdd              func(ctxT, testCaseT, int64, *int64) Error
	poolGenerate         func(ctxT, testCaseT, int64, bool, *int64) Error
	newStateMachine      func(ctxT, testCaseT, **byte, uint64, **byte, uint64, *StateMachine) Error
	stateMachineNextRule func(ctxT, testCaseT, StateMachine, *int64) Error
	primitiveBoolean     func(ctxT, testCaseT, float64, bool, bool, *bool) Error
	target               func(ctxT, testCaseT, float64, string) Error
	markComplete         func(ctxT, testCaseT, Status, string) Error

	resultStatus       func(ctxT, resultT, *RunStatus) Error
	resultError        func(ctxT, resultT, **byte) Error
	resultFailureCount func(ctxT, resultT, *uint64) Error
	resultFailure      func(ctxT, resultT, uint64, *failureT) Error

	failureOrigin           func(ctxT, failureT, **byte) Error
	failureReproductionBlob func(ctxT, failureT, **byte) Error

	version func(ctxT, **byte) Error
}

type Context pointer[ctxT]

// NewContext allocates a fresh error-reporting context over the process-wide
// loaded library, loading the library on first use (panicking if it cannot be
// found).
func NewContext() *Context {
	syms := globalSymbols()
	ctx := &Context{syms, syms.contextNew()}
	runtime.AddCleanup(ctx, func(ctx ctxT) { syms.contextFree(ctx) }, ctx.raw)
	return ctx
}

func allocate[R ~uintptr](c *Context, op string, new func(ctx ctxT, raw *R) Error, free func(ctx ctxT, raw R) Error) (*pointer[R], error) {
	ptr := pointer[R]{syms: c.syms}
	err := c.invoke(op, func(ctx ctxT) Error {
		return new(ctx, &ptr.raw)
	})

	if err != nil {
		return nil, err
	}

	// A zero handle with no error is the "no handle" sentinel: the engine
	// declined to produce one without raising an error (e.g.
	// hegel_next_test_case once the run is finished, or
	// hegel_test_case_from_blob rejecting a stale blob). Callers map this to a
	// nil result.
	if ptr.raw == 0 {
		return nil, nil
	}

	if free != nil {
		runtime.AddCleanup(&ptr, func(raw R) { free(0, raw) }, ptr.raw)
	}

	return &ptr, nil
}

// invoke a function with a ctxT parameter.
//
// It is invalid to invoke on a nil context, in which case a NULL pointer is passed.
func (c *Context) invoke(op string, fn func(ctx ctxT) Error) error {
	var ptr ctxT
	if c != nil {
		ptr = c.raw
	}

	e := fn(ptr)
	if e == OK {
		return nil
	}

	if c != nil {
		if msg := c.syms.contextLastError(c.raw); msg != "" {
			return fmt.Errorf("%s: %w: %s", op, e, msg)
		}
	}

	return fmt.Errorf("%s: %w", op, e)
}

// globalSymbols loads the libhegel shared library once and returns the shared,
// immutable symbol table together with the path it was loaded from. Panics on
// error. Callers obtain a usable handle via [NewContext], which pairs these
// symbols with a fresh error-reporting context.
var globalSymbols = sync.OnceValue(func() *symbols {
	syms, err := load()
	if err != nil {
		panic(err)
	}
	return syms
})

// Close unloads the library (dlclose). Invoking any method that routes through
// these symbols afterwards is undefined behaviour.
func (s *symbols) Close() error {
	return dlclose(s.handle)
}

// Version reports the loaded library's version (hegel_version).
func (s *symbols) Version() string {
	var out *byte
	_ = s.version(0, &out)
	return goString(out)
}

func load() (*symbols, error) {
	path := os.Getenv(LibraryPathEnv)
	if path == "" {
		var err error
		path, err = downloadCandidate()
		if err != nil { // coverage-ignore (download fallback hits the network, not exercised in unit tests)
			return nil, fmt.Errorf("download libhegel: %w", err)
		}
	}

	syms, err := tryOpen(path)
	if err != nil {
		return nil, fmt.Errorf("could not load libhegel from %s (set %s to override): %w", path, LibraryPathEnv, err)
	}

	return syms, nil
}

// tryOpen dlopens a single path and resolves every symbol against the
// returned handle.
func tryOpen(path string) (syms *symbols, err error) {
	defer func() {
		if r := recover(); r != nil { // coverage-ignore (requires stub .so)
			err = fmt.Errorf("loading %s: symbol resolution failed: %v", path, r)
			syms = nil
		}
	}()

	libHandle, err := dlopen(path)
	if err != nil {
		return nil, err
	}

	syms = &symbols{handle: libHandle}
	err = registerSymbols(libHandle, []symbol{
		{"hegel_context_new", &syms.contextNew},
		{"hegel_context_free", &syms.contextFree},
		{"hegel_context_last_error", &syms.contextLastError},

		{"hegel_settings_new", &syms.settingsNew},
		{"hegel_settings_free", &syms.settingsFree},
		{"hegel_settings_set_mode", &syms.settingsSetMode},
		{"hegel_settings_set_backend", &syms.settingsSetBackend},
		{"hegel_settings_set_test_cases", &syms.settingsSetTestCases},
		{"hegel_settings_set_verbosity", &syms.settingsSetVerbosity},
		{"hegel_settings_set_seed", &syms.settingsSetSeed},
		{"hegel_settings_set_derandomize", &syms.settingsSetDerandomize},
		{"hegel_settings_set_report_multiple_failures", &syms.settingsSetReportMultipleFailures},
		{"hegel_settings_set_database", &syms.settingsSetDatabase},
		{"hegel_settings_set_database_key", &syms.settingsSetDatabaseKey},
		{"hegel_settings_set_phases", &syms.settingsSetPhases},
		{"hegel_settings_set_suppress_health_check", &syms.settingsSetSuppressHealthCheck},

		{"hegel_run_start", &syms.runStart},
		{"hegel_next_test_case", &syms.nextTestCase},
		{"hegel_run_result", &syms.runResult},
		{"hegel_run_free", &syms.runFree},

		{"hegel_test_case_from_blob", &syms.testCaseFromBlob},
		{"hegel_test_case_free", &syms.testCaseFree},

		{"hegel_generate", &syms.generate},
		{"hegel_start_span", &syms.startSpan},
		{"hegel_stop_span", &syms.stopSpan},
		{"hegel_new_collection", &syms.newCollection},
		{"hegel_collection_more", &syms.collectionMore},
		{"hegel_collection_reject", &syms.collectionReject},
		{"hegel_new_pool", &syms.newPool},
		{"hegel_pool_add", &syms.poolAdd},
		{"hegel_pool_generate", &syms.poolGenerate},
		{"hegel_new_state_machine", &syms.newStateMachine},
		{"hegel_state_machine_next_rule", &syms.stateMachineNextRule},
		{"hegel_primitive_boolean", &syms.primitiveBoolean},
		{"hegel_target", &syms.target},
		{"hegel_mark_complete", &syms.markComplete},

		{"hegel_run_result_status", &syms.resultStatus},
		{"hegel_run_result_error", &syms.resultError},
		{"hegel_run_result_failure_count", &syms.resultFailureCount},
		{"hegel_run_result_failure", &syms.resultFailure},
		{"hegel_failure_origin", &syms.failureOrigin},
		{"hegel_failure_reproduction_blob", &syms.failureReproductionBlob},

		{"hegel_version", &syms.version},
	})
	if err != nil { // coverage-ignore (requires a libhegel with missing symbols)
		_ = dlclose(libHandle)
		return nil, err
	}
	return syms, nil
}

type Settings pointer[settingsT]

// SettingsNew allocates a fresh settings object on this context.
func (c *Context) SettingsNew() *Settings {
	ptr, _ := allocate[settingsT](c, "hegel_settings_new", func(ctx ctxT, raw *settingsT) Error {
		return c.syms.settingsNew(ctx, raw)
	}, c.syms.settingsFree)
	return (*Settings)(ptr)
}

func (s *Settings) Mode(ctx *Context, m Mode) error {
	return ctx.invoke("hegel_settings_set_mode", func(ctx ctxT) Error {
		return s.syms.settingsSetMode(ctx, s.raw, m)
	})
}

// Backend selects the engine's randomness backend. See [Backend].
func (s *Settings) Backend(ctx *Context, b Backend) error {
	return ctx.invoke("hegel_settings_set_backend", func(ctx ctxT) Error {
		return s.syms.settingsSetBackend(ctx, s.raw, b)
	})
}

func (s *Settings) TestCases(ctx *Context, n uint64) error {
	return ctx.invoke("hegel_settings_set_test_cases", func(ctx ctxT) Error {
		return s.syms.settingsSetTestCases(ctx, s.raw, n)
	})
}

func (s *Settings) Verbosity(ctx *Context, v Verbosity) error {
	return ctx.invoke("hegel_settings_set_verbosity", func(ctx ctxT) Error {
		return s.syms.settingsSetVerbosity(ctx, s.raw, v)
	})
}

func (s *Settings) Seed(ctx *Context, seed uint64, hasSeed bool) error {
	return ctx.invoke("hegel_settings_set_seed", func(ctx ctxT) Error {
		return s.syms.settingsSetSeed(ctx, s.raw, seed, hasSeed)
	})
}

func (s *Settings) Derandomize(ctx *Context, on bool) error {
	return ctx.invoke("hegel_settings_set_derandomize", func(ctx ctxT) Error {
		return s.syms.settingsSetDerandomize(ctx, s.raw, on)
	})
}

func (s *Settings) ReportMultipleFailures(ctx *Context, yes bool) error {
	return ctx.invoke("hegel_settings_set_report_multiple_failures", func(ctx ctxT) Error {
		return s.syms.settingsSetReportMultipleFailures(ctx, s.raw, yes)
	})
}

func (s *Settings) Database(ctx *Context, path string) error {
	return ctx.invoke("hegel_settings_set_database", func(ctx ctxT) Error {
		return s.syms.settingsSetDatabase(ctx, s.raw, path)
	})
}

func (s *Settings) DatabaseKey(ctx *Context, key string) error {
	return ctx.invoke("hegel_settings_set_database_key", func(ctx ctxT) Error {
		return s.syms.settingsSetDatabaseKey(ctx, s.raw, key)
	})
}

func (s *Settings) Phases(ctx *Context, p Phase) error {
	return ctx.invoke("hegel_settings_set_phases", func(ctx ctxT) Error {
		return s.syms.settingsSetPhases(ctx, s.raw, p)
	})
}

func (s *Settings) SuppressHealthCheck(ctx *Context, checks HealthCheck) error {
	return ctx.invoke("hegel_settings_set_suppress_health_check", func(ctx ctxT) Error {
		return s.syms.settingsSetSuppressHealthCheck(ctx, s.raw, checks)
	})
}

func (s *Settings) RunStart(ctx *Context) (*Run, error) {
	ptr, err := allocate(ctx, "hegel_run_start", func(ctx ctxT, raw *runT) Error {
		e := s.syms.runStart(ctx, s.raw, raw)
		runtime.KeepAlive(s)
		return e
	}, s.syms.runFree)
	return (*Run)(ptr), err
}

// TestCaseFromBlob builds a standalone test case that replays the example
// encoded in a base64 failure blob (from [Failure.ReproductionBlob]). Unlike
// test cases from [Run.NextTestCase], the returned handle is owned by the
// caller and is freed automatically via the GC. A rejected blob surfaces as a
// nil test case and a non-nil error.
func (s *Settings) TestCaseFromBlob(ctx *Context, blob string) (tc *TestCase, err error) {
	ptr, err := allocate(ctx, "hegel_test_case_from_blob", func(ctx ctxT, raw *testCaseT) Error {
		e := s.syms.testCaseFromBlob(ctx, s.raw, blob, raw)
		runtime.KeepAlive(s)
		return e
	}, s.syms.testCaseFree)
	if ptr == nil {
		return nil, err
	}
	return &TestCase{pointer: ptr}, err
}

type Run pointer[runT]

// Returns nil, nil when there are no more test cases.
func (r *Run) NextTestCase(ctx *Context) (*TestCase, error) {
	ptr, err := allocate(ctx, "hegel_next_test_case", func(ctx ctxT, raw *testCaseT) Error {
		e := r.syms.nextTestCase(ctx, r.raw, raw)
		runtime.KeepAlive(r)
		return e
	}, nil)
	if ptr == nil {
		return nil, err
	}
	return &TestCase{pointer: ptr}, err
}

func (r *Run) RunResult(ctx *Context) (*Result, error) {
	ptr, err := allocate(ctx, "hegel_run_result", func(ctx ctxT, raw *resultT) Error {
		e := r.syms.runResult(ctx, r.raw, raw)
		runtime.KeepAlive(r)
		return e
	}, nil)
	return &Result{pointer: ptr, parent: r}, err
}

// TestCase wraps a hegel_test_case_t. The out* fields are reusable scratch for
// the hot per-draw calls below: keeping them on this (heap-allocated) struct
// lets each call pass a pointer into long-lived storage instead of allocating a
// fresh temporary on every draw (a pointer handed to purego's reflection-based
// call path necessarily escapes to the heap).
type TestCase struct {
	*pointer[testCaseT]

	outBytes *byte
	outSize  uint64
	outBool  bool
	outInt   int64
	outColl  Collection
	outSM    StateMachine
}

func (tc *TestCase) Generate(ctx *Context, schema []byte) ([]byte, error) {
	err := ctx.invoke("hegel_generate", func(ctx ctxT) Error {
		return tc.syms.generate(ctx, tc.raw, slicePtr(schema), uint64(len(schema)), &tc.outBytes, &tc.outSize)
	})
	if err != nil {
		return nil, err
	}
	return slices.Clone(unsafe.Slice(tc.outBytes, tc.outSize)), nil
}

func (tc *TestCase) StartSpan(ctx *Context, label Label) error {
	return ctx.invoke("hegel_start_span", func(ctx ctxT) Error {
		return tc.syms.startSpan(ctx, tc.raw, label)
	})
}

func (tc *TestCase) StopSpan(ctx *Context, discard bool) error {
	return ctx.invoke("hegel_stop_span", func(ctx ctxT) Error {
		return tc.syms.stopSpan(ctx, tc.raw, discard)
	})
}

func (tc *TestCase) NewCollection(ctx *Context, min, max uint64) (Collection, error) {
	err := ctx.invoke("hegel_new_collection", func(ctx ctxT) Error {
		return tc.syms.newCollection(ctx, tc.raw, min, max, &tc.outColl)
	})
	return tc.outColl, err
}

func (tc *TestCase) CollectionMore(ctx *Context, coll Collection) (bool, error) {
	err := ctx.invoke("hegel_collection_more", func(ctx ctxT) Error {
		return tc.syms.collectionMore(ctx, tc.raw, coll, &tc.outBool)
	})
	return tc.outBool, err
}

func (tc *TestCase) CollectionReject(ctx *Context, coll Collection, why string) error {
	return ctx.invoke("hegel_collection_reject", func(ctx ctxT) Error {
		return tc.syms.collectionReject(ctx, tc.raw, coll, why)
	})
}

// NewPool creates an engine-managed variable pool for stateful testing.
func (tc *TestCase) NewPool(ctx *Context) (int64, error) {
	err := ctx.invoke("hegel_new_pool", func(ctx ctxT) Error {
		return tc.syms.newPool(ctx, tc.raw, &tc.outInt)
	})
	return tc.outInt, err
}

// PoolAdd registers a new variable in the pool, returning the engine-assigned id.
func (tc *TestCase) PoolAdd(ctx *Context, pool int64) (int64, error) {
	err := ctx.invoke("hegel_pool_add", func(ctx ctxT) Error {
		return tc.syms.poolAdd(ctx, tc.raw, pool, &tc.outInt)
	})
	return tc.outInt, err
}

// PoolGenerate draws a variable id from the pool, letting the engine choose and
// shrink which previously-added variable to reuse. When consume is true the
// drawn variable is removed from the pool.
func (tc *TestCase) PoolGenerate(ctx *Context, pool int64, consume bool) (int64, error) {
	err := ctx.invoke("hegel_pool_generate", func(ctx ctxT) Error {
		return tc.syms.poolGenerate(ctx, tc.raw, pool, consume, &tc.outInt)
	})
	return tc.outInt, err
}

// NewStateMachine registers a state machine for engine-owned stateful testing,
// with the named rules and invariants. Rule selection (including swarm testing)
// is owned by the engine and driven via [TestCase.StateMachineNextRule].
func (tc *TestCase) NewStateMachine(ctx *Context, ruleNames, invariantNames []string) (StateMachine, error) {
	rules, err := cStringArray(ruleNames)
	if err != nil {
		return 0, fmt.Errorf("hegel_new_state_machine: rule names: %w", err)
	}
	invariants, err := cStringArray(invariantNames)
	if err != nil {
		return 0, fmt.Errorf("hegel_new_state_machine: invariant names: %w", err)
	}
	err = ctx.invoke("hegel_new_state_machine", func(ctx ctxT) Error {
		return tc.syms.newStateMachine(
			ctx, tc.raw,
			slicePtr(rules), uint64(len(ruleNames)),
			slicePtr(invariants), uint64(len(invariantNames)),
			&tc.outSM,
		)
	})
	return tc.outSM, err
}

// StateMachineNextRule draws the index of the next rule to run, in
// [0, num_rules), letting the engine choose and shrink the rule sequence.
func (tc *TestCase) StateMachineNextRule(ctx *Context, machine StateMachine) (int64, error) {
	err := ctx.invoke("hegel_state_machine_next_rule", func(ctx ctxT) Error {
		return tc.syms.stateMachineNextRule(ctx, tc.raw, machine, &tc.outInt)
	})
	return tc.outInt, err
}

// PrimitiveBoolean draws a single boolean that is true with probability p. When
// hasForced is true the result is forced to forced (consuming no entropy and
// not shrunk).
func (tc *TestCase) PrimitiveBoolean(ctx *Context, p float64, forced, hasForced bool) (bool, error) {
	err := ctx.invoke("hegel_primitive_boolean", func(ctx ctxT) Error {
		return tc.syms.primitiveBoolean(ctx, tc.raw, p, forced, hasForced, &tc.outBool)
	})
	return tc.outBool, err
}

func (tc *TestCase) Target(ctx *Context, value float64, label string) error {
	return ctx.invoke("hegel_target", func(ctx ctxT) Error {
		return tc.syms.target(ctx, tc.raw, value, label)
	})
}

func (tc *TestCase) MarkComplete(ctx *Context, status Status, origin string) error {
	return ctx.invoke("hegel_mark_complete", func(ctx ctxT) Error {
		return tc.syms.markComplete(ctx, tc.raw, status, origin)
	})
}

// Result wraps a hegel_run_result_t. The out* fields are reusable out-parameter
// scratch (see [TestCase] for the rationale).
//
// The handle is borrowed from the parent [Run] — freeing the run invalidates
// it — so the result retains the run to keep it reachable (and thus unfreed by
// the GC) for as long as the result itself is live.
type Result struct {
	*pointer[resultT]

	parent *Run

	outStatus RunStatus
	outCount  uint64
	outBytes  *byte
}

// Status reports whether the run passed, failed with counterexamples, or
// errored (the run itself failed and produced no verdict — see [Result.ErrorMessage]).
func (r *Result) Status(ctx *Context) RunStatus {
	// Default to ERROR so a call that somehow fails (only possible on a NULL
	// handle, which we never produce) is never mistaken for a pass.
	r.outStatus = RUN_STATUS_ERROR
	_ = ctx.invoke("hegel_run_result_status", func(ctx ctxT) Error {
		return r.syms.resultStatus(ctx, r.raw, &r.outStatus)
	})
	return r.outStatus
}

// ErrorMessage returns the run-level error message when [Result.Status] is
// [RUN_STATUS_ERROR], or the empty string otherwise.
func (r *Result) ErrorMessage(ctx *Context) string {
	_ = ctx.invoke("hegel_run_result_error", func(ctx ctxT) Error {
		return r.syms.resultError(ctx, r.raw, &r.outBytes)
	})
	return goString(r.outBytes)
}

func (r *Result) FailureCount(ctx *Context) uint64 {
	_ = ctx.invoke("hegel_run_result_failure_count", func(ctx ctxT) Error {
		return r.syms.resultFailureCount(ctx, r.raw, &r.outCount)
	})
	return r.outCount
}

func (r *Result) Failure(ctx *Context, index uint64) (*Failure, error) {
	ptr, err := allocate(ctx, "hegel_run_result_failure", func(ctx ctxT, raw *failureT) Error {
		e := r.syms.resultFailure(ctx, r.raw, index, raw)
		runtime.KeepAlive(r)
		return e
	}, nil)
	return &Failure{pointer: ptr, parent: r.parent}, err
}

// Failure wraps a hegel_failure_t. outBytes is reusable out-parameter scratch
// (see [TestCase] for the rationale).
//
// Like [Result], the handle is borrowed from the run behind the scenes, so the
// failure retains the run to keep it reachable (and thus unfreed by the GC) for
// as long as the failure itself is live.
type Failure struct {
	*pointer[failureT]

	parent *Run

	outBytes *byte
}

// Origin returns the failure's origin string — the stable identifier the
// shrinker used to group probes for this bug.
func (f *Failure) Origin(ctx *Context) string {
	_ = ctx.invoke("hegel_failure_origin", func(ctx ctxT) Error {
		return f.syms.failureOrigin(ctx, f.raw, &f.outBytes)
	})
	return goString(f.outBytes)
}

// ReproductionBlob returns the failure's base64 reproduction blob, suitable
// for deterministic replay via [Settings.TestCaseFromBlob], or the empty
// string if the engine produced no blob for this failure.
func (f *Failure) ReproductionBlob(ctx *Context) string {
	_ = ctx.invoke("hegel_failure_reproduction_blob", func(ctx ctxT) Error {
		return f.syms.failureReproductionBlob(ctx, f.raw, &f.outBytes)
	})
	return goString(f.outBytes)
}

func slicePtr[E any](s []E) *E {
	if len(s) > 0 {
		return &s[0]
	}
	return nil
}

// goString copies a NUL-terminated C string (as written into a `const char**`
// out-parameter) into a Go string. A NULL pointer — which libhegel uses to mean
// "no string" (e.g. the error message of a run that did not error) — yields "".
// The bytes are copied immediately, so a borrowed/transient C buffer is safe.
func goString(p *byte) string {
	if p == nil {
		return ""
	}
	// TODO: This is atrocious, can we fix?
	n := 0
	for q := unsafe.Pointer(p); *(*byte)(q) != 0; q = unsafe.Add(q, 1) {
		n++
	}
	return strings.Clone(string(unsafe.Slice(p, n)))
}

// cStringArray builds a C `const char *const *` array from a slice of Go
// strings, returning the pointer array whose first element is what the C
// function receives via [slicePtr]. Each pointer addresses a NUL-terminated
// buffer; those buffers stay reachable (and thus alive for the duration of the
// FFI call) through the returned []*byte, which the caller passes by pointer.
//
// A Go string may contain an interior NUL byte, but a C string cannot — it
// would be silently truncated at the first NUL. Such an input is rejected with
// an error rather than passed on as a corrupted name.
func cStringArray(ss []string) ([]*byte, error) {
	if len(ss) == 0 {
		return nil, nil
	}
	ptrs := make([]*byte, len(ss))
	for i, s := range ss {
		if strings.IndexByte(s, 0) >= 0 {
			return nil, fmt.Errorf("string %q contains an interior NUL byte", s)
		}
		buf := append([]byte(s), 0) // NUL-terminate for C
		ptrs[i] = &buf[0]
	}
	return ptrs, nil
}
