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
	"errors"
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

type wrapper[T ~uintptr] struct {
	lib *Handle
	ptr T
}

// Wrap a C pointer into an object which automatically frees it via the GC. The
// error-reporting context is carried onto the wrapper so calls derived from it
// report their diagnostics through the same context; on a NULL return the most
// recent message is read back from it.
func wrap[T ~uintptr](op string, lib *Handle, ctx *context, new func() T, free func(T)) (*wrapper[T], error) {
	ptr := new()

	if ptr == 0 {
		if msg := ctx.lastError(); msg != "" {
			return nil, fmt.Errorf("%s: %s", op, msg)
		}
		return nil, nil
	}

	w := &wrapper[T]{lib, ptr}
	if free != nil {
		runtime.AddCleanup(w, free, ptr)
	}

	return w, nil
}

type contextT uintptr  // Equivalent of hegel_context_t
type settingsT uintptr // Equivalent of hegel_settings_t
type runT uintptr      // Equivalent of hegel_run_t
type testCaseT uintptr // Equivalent of hegel_test_case_t
type resultT uintptr   // Equivalent of hegel_result_t
type failureT uintptr  // Equivalent of hegel_failure_t

// A handle to a libhegel instance.
type Handle struct {
	handle dlhandle

	contextNew       func() contextT
	contextFree      func(contextT)
	contextLastError func(contextT) string
	version          func() string

	settingsNew             func() settingsT
	settingsFree            func(settingsT)
	settingsMode            func(settingsT, Mode)
	settingsBackend         func(settingsT, Backend)
	settingsTestCases       func(settingsT, uint64)
	settingsVerbosity       func(settingsT, Verbosity)
	settingsSeed            func(settingsT, uint64, bool)
	settingsDerandomize     func(settingsT, bool)
	settingsReportMultiFail func(settingsT, bool)
	settingsDatabase        func(contextT, settingsT, string)
	settingsDatabaseKey     func(contextT, settingsT, string)
	settingsPhases          func(settingsT, Phase)
	settingsSuppressHC      func(settingsT, HealthCheck)

	runStart     func(contextT, settingsT) runT
	runFree      func(runT)
	nextTestCase func(contextT, runT) testCaseT
	runResult    func(contextT, runT) resultT

	testCaseFromBlob func(contextT, settingsT, string) testCaseT
	testCaseFree     func(contextT, testCaseT)

	generate             func(contextT, testCaseT, *byte, uint64, **byte, *uint64) Error
	startSpan            func(contextT, testCaseT, Label) Error
	stopSpan             func(contextT, testCaseT, bool) Error
	newCollection        func(contextT, testCaseT, uint64, uint64, *Collection) Error
	collectionMore       func(contextT, testCaseT, Collection, *bool) Error
	collectionReject     func(contextT, testCaseT, Collection, string) Error
	newPool              func(contextT, testCaseT, *int64) Error
	poolAdd              func(contextT, testCaseT, int64, *int64) Error
	poolGenerate         func(contextT, testCaseT, int64, bool, *int64) Error
	newStateMachine      func(contextT, testCaseT, **byte, uint64, **byte, uint64, *int64) Error
	stateMachineNextRule func(contextT, testCaseT, int64, *int64) Error
	primitiveBoolean     func(contextT, testCaseT, float64, bool, bool, *bool) Error
	target               func(contextT, testCaseT, float64, string) Error
	markComplete         func(contextT, testCaseT, Status, string) Error
	isFinalReplay        func(testCaseT) bool

	resultStatus       func(resultT) RunStatus
	resultError        func(resultT) string
	resultFailureCount func(resultT) uint64
	resultFailure      func(resultT, uint64) failureT

	failurePanicMsg         func(failureT) string
	failureOrigin           func(failureT) string
	failureReproductionBlob func(failureT) string
}

// The global libhegel handle.
//
// Returns the handle and the path the library was loaded from.
// panics on error.
var GlobalHandle = sync.OnceValues(func() (*Handle, string) {
	handle, path, err := load()
	if err != nil {
		panic(err)
	}
	return handle, path
})

// load returns the package-level libhegel binding, loading it on first call.
// All callers share the same instance; function pointers are immutable after
// the first successful load.
func load() (*Handle, string, error) {
	return loadFromPaths(candidatePaths())
}

