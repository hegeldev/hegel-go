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
	"sync"
	"unsafe"
)

//go:generate go tool stringer -type=Error,Status,Mode,Verbosity,HealthCheck,Phase,Label -linecomment -output=libhegel_string.go

// LibraryPathEnv overrides automatic library discovery.
const LibraryPathEnv = "HEGEL_LIBHEGEL_PATH"

type Error int32

func toError(op string, e Error) error {
	if e == OK {
		return nil
	}
	return fmt.Errorf("%s: %w", op, error(e))
}

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

type Verbosity int32 // Equivalent of hegel_verbosity_t

const (
	VERBOSITY_QUIET Verbosity = iota
	VERBOSITY_NORMAL
	VERBOSITY_VERBOSE
	VERBOSITY_DEBUG
)

// TODO: Stringer

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

	// TODO: Missing from upstream
	LABEL_COMPOSITE
	LABEL_STATEFUL
)

type wrapper[T ~uintptr] struct {
	lib *Handle
	ptr T
}

// Wrap a C pointer into an object which automatically frees it via the GC.
func wrap[T ~uintptr](op string, lib *Handle, new func() T, free func(T)) (*wrapper[T], error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ptr := new()

	if ptr == 0 {
		if msg := lib.lastErrorMessage(); msg != "" {
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

type settingsT uintptr // Equivalent of hegel_settings_t
type runT uintptr      // Equivalent of hegel_run_t
type testCaseT uintptr // Equivalent of hegel_test_case_t
type resultT uintptr   // Equivalent of hegel_result_t
type failureT uintptr  // Equivalent of hegel_failure_t

// A handle to a libhegel instance.
type Handle struct {
	handle dlhandle

	// This function accesses thread-local state and therefore must be
	// called on goroutine locked to an OS thread.
	lastErrorMessage func() string
	version          func() string

	settingsNew             func() settingsT
	settingsFree            func(settingsT)
	settingsMode            func(settingsT, Mode)
	settingsTestCases       func(settingsT, uint64)
	settingsVerbosity       func(settingsT, Verbosity)
	settingsSeed            func(settingsT, uint64, bool)
	settingsDerandomize     func(settingsT, bool)
	settingsReportMultiFail func(settingsT, bool)
	settingsDatabase        func(settingsT, string)
	settingsDatabaseKey     func(settingsT, string)
	settingsPhases          func(settingsT, Phase)
	settingsSuppressHC      func(settingsT, HealthCheck)

	runStart     func(settingsT) runT
	runFree      func(runT)
	nextTestCase func(runT) testCaseT
	runResult    func(runT) resultT

	generate         func(testCaseT, *byte, uint64, **byte, *uint64) Error
	startSpan        func(testCaseT, Label) Error
	stopSpan         func(testCaseT, bool) Error
	newCollection    func(testCaseT, uint64, uint64, *Collection) Error
	collectionMore   func(testCaseT, Collection, *bool) Error
	collectionReject func(testCaseT, Collection, string) Error
	target           func(testCaseT, float64, string) Error
	markComplete     func(testCaseT, Status, string) Error
	isFinalReplay    func(testCaseT) bool

	resultPassed       func(resultT) bool
	resultFailureCount func(resultT) uint64
	resultFailure      func(resultT, uint64) failureT

	failurePanicMsg   func(failureT) string
	failureDiagnostic func(failureT) string
	failureOrigin     func(failureT) string
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
	if os.Getenv(LibraryPathEnv) == "" { // coverage-ignore (download fallback only runs when every local path fails)
		// Last resort: fetch from GitHub releases into the per-version cache.
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
		{"hegel_settings_new", &lib.settingsNew},
		{"hegel_settings_free", &lib.settingsFree},
		{"hegel_settings_mode", &lib.settingsMode},
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

		{"hegel_generate", &lib.generate},
		{"hegel_start_span", &lib.startSpan},
		{"hegel_stop_span", &lib.stopSpan},
		{"hegel_new_collection", &lib.newCollection},
		{"hegel_collection_more", &lib.collectionMore},
		{"hegel_collection_reject", &lib.collectionReject},
		{"hegel_target", &lib.target},
		{"hegel_mark_complete", &lib.markComplete},
		{"hegel_test_case_is_final_replay", &lib.isFinalReplay},

		{"hegel_run_result_passed", &lib.resultPassed},
		{"hegel_run_result_failure_count", &lib.resultFailureCount},
		{"hegel_run_result_failure", &lib.resultFailure},
		{"hegel_failure_panic_message", &lib.failurePanicMsg},
		{"hegel_failure_diagnostic", &lib.failureDiagnostic},
		{"hegel_failure_origin", &lib.failureOrigin},

		{"hegel_last_error_message", &lib.lastErrorMessage},
		{"hegel_version", &lib.version},
	})
	if err != nil { // coverage-ignore (requires a libhegel with missing symbols)
		_ = dlclose(libHandle)
		return nil, err
	}
	return lib, nil
}

type Settings wrapper[settingsT]

func (lib *Handle) SettingsNew() *Settings {
	s, _ := wrap("hegel_settings_new", lib, lib.settingsNew, lib.settingsFree)
	return (*Settings)(s)
}

func (s *Settings) Mode(m Mode) {
	s.lib.settingsMode(s.ptr, m)
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
	s.lib.settingsDatabase(s.ptr, path)
}

func (s *Settings) DatabaseKey(key string) {
	s.lib.settingsDatabaseKey(s.ptr, key)
}

func (s *Settings) Phases(p Phase) {
	s.lib.settingsPhases(s.ptr, p)
}

func (s *Settings) SuppressHealthCheck(checks HealthCheck) {
	s.lib.settingsSuppressHC(s.ptr, checks)
}

func (s *Settings) RunStart() (*Run, error) {
	r, err := wrap("hegel_run_start", s.lib, func() runT { return s.lib.runStart(s.ptr) }, s.lib.runFree)
	return (*Run)(r), err
}

type Run wrapper[runT]

// Returns nil, nil when there are no more test cases.
func (r *Run) NextTestCase() (*TestCase, error) {
	// TODO: What does cleanup look like?
	tc, err := wrap("hegel_next_test_case", r.lib, func() testCaseT { return r.lib.nextTestCase(r.ptr) }, nil)
	return (*TestCase)(tc), err
}

func (r *Run) RunResult() (*Result, error) {
	res, err := wrap("hegel_run_result", r.lib, func() resultT { return r.lib.runResult(r.ptr) }, nil)
	return (*Result)(res), err
}

type TestCase wrapper[testCaseT]

func (tc *TestCase) Generate(schema []byte) ([]byte, error) {
	var out *byte
	var size uint64
	err := toError("hegel_generate", tc.lib.generate(tc.ptr, slicePtr(schema), uint64(len(schema)), &out, &size))
	if err != nil {
		return nil, err
	}
	return slices.Clone(unsafe.Slice(out, size)), nil
}

func (tc *TestCase) StartSpan(label Label) error {
	return toError("hegel_start_span", tc.lib.startSpan(tc.ptr, label))
}

func (tc *TestCase) StopSpan(discard bool) error {
	return toError("hegel_stop_span", tc.lib.stopSpan(tc.ptr, discard))
}

func (tc *TestCase) NewCollection(min, max uint64) (coll Collection, err error) {
	err = toError("hegel_new_collection", tc.lib.newCollection(tc.ptr, min, max, &coll))
	return
}

func (tc *TestCase) CollectionMore(coll Collection) (more bool, err error) {
	err = toError("hegel_collection_more", tc.lib.collectionMore(tc.ptr, coll, &more))
	return
}

func (tc *TestCase) CollectionReject(coll Collection, why string) error {
	return toError("hegel_collection_reject", tc.lib.collectionReject(tc.ptr, coll, why))
}

func (tc *TestCase) Target(value float64, label string) error {
	return toError("hegel_target", tc.lib.target(tc.ptr, value, label))
}

func (tc *TestCase) MarkComplete(status Status, origin string) error {
	return toError("hegel_mark_complete", tc.lib.markComplete(tc.ptr, status, origin))
}

func (tc *TestCase) IsFinalReplay() bool {
	return tc.lib.isFinalReplay(tc.ptr)
}

type Result wrapper[resultT]

func (r *Result) Passed() bool {
	return r.lib.resultPassed(r.ptr)
}

func (r *Result) FailureCount() uint64 {
	return r.lib.resultFailureCount(r.ptr)
}

func (r *Result) Failure(index uint64) (*Failure, error) {
	f, err := wrap("hegel_run_result_failure", r.lib, func() failureT { return r.lib.resultFailure(r.ptr, index) }, nil)
	return (*Failure)(f), err
}

type Failure wrapper[failureT]

func (f *Failure) PanicMessage() string { // coverage-ignore (diagnostic/panic_message not yet wired into collectFailures)
	return f.lib.failurePanicMsg(f.ptr)
}

func (f *Failure) Diagnostic() string { // coverage-ignore (diagnostic/panic_message not yet wired into collectFailures)
	return f.lib.failureDiagnostic(f.ptr)
}

func (f *Failure) Origin() string {
	return f.lib.failureOrigin(f.ptr)
}

func slicePtr[E any](s []E) *E {
	if len(s) > 0 {
		return &s[0]
	}
	return nil
}