// Close the handle to the library.
//
// Invoking any method on [Handle] after calling this method is undefined behaviour.
func (lib *Handle) Close() error {
	return dlclose(lib.handle)
}

// loadFromPaths tries each path in order, returning the first that loads. If
// none load, falls back to fetching a cached/release artifact via
// [downloadCandidate] (skipped when HEGEL_LIBHEGEL_PATH is set: an explicit
// override means the user wants that exact file, no auto-download fallback).
// All failures are accumulated into the returned error if every attempt
// fails.
func loadFromPaths(paths []string) (*Handle, string, error) {
	var attempts []error
	for _, path := range paths {
		if lib, err := tryOpen(path); err != nil {
			attempts = append(attempts, fmt.Errorf("%s: %w", path, err))
		} else {
			return lib, path, nil
		}
	}
	if os.Getenv(LibraryPathEnv) == "" { // coverage-ignore (download fallback hits the network, not exercised in unit tests)
		// No explicit override: fetch the pinned release into the per-version cache.
		downloaded, err := downloadCandidate()
		if err != nil {
			attempts = append(attempts, fmt.Errorf("download fallback: %w", err))
		} else if lib, err := tryOpen(downloaded); err != nil {
			attempts = append(attempts, fmt.Errorf("%s: %w", downloaded, err))
		} else {
			return lib, downloaded, nil
		}
	}
	return nil, "", fmt.Errorf("could not load libhegel; tried %d location(s) [set %s to override]:\n%w",
		len(paths), LibraryPathEnv, errors.Join(attempts...))
}

// tryOpen dlopens a single path and resolves every symbol against the
// returned handle.
func tryOpen(path string) (lib *Handle, err error) {
	defer func() {
		if r := recover(); r != nil { // coverage-ignore (requires stub .so)
			err = fmt.Errorf("loading %s: symbol resolution failed: %v", path, r)
			lib = nil
		}
	}()

	libHandle, err := dlopen(path)
	if err != nil {
		return nil, err
	}

	lib = &Handle{handle: libHandle}
	err = registerSymbols(libHandle, []symbol{
		{"hegel_context_new", &lib.contextNew},
		{"hegel_context_free", &lib.contextFree},
		{"hegel_context_last_error", &lib.contextLastError},

		{"hegel_settings_new", &lib.settingsNew},
		{"hegel_settings_free", &lib.settingsFree},
		{"hegel_settings_mode", &lib.settingsMode},
		{"hegel_settings_backend", &lib.settingsBackend},
		{"hegel_settings_test_cases", &lib.settingsTestCases},
		{"hegel_settings_verbosity", &lib.settingsVerbosity},
		{"hegel_settings_seed", &lib.settingsSeed},
		{"hegel_settings_derandomize", &lib.settingsDerandomize},
		{"hegel_settings_report_multiple_failures", &lib.settingsReportMultiFail},
		{"hegel_settings_database", &lib.settingsDatabase},
		{"hegel_settings_database_key", &lib.settingsDatabaseKey},
		{"hegel_settings_phases", &lib.settingsPhases},
		{"hegel_settings_suppress_health_check", &lib.settingsSuppressHC},

		{"hegel_run_start", &lib.runStart},
		{"hegel_next_test_case", &lib.nextTestCase},
		{"hegel_run_result", &lib.runResult},
		{"hegel_run_free", &lib.runFree},

		{"hegel_test_case_from_blob", &lib.testCaseFromBlob},
		{"hegel_test_case_free", &lib.testCaseFree},

		{"hegel_generate", &lib.generate},
		{"hegel_start_span", &lib.startSpan},
		{"hegel_stop_span", &lib.stopSpan},
		{"hegel_new_collection", &lib.newCollection},
		{"hegel_collection_more", &lib.collectionMore},
		{"hegel_collection_reject", &lib.collectionReject},
		{"hegel_new_pool", &lib.newPool},
		{"hegel_pool_add", &lib.poolAdd},
		{"hegel_pool_generate", &lib.poolGenerate},
		{"hegel_new_state_machine", &lib.newStateMachine},
		{"hegel_state_machine_next_rule", &lib.stateMachineNextRule},
		{"hegel_primitive_boolean", &lib.primitiveBoolean},
		{"hegel_target", &lib.target},
		{"hegel_mark_complete", &lib.markComplete},
		{"hegel_test_case_is_final_replay", &lib.isFinalReplay},

		{"hegel_run_result_status", &lib.resultStatus},
		{"hegel_run_result_error", &lib.resultError},
		{"hegel_run_result_failure_count", &lib.resultFailureCount},
		{"hegel_run_result_failure", &lib.resultFailure},
		{"hegel_failure_panic_message", &lib.failurePanicMsg},
		{"hegel_failure_origin", &lib.failureOrigin},
		{"hegel_failure_reproduction_blob", &lib.failureReproductionBlob},

		{"hegel_version", &lib.version},
	})
	if err != nil { // coverage-ignore (requires a libhegel with missing symbols)
		_ = dlclose(libHandle)
		return nil, err
	}
	return lib, nil
}

func (lib *Handle) withContext(op string, fn func(ctx contextT) Error) error {
	ctx := lib.contextNew()
	defer lib.contextFree(ctx)

	e := fn(ctx)
	if e == OK {
		return nil
	}

	if msg := lib.contextLastError(ctx); msg != "" {
		return fmt.Errorf("%s: %s (%w)", op, msg, error(e))
	}

	return fmt.Errorf("%s: %w", op, error(e))
}

// context wraps error state.
//
// It is cheap to allocate / deallocate and must not be used concurrently.
type context wrapper[contextT]

// newContext allocates a fresh error-reporting context, freed via the GC.
func (lib *Handle) newContext() *context {
	c, _ := wrap("hegel_context_new", lib, nil, func() contextT { return lib.contextNew() }, lib.contextFree)
	return (*context)(c)
}

// rawPointer returns the underlying context pointer, or 0 for a nil context.
func (c *context) rawPointer() contextT {
	if c == nil {
		return 0
	}
	return c.ptr
}

// lastError returns the most recent diagnostic recorded on the context, or the
// empty string for a nil context or one whose most recent call succeeded.
func (c *context) lastError() string {
	if c == nil {
		return ""
	}
	return c.lib.contextLastError(c.ptr)
}

type Settings wrapper[settingsT]

func (lib *Handle) SettingsNew() *Settings {
	s, _ := wrap("hegel_settings_new", lib, nil, lib.settingsNew, lib.settingsFree)
	return (*Settings)(s)
}

func (s *Settings) Mode(m Mode) {
	s.lib.settingsMode(s.ptr, m)
}

// Backend selects the engine's randomness backend. See [Backend].
func (s *Settings) Backend(b Backend) {
	s.lib.settingsBackend(s.ptr, b)
}

func (s *Settings) TestCases(n uint64) {
	s.lib.settingsTestCases(s.ptr, n)
}

func (s *Settings) Verbosity(v Verbosity) {
	s.lib.settingsVerbosity(s.ptr, v)
}

func (s *Settings) Seed(seed uint64, hasSeed bool) {
	s.lib.settingsSeed(s.ptr, seed, hasSeed)
}

func (s *Settings) Derandomize(on bool) {
	s.lib.settingsDerandomize(s.ptr, on)
}

func (s *Settings) ReportMultipleFailures(yes bool) {
	s.lib.settingsReportMultiFail(s.ptr, yes)
}

func (s *Settings) Database(path string) {
	s.lib.settingsDatabase(0, s.ptr, path)
}

func (s *Settings) DatabaseKey(key string) {
	s.lib.settingsDatabaseKey(0, s.ptr, key)
}

func (s *Settings) Phases(p Phase) {
	s.lib.settingsPhases(s.ptr, p)
}

func (s *Settings) SuppressHealthCheck(checks HealthCheck) {
	s.lib.settingsSuppressHC(s.ptr, checks)
}

func (s *Settings) RunStart() (*Run, error) {
	ctx := s.lib.newContext()
	r, err := wrap("hegel_run_start", s.lib, ctx, func() runT { return s.lib.runStart(ctx.ptr, s.ptr) }, s.lib.runFree)
	return (*Run)(r), err
}

// TestCaseFromBlob builds a standalone test case that replays the example
// encoded in a base64 failure blob (from [Failure.ReproductionBlob]). Unlike
// test cases from [Run.NextTestCase], the returned handle is owned by the
// caller and is freed automatically via the GC. Returns nil, nil when the blob
// is rejected without a diagnostic.
func (s *Settings) TestCaseFromBlob(blob string) (*TestCase, error) {
	// hegel_test_case_free now takes the context too; the free closure captures
	// the *libContext (not just its pointer) so the context outlives the test
	// case it frees.
	lib := s.lib
	ctx := lib.newContext()
	tc, err := wrap("hegel_test_case_from_blob", lib, ctx,
		func() testCaseT { return lib.testCaseFromBlob(ctx.rawPointer(), s.ptr, blob) },
		func(t testCaseT) { lib.testCaseFree(0, t) })
	return (*TestCase)(tc), err
}

type Run wrapper[runT]

// Returns nil, nil when there are no more test cases.
func (r *Run) NextTestCase() (*TestCase, error) {
	// TODO: What does cleanup look like?
	ctx := r.lib.newContext()
	tc, err := wrap("hegel_next_test_case", r.lib, ctx, func() testCaseT { return r.lib.nextTestCase(ctx.rawPointer(), r.ptr) }, nil)
	return (*TestCase)(tc), err
}

func (r *Run) RunResult() (*Result, error) {
	ctx := r.lib.newContext()
	res, err := wrap("hegel_run_result", r.lib, ctx, func() resultT { return r.lib.runResult(ctx.rawPointer(), r.ptr) }, nil)
	return (*Result)(res), err
}

type TestCase wrapper[testCaseT]

func (tc *TestCase) Generate(schema []byte) ([]byte, error) {
	var out *byte
	var size uint64
	err := tc.lib.withContext("hegel_generate", func(ctx contextT) Error {
		return tc.lib.generate(ctx, tc.ptr, slicePtr(schema), uint64(len(schema)), &out, &size)
	})
	if err != nil {
		return nil, err
	}
	return slices.Clone(unsafe.Slice(out, size)), nil
}

func (tc *TestCase) StartSpan(label Label) error {
	return tc.lib.withContext("hegel_start_span", func(ctx contextT) Error {
		return tc.lib.startSpan(ctx, tc.ptr, label)
	})
}

func (tc *TestCase) StopSpan(discard bool) error {
	return tc.lib.withContext("hegel_stop_span", func(ctx contextT) Error {
		return tc.lib.stopSpan(ctx, tc.ptr, discard)
	})
}

func (tc *TestCase) NewCollection(min, max uint64) (coll Collection, err error) {
	err = tc.lib.withContext("hegel_new_collection", func(ctx contextT) Error {
		return tc.lib.newCollection(ctx, tc.ptr, min, max, &coll)
	})
	return
}

func (tc *TestCase) CollectionMore(coll Collection) (more bool, err error) {
	err = tc.lib.withContext("hegel_collection_more", func(ctx contextT) Error {
		return tc.lib.collectionMore(ctx, tc.ptr, coll, &more)
	})
	return
}

func (tc *TestCase) CollectionReject(coll Collection, why string) error {
	return tc.lib.withContext("hegel_collection_reject", func(ctx contextT) Error {
		return tc.lib.collectionReject(ctx, tc.ptr, coll, why)
	})
}

// NewPool creates an engine-managed variable pool for stateful testing.
func (tc *TestCase) NewPool() (pool int64, err error) {
	err = tc.lib.withContext("hegel_new_pool", func(ctx contextT) Error {
		return tc.lib.newPool(ctx, tc.ptr, &pool)
	})
	return
}

// PoolAdd registers a new variable in the pool, returning the engine-assigned id.
func (tc *TestCase) PoolAdd(pool int64) (variable int64, err error) {
	err = tc.lib.withContext("hegel_pool_add", func(ctx contextT) Error {
		return tc.lib.poolAdd(ctx, tc.ptr, pool, &variable)
	})
	return
}

// PoolGenerate draws a variable id from the pool, letting the engine choose and
// shrink which previously-added variable to reuse. When consume is true the
// drawn variable is removed from the pool.
func (tc *TestCase) PoolGenerate(pool int64, consume bool) (variable int64, err error) {
	err = tc.lib.withContext("hegel_pool_generate", func(ctx contextT) Error {
		return tc.lib.poolGenerate(ctx, tc.ptr, pool, consume, &variable)
	})
	return
}

// NewStateMachine registers a state machine for engine-owned stateful testing,
// with the named rules and invariants. Rule selection (including swarm testing)
// is owned by the engine and driven via [TestCase.StateMachineNextRule].
func (tc *TestCase) NewStateMachine(ruleNames, invariantNames []string) (machine int64, err error) {
	rules, err := cStringArray(ruleNames)
	if err != nil {
		return 0, fmt.Errorf("hegel_new_state_machine: rule names: %w", err)
	}
	invariants, err := cStringArray(invariantNames)
	if err != nil {
		return 0, fmt.Errorf("hegel_new_state_machine: invariant names: %w", err)
	}
	err = tc.lib.withContext("hegel_new_state_machine", func(ctx contextT) Error {
		return tc.lib.newStateMachine(
			ctx,
			tc.ptr,
			slicePtr(rules), uint64(len(ruleNames)),
			slicePtr(invariants), uint64(len(invariantNames)),
			&machine,
		)
	})
	return
}

// StateMachineNextRule draws the index of the next rule to run, in
// [0, num_rules), letting the engine choose and shrink the rule sequence.
func (tc *TestCase) StateMachineNextRule(machine int64) (rule int64, err error) {
	err = tc.lib.withContext("hegel_state_machine_next_rule", func(ctx contextT) Error {
		return tc.lib.stateMachineNextRule(ctx, tc.ptr, machine, &rule)
	})
	return
}

// PrimitiveBoolean draws a single boolean that is true with probability p. When
// hasForced is true the result is forced to forced (consuming no entropy and
// not shrunk).
func (tc *TestCase) PrimitiveBoolean(p float64, forced, hasForced bool) (value bool, err error) {
	err = tc.lib.withContext("hegel_primitive_boolean", func(ctx contextT) Error {
		return tc.lib.primitiveBoolean(ctx, tc.ptr, p, forced, hasForced, &value)
	})
	return
}

func (tc *TestCase) Target(value float64, label string) error {
	return tc.lib.withContext("hegel_target", func(ctx contextT) Error {
		return tc.lib.target(ctx, tc.ptr, value, label)
	})
}

func (tc *TestCase) MarkComplete(status Status, origin string) error {
	return tc.lib.withContext("hegel_mark_complete", func(ctx contextT) Error {
		return tc.lib.markComplete(ctx, tc.ptr, status, origin)
	})
}

func (tc *TestCase) IsFinalReplay() bool {
	return tc.lib.isFinalReplay(tc.ptr)
}

type Result wrapper[resultT]

// Status reports whether the run passed, failed with counterexamples, or
// errored (the run itself failed and produced no verdict — see [Result.ErrorMessage]).
func (r *Result) Status() RunStatus {
	return r.lib.resultStatus(r.ptr)
}

// ErrorMessage returns the run-level error message when [Result.Status] is
// [RUN_STATUS_ERROR], or the empty string otherwise.
func (r *Result) ErrorMessage() string {
	return r.lib.resultError(r.ptr)
}

func (r *Result) FailureCount() uint64 {
	return r.lib.resultFailureCount(r.ptr)
}

// Failure returns the index-th distinct counterexample, or nil when index is
// out of range (index >= [Result.FailureCount]) or the result is NULL.
// hegel_run_result_failure takes no context and has no diagnostic channel, so
// there is no error to report — a rejected lookup simply yields nil.
func (r *Result) Failure(index uint64) *Failure {
	f, _ := wrap("hegel_run_result_failure", r.lib, nil, func() failureT { return r.lib.resultFailure(r.ptr, index) }, nil)
	return (*Failure)(f)
}

type Failure wrapper[failureT]

func (f *Failure) PanicMessage() string {
	return f.lib.failurePanicMsg(f.ptr)
}

func (f *Failure) Origin() string {
	return f.lib.failureOrigin(f.ptr)
}

// ReproductionBlob returns the failure's base64 reproduction blob, suitable
// for deterministic replay via [Settings.TestCaseFromBlob], or the empty
// string if the engine produced no blob for this failure.
func (f *Failure) ReproductionBlob() string {
	return f.lib.failureReproductionBlob(f.ptr)
}

func slicePtr[E any](s []E) *E {
	if len(s) > 0 {
		return &s[0]
	}
	return nil
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
