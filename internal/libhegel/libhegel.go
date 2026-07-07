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
	"time"
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

	// A single test-case handle was used from two threads at once. hegel-go
	// drives each test case from one goroutine, so this is not expected; it is
	// mapped here for completeness of the error space.
	E_CONCURRENT_USE
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

	// The remaining upstream labels (17..30) are emitted internally by the
	// engine's per-draw primitives; callers normally never open these spans
	// themselves. They are mirrored here so the binding's constant values stay
	// aligned with hegel_label_t.
	LABEL_REGEX
	LABEL_EMAIL
	LABEL_URL
	LABEL_DOMAIN
	LABEL_DATE
	LABEL_TIME
	LABEL_DATETIME
	LABEL_UUID
	LABEL_IP_ADDRESS
	LABEL_INTEGER
	LABEL_FLOAT
	LABEL_BOOLEAN
	LABEL_BYTES
	LABEL_STRING

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

type ctxT uintptr       // Equivalent of hegel_context_t
type settingsT uintptr  // Equivalent of hegel_settings_t
type runT uintptr       // Equivalent of hegel_run_t
type testCaseT uintptr  // Equivalent of hegel_test_case_t
type resultT uintptr    // Equivalent of hegel_run_result_t
type failureT uintptr   // Equivalent of hegel_failure_t
type stringGenT uintptr // Equivalent of hegel_string_generator_t

// Date mirrors hegel_date_t: a Gregorian calendar date passed to / returned
// from hegel_generate_date by value.
type Date struct {
	Year  int32
	Month uint8
	Day   uint8
	_     uint16 // pad to match C abi (purego limitation)
}

// ToTime converts the date to a [time.Time] at midnight UTC.
func (d *Date) ToTime() time.Time {
	return time.Date(int(d.Year), time.Month(d.Month), int(d.Day), 0, 0, 0, 0, time.UTC)
}

// Time mirrors hegel_time_t: a time of day passed to / returned from
// hegel_generate_time by value.
type Time struct {
	Hour        uint8
	Minute      uint8
	Second      uint8
	_           uint8 // pad to match C abi (purego limitation)
	Microsecond uint32
}

// Datetime mirrors hegel_datetime_t: a naive datetime (no timezone).
type Datetime struct {
	Date Date
	Time Time
}

// ToTime converts the naive datetime to a [time.Time] in UTC.
func (dt *Datetime) ToTime() time.Time {
	return time.Date(int(dt.Date.Year), time.Month(dt.Date.Month), int(dt.Date.Day),
		int(dt.Time.Hour), int(dt.Time.Minute), int(dt.Time.Second),
		int(dt.Time.Microsecond)*1000, time.UTC)
}

// bytesResult mirrors hegel_generate_bytes_result_t: an engine-allocated byte
// buffer written by hegel_generate_bytes and released by
// hegel_generate_bytes_result_free.
type bytesResult struct {
	data *byte
	len  uint64
}

// stringResult mirrors hegel_generate_string_result_t: an engine-allocated,
// length-delimited UTF-8 buffer (not NUL-terminated) written by
// hegel_generate_string and released by hegel_generate_string_result_free.
type stringResult struct {
	data *byte
	len  uint64
}

// symbols is the loaded libhegel shared library: the dlopen handle plus one Go
// func per C entry point. It is immutable after load and freely shared across
// goroutines; the per-call error-reporting context lives on [Context] instead.
//
// Every fallible C call takes a hegel_context_t as its first argument and
// returns a result code; any value it produces is written through a trailing
// out-parameter. The func signatures below mirror that convention exactly.
type symbols struct {
	handle dlhandle

	ContextNew       func() ctxT
	ContextFree      func(ctxT) Error
	ContextLastError func(ctxT) string

	SettingsNew                       func(ctxT, *settingsT) Error
	SettingsFree                      func(ctxT, settingsT) Error
	SettingsSetMode                   func(ctxT, settingsT, Mode) Error
	SettingsSetBackend                func(ctxT, settingsT, Backend) Error
	SettingsSetTestCases              func(ctxT, settingsT, uint64) Error
	SettingsSetVerbosity              func(ctxT, settingsT, Verbosity) Error
	SettingsSetSeed                   func(ctxT, settingsT, uint64, bool) Error
	SettingsSetDerandomize            func(ctxT, settingsT, bool) Error
	SettingsSetReportMultipleFailures func(ctxT, settingsT, bool) Error
	SettingsSetDatabase               func(ctxT, settingsT, string) Error
	SettingsSetDatabaseKey            func(ctxT, settingsT, string) Error
	SettingsSetPhases                 func(ctxT, settingsT, Phase) Error
	SettingsSetSuppressHealthCheck    func(ctxT, settingsT, HealthCheck) Error

	RunStart     func(ctxT, settingsT, *runT) Error
	RunFree      func(ctxT, runT) Error
	NextTestCase func(ctxT, runT, *testCaseT) Error
	RunResult    func(ctxT, runT, *resultT) Error

	TestCaseFromBlob func(ctxT, settingsT, string, *testCaseT) Error
	TestCaseClone    func(ctxT, testCaseT, *testCaseT) Error
	TestCaseFree     func(ctxT, testCaseT) Error

	StartSpan            func(ctxT, testCaseT, Label) Error
	StopSpan             func(ctxT, testCaseT, bool) Error
	NewCollection        func(ctxT, testCaseT, uint64, uint64, *Collection) Error
	CollectionMore       func(ctxT, testCaseT, Collection, *bool) Error
	CollectionReject     func(ctxT, testCaseT, Collection, string) Error
	NewPool              func(ctxT, testCaseT, *int64) Error
	PoolAdd              func(ctxT, testCaseT, int64, *int64) Error
	PoolGenerate         func(ctxT, testCaseT, int64, bool, *int64) Error
	NewStateMachine      func(ctxT, testCaseT, cStringArrayPtr, uint64, cStringArrayPtr, uint64, *StateMachine) Error
	StateMachineNextRule func(ctxT, testCaseT, StateMachine, *int64) Error
	Target               func(ctxT, testCaseT, float64, string) Error
	MarkComplete         func(ctxT, testCaseT, Status, string) Error

	// Typed primitive draws (0.27.0 replaced the generic schema-driven
	// hegel_generate with these).
	GenerateBoolean    func(ctxT, testCaseT, float64, bool, bool, *bool) Error
	GenerateInteger    func(ctxT, testCaseT, int64, int64, *int64) Error
	GenerateIntegerBig func(ctxT, testCaseT, *byte, uint64, *byte, uint64, outBuf, uint64, *uint64) Error
	GenerateFloat      func(ctxT, testCaseT, uint32, float64, float64, bool, bool, bool, bool, float64, *float64) Error
	GenerateBytes      func(ctxT, testCaseT, uint64, uint64, *bytesResult) Error
	GenerateBytesFree  func(ctxT, *bytesResult) Error
	GenerateString     func(ctxT, testCaseT, stringGenT, *stringResult) Error
	GenerateStringFree func(ctxT, *stringResult) Error
	GenerateDate       func(ctxT, testCaseT, Date, Date, *Date) Error
	GenerateTime       func(ctxT, testCaseT, Time, Time, *Time) Error
	GenerateDatetime   func(ctxT, testCaseT, Datetime, Datetime, *Datetime) Error
	GenerateUUID       func(ctxT, testCaseT, uint8, bool, outBuf) Error
	GenerateIPv4       func(ctxT, testCaseT, outBuf) Error
	GenerateIPv6       func(ctxT, testCaseT, outBuf) Error

	// String-generator constructors (build the alphabet-and-shape spec passed
	// to hegel_generate_string) and their shared free.
	StringGeneratorText   func(ctxT, uint64, uint64, string, uint32, uint32, cStringArrayPtr, uint64, cStringArrayPtr, uint64, *byte, uint64, *byte, uint64, *stringGenT) Error
	StringGeneratorRegex  func(ctxT, string, bool, stringGenT, *stringGenT) Error
	StringGeneratorEmail  func(ctxT, *stringGenT) Error
	StringGeneratorURL    func(ctxT, *stringGenT) Error
	StringGeneratorDomain func(ctxT, uint64, *stringGenT) Error
	StringGeneratorFree   func(ctxT, stringGenT) Error

	ResultFree         func(ctxT, resultT) Error
	ResultStatus       func(ctxT, resultT, *RunStatus) Error
	ResultError        func(ctxT, resultT, **byte) Error
	ResultFailureCount func(ctxT, resultT, *uint64) Error
	ResultFailure      func(ctxT, resultT, uint64, *failureT) Error

	FailureFree             func(ctxT, failureT) Error
	FailureOrigin           func(ctxT, failureT, **byte) Error
	FailureReproductionBlob func(ctxT, failureT, **byte) Error

	Version func(ctxT, **byte) Error
}

type Context pointer[ctxT]

// NewContext allocates a fresh error-reporting context over the process-wide
// loaded library, loading the library on first use (panicking if it cannot be
// found).
func NewContext() *Context {
	syms := globalSymbols()
	ctx := &Context{syms, syms.ContextNew()}
	runtime.AddCleanup(ctx, func(ctx ctxT) { syms.ContextFree(ctx) }, ctx.raw)
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
		if msg := c.syms.ContextLastError(c.raw); msg != "" {
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

// versionString reports the loaded library's version (hegel_version).
func (s *symbols) versionString() string {
	var out *byte
	_ = s.Version(0, &out)
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
		{"hegel_context_new", &syms.ContextNew},
		{"hegel_context_free", &syms.ContextFree},
		{"hegel_context_last_error", &syms.ContextLastError},

		{"hegel_settings_new", &syms.SettingsNew},
		{"hegel_settings_free", &syms.SettingsFree},
		{"hegel_settings_set_mode", &syms.SettingsSetMode},
		{"hegel_settings_set_backend", &syms.SettingsSetBackend},
		{"hegel_settings_set_test_cases", &syms.SettingsSetTestCases},
		{"hegel_settings_set_verbosity", &syms.SettingsSetVerbosity},
		{"hegel_settings_set_seed", &syms.SettingsSetSeed},
		{"hegel_settings_set_derandomize", &syms.SettingsSetDerandomize},
		{"hegel_settings_set_report_multiple_failures", &syms.SettingsSetReportMultipleFailures},
		{"hegel_settings_set_database", &syms.SettingsSetDatabase},
		{"hegel_settings_set_database_key", &syms.SettingsSetDatabaseKey},
		{"hegel_settings_set_phases", &syms.SettingsSetPhases},
		{"hegel_settings_set_suppress_health_check", &syms.SettingsSetSuppressHealthCheck},

		{"hegel_run_start", &syms.RunStart},
		{"hegel_next_test_case", &syms.NextTestCase},
		{"hegel_run_result", &syms.RunResult},
		{"hegel_run_free", &syms.RunFree},

		{"hegel_test_case_from_blob", &syms.TestCaseFromBlob},
		{"hegel_test_case_clone", &syms.TestCaseClone},
		{"hegel_test_case_free", &syms.TestCaseFree},

		{"hegel_start_span", &syms.StartSpan},
		{"hegel_stop_span", &syms.StopSpan},
		{"hegel_new_collection", &syms.NewCollection},
		{"hegel_collection_more", &syms.CollectionMore},
		{"hegel_collection_reject", &syms.CollectionReject},
		{"hegel_new_pool", &syms.NewPool},
		{"hegel_pool_add", &syms.PoolAdd},
		{"hegel_pool_generate", &syms.PoolGenerate},
		{"hegel_new_state_machine", &syms.NewStateMachine},
		{"hegel_state_machine_next_rule", &syms.StateMachineNextRule},
		{"hegel_target", &syms.Target},
		{"hegel_mark_complete", &syms.MarkComplete},

		{"hegel_generate_boolean", &syms.GenerateBoolean},
		{"hegel_generate_integer", &syms.GenerateInteger},
		{"hegel_generate_integer_big", &syms.GenerateIntegerBig},
		{"hegel_generate_float", &syms.GenerateFloat},
		{"hegel_generate_bytes", &syms.GenerateBytes},
		{"hegel_generate_bytes_result_free", &syms.GenerateBytesFree},
		{"hegel_generate_string", &syms.GenerateString},
		{"hegel_generate_string_result_free", &syms.GenerateStringFree},
		{"hegel_generate_date", &syms.GenerateDate},
		{"hegel_generate_time", &syms.GenerateTime},
		{"hegel_generate_datetime", &syms.GenerateDatetime},
		{"hegel_generate_uuid", &syms.GenerateUUID},
		{"hegel_generate_ipv4", &syms.GenerateIPv4},
		{"hegel_generate_ipv6", &syms.GenerateIPv6},

		{"hegel_string_generator_text", &syms.StringGeneratorText},
		{"hegel_string_generator_regex", &syms.StringGeneratorRegex},
		{"hegel_string_generator_email", &syms.StringGeneratorEmail},
		{"hegel_string_generator_url", &syms.StringGeneratorURL},
		{"hegel_string_generator_domain", &syms.StringGeneratorDomain},
		{"hegel_string_generator_free", &syms.StringGeneratorFree},

		{"hegel_run_result_free", &syms.ResultFree},
		{"hegel_run_result_status", &syms.ResultStatus},
		{"hegel_run_result_error", &syms.ResultError},
		{"hegel_run_result_failure_count", &syms.ResultFailureCount},
		{"hegel_run_result_failure", &syms.ResultFailure},
		{"hegel_failure_free", &syms.FailureFree},
		{"hegel_failure_origin", &syms.FailureOrigin},
		{"hegel_failure_reproduction_blob", &syms.FailureReproductionBlob},

		{"hegel_version", &syms.Version},
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
		return c.syms.SettingsNew(ctx, raw)
	}, c.syms.SettingsFree)
	return (*Settings)(ptr)
}

func (s *Settings) Mode(ctx *Context, m Mode) error {
	return ctx.invoke("hegel_settings_set_mode", func(ctx ctxT) Error {
		return s.syms.SettingsSetMode(ctx, s.raw, m)
	})
}

// Backend selects the engine's randomness backend. See [Backend].
func (s *Settings) Backend(ctx *Context, b Backend) error {
	return ctx.invoke("hegel_settings_set_backend", func(ctx ctxT) Error {
		return s.syms.SettingsSetBackend(ctx, s.raw, b)
	})
}

func (s *Settings) TestCases(ctx *Context, n uint64) error {
	return ctx.invoke("hegel_settings_set_test_cases", func(ctx ctxT) Error {
		return s.syms.SettingsSetTestCases(ctx, s.raw, n)
	})
}

func (s *Settings) Verbosity(ctx *Context, v Verbosity) error {
	return ctx.invoke("hegel_settings_set_verbosity", func(ctx ctxT) Error {
		return s.syms.SettingsSetVerbosity(ctx, s.raw, v)
	})
}

func (s *Settings) Seed(ctx *Context, seed uint64, hasSeed bool) error {
	return ctx.invoke("hegel_settings_set_seed", func(ctx ctxT) Error {
		return s.syms.SettingsSetSeed(ctx, s.raw, seed, hasSeed)
	})
}

func (s *Settings) Derandomize(ctx *Context, on bool) error {
	return ctx.invoke("hegel_settings_set_derandomize", func(ctx ctxT) Error {
		return s.syms.SettingsSetDerandomize(ctx, s.raw, on)
	})
}

func (s *Settings) ReportMultipleFailures(ctx *Context, yes bool) error {
	return ctx.invoke("hegel_settings_set_report_multiple_failures", func(ctx ctxT) Error {
		return s.syms.SettingsSetReportMultipleFailures(ctx, s.raw, yes)
	})
}

func (s *Settings) Database(ctx *Context, path string) error {
	return ctx.invoke("hegel_settings_set_database", func(ctx ctxT) Error {
		return s.syms.SettingsSetDatabase(ctx, s.raw, path)
	})
}

func (s *Settings) DatabaseKey(ctx *Context, key string) error {
	return ctx.invoke("hegel_settings_set_database_key", func(ctx ctxT) Error {
		return s.syms.SettingsSetDatabaseKey(ctx, s.raw, key)
	})
}

func (s *Settings) Phases(ctx *Context, p Phase) error {
	return ctx.invoke("hegel_settings_set_phases", func(ctx ctxT) Error {
		return s.syms.SettingsSetPhases(ctx, s.raw, p)
	})
}

func (s *Settings) SuppressHealthCheck(ctx *Context, checks HealthCheck) error {
	return ctx.invoke("hegel_settings_set_suppress_health_check", func(ctx ctxT) Error {
		return s.syms.SettingsSetSuppressHealthCheck(ctx, s.raw, checks)
	})
}

func (s *Settings) RunStart(ctx *Context) (*Run, error) {
	ptr, err := allocate(ctx, "hegel_run_start", func(ctx ctxT, raw *runT) Error {
		e := s.syms.RunStart(ctx, s.raw, raw)
		runtime.KeepAlive(s)
		return e
	}, s.syms.RunFree)
	return (*Run)(ptr), err
}

// TestCaseFromBlob builds a standalone test case that replays the example
// encoded in a base64 failure blob (from [Failure.ReproductionBlob]). Unlike
// test cases from [Run.NextTestCase], the returned handle is owned by the
// caller and is freed automatically via the GC. A rejected blob surfaces as a
// nil test case and a non-nil error.
func (s *Settings) TestCaseFromBlob(ctx *Context, blob string) (tc *TestCase, err error) {
	ptr, err := allocate(ctx, "hegel_test_case_from_blob", func(ctx ctxT, raw *testCaseT) Error {
		e := s.syms.TestCaseFromBlob(ctx, s.raw, blob, raw)
		runtime.KeepAlive(s)
		return e
	}, s.syms.TestCaseFree)
	if ptr == nil {
		return nil, err
	}
	return &TestCase{pointer: ptr}, err
}

type Run pointer[runT]

// NextTestCase blocks until the engine produces the next test case. The
// returned handle is owned by the caller and freed automatically via the GC;
// the run keeps its own internal reference, so freeing the handle never
// disturbs the run. Returns nil, nil when there are no more test cases.
func (r *Run) NextTestCase(ctx *Context) (*TestCase, error) {
	ptr, err := allocate(ctx, "hegel_next_test_case", func(ctx ctxT, raw *testCaseT) Error {
		e := r.syms.NextTestCase(ctx, r.raw, raw)
		runtime.KeepAlive(r)
		return e
	}, r.syms.TestCaseFree)
	if ptr == nil {
		return nil, err
	}
	return &TestCase{pointer: ptr}, err
}

// RunResult reads the aggregated outcome of a finished run. The returned value
// is a caller-owned snapshot, independent of the run — it stays valid after the
// run is freed — and is released automatically via the GC.
func (r *Run) RunResult(ctx *Context) (*Result, error) {
	ptr, err := allocate(ctx, "hegel_run_result", func(ctx ctxT, raw *resultT) Error {
		e := r.syms.RunResult(ctx, r.raw, raw)
		runtime.KeepAlive(r)
		return e
	}, r.syms.ResultFree)
	return &Result{pointer: ptr}, err
}

// TestCase wraps a hegel_test_case_t. The out* fields are reusable scratch for
// the hot per-draw calls below: keeping them on this (heap-allocated) struct
// lets each call pass a pointer into long-lived storage instead of allocating a
// fresh temporary on every draw (a pointer handed to purego's reflection-based
// call path necessarily escapes to the heap).
type TestCase struct {
	*pointer[testCaseT]

	outBool      bool
	outInt       int64
	outFloat     float64
	outColl      Collection
	outSM        StateMachine
	outDate      Date
	outTime      Time
	outDatetime  Datetime
	outBytesRes  bytesResult
	outStringRes stringResult
	outLen       uint64
	// outFixed backs the fixed-width byte outputs (UUID: 16 bytes, IPv6: 16,
	// IPv4: 4) and the integer_big result buffer, so a draw passes a pointer
	// into long-lived storage instead of allocating per call.
	outFixed [16]byte
}

// Clone produces an independent handle onto the same underlying test case, so
// it can be driven from another goroutine. The clone is a view, not a copy: it
// draws from the same data source and shares completion state (marking any
// handle in the family complete marks them all). Each handle has its own lock,
// so clones may draw concurrently where a single shared handle would report
// [E_CONCURRENT_USE]. The returned handle is owned by the caller and freed
// automatically via the GC; the underlying test case stays alive until every
// handle in the family is freed.
func (tc *TestCase) Clone(ctx *Context) (*TestCase, error) {
	ptr, err := allocate(ctx, "hegel_test_case_clone", func(ctx ctxT, raw *testCaseT) Error {
		e := tc.syms.TestCaseClone(ctx, tc.raw, raw)
		runtime.KeepAlive(tc)
		return e
	}, tc.syms.TestCaseFree)
	if ptr == nil {
		return nil, err
	}
	return &TestCase{pointer: ptr}, err
}

// GenerateBoolean draws a single boolean that is true with probability p. When
// hasForced is true the result is forced to forced (consuming no entropy and
// not shrunk).
func (tc *TestCase) GenerateBoolean(ctx *Context, p float64, forced, hasForced bool) (bool, error) {
	err := ctx.invoke("hegel_generate_boolean", func(ctx ctxT) Error {
		return tc.syms.GenerateBoolean(ctx, tc.raw, p, forced, hasForced, &tc.outBool)
	})
	return tc.outBool, err
}

// GenerateInteger draws an integer in [minValue, maxValue]. Both bounds must
// fit in int64; for wider bounds use [TestCase.GenerateIntegerBig].
func (tc *TestCase) GenerateInteger(ctx *Context, minValue, maxValue int64) (int64, error) {
	err := ctx.invoke("hegel_generate_integer", func(ctx ctxT) Error {
		return tc.syms.GenerateInteger(ctx, tc.raw, minValue, maxValue, &tc.outInt)
	})
	return tc.outInt, err
}

// BigInt is an arbitrary-precision integer encoded as a two's-complement
// little-endian byte buffer — the wire format hegel_generate_integer_big
// consumes for its bounds and produces for its result.
type BigInt []byte

type integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

// NewBigInt encodes v as a minimal two's-complement little-endian [BigInt] — the
// format hegel_generate_integer_big expects for its bounds. It handles both
// signed and unsigned types across their full range: negative values are
// encoded in two's complement (with a leading 0xff sign byte where needed), and
// a non-negative value whose top bit would otherwise read as negative gets a
// trailing 0x00 sign byte (so, e.g., an unsigned value above math.MaxInt64 stays
// non-negative on the wire).
func NewBigInt[T integer](v T) BigInt {
	le := make(BigInt, 0, 9) // up to 8 magnitude bytes plus a sign byte
	// Widen to 64 bits so the byte-at-a-time shift is well-defined even for an
	// 8-bit T (`x >>= 8` on a possibly-8-bit value trips go vet). A signed T
	// sign-extends into int64 and shifts arithmetically; an unsigned T stays
	// non-negative in uint64 and shifts logically.
	if ^T(0) < T(0) { // signed
		x := int64(v)
		for {
			b := byte(x)
			x >>= 8
			le = append(le, b)
			// Stop once the rest is pure sign extension and the last byte
			// already carries the correct sign.
			if (x == 0 && b&0x80 == 0) || (x == -1 && b&0x80 != 0) {
				break
			}
		}
		return le
	}
	x := uint64(v)
	for {
		b := byte(x)
		x >>= 8
		le = append(le, b)
		// A non-negative value ends when nothing remains and the top bit is
		// clear; otherwise a trailing 0x00 sign byte is emitted next.
		if x == 0 && b&0x80 == 0 {
			break
		}
	}
	return le
}

// Uint64 decodes a [BigInt] holding a non-negative value into a uint64. Bytes
// beyond the low eight must be zero (non-negative sign extension); a non-zero
// high byte means the value overflows a uint64, and Uint64 panics rather than
// silently truncating it.
func (b BigInt) Uint64() uint64 {
	var v uint64
	for i, byteVal := range b {
		if i >= 8 {
			if byteVal != 0 {
				panic(fmt.Sprintf("BigInt %v overflows uint64", []byte(b)))
			}
			continue
		}
		v |= uint64(byteVal) << (8 * i)
	}
	return v
}

// GenerateIntegerBig draws an arbitrary-precision integer in [minValue,
// maxValue]. Bounds and result are two's-complement little-endian signed byte
// buffers. Returns the drawn value's minimal little-endian bytes.
func (tc *TestCase) GenerateIntegerBig(ctx *Context, minValue, maxValue BigInt) (BigInt, error) {
	err := ctx.invoke("hegel_generate_integer_big", func(ctx ctxT) Error {
		return tc.syms.GenerateIntegerBig(ctx, tc.raw,
			slicePtr(minValue), uint64(len(minValue)),
			slicePtr(maxValue), uint64(len(maxValue)),
			outBuf(&tc.outFixed[0]), uint64(len(tc.outFixed)), &tc.outLen)
	})
	if err != nil {
		return nil, err
	}
	return slices.Clone(BigInt(tc.outFixed[:tc.outLen])), nil
}

// GenerateFloat draws a float of the given width (32 or 64) in [minValue,
// maxValue]. See hegel_generate_float for the meaning of each flag.
func (tc *TestCase) GenerateFloat(ctx *Context, width uint32, minValue, maxValue float64, allowNaN, allowInfinity, excludeMin, excludeMax bool, smallestNonzero float64) (float64, error) {
	err := ctx.invoke("hegel_generate_float", func(ctx ctxT) Error {
		return tc.syms.GenerateFloat(ctx, tc.raw, width, minValue, maxValue, allowNaN, allowInfinity, excludeMin, excludeMax, smallestNonzero, &tc.outFloat)
	})
	return tc.outFloat, err
}

// GenerateBytes draws a byte string with length in [minSize, maxSize].
func (tc *TestCase) GenerateBytes(ctx *Context, minSize, maxSize uint64) ([]byte, error) {
	err := ctx.invoke("hegel_generate_bytes", func(ctx ctxT) Error {
		return tc.syms.GenerateBytes(ctx, tc.raw, minSize, maxSize, &tc.outBytesRes)
	})
	if err != nil {
		return nil, err
	}
	out := slices.Clone(unsafe.Slice(tc.outBytesRes.data, tc.outBytesRes.len))
	_ = tc.syms.GenerateBytesFree(0, &tc.outBytesRes)
	return out, nil
}

// GenerateString draws a string described by gen (built with a
// [Context] string-generator constructor).
func (tc *TestCase) GenerateString(ctx *Context, gen *StringGenerator) (string, error) {
	err := ctx.invoke("hegel_generate_string", func(ctx ctxT) Error {
		e := tc.syms.GenerateString(ctx, tc.raw, gen.raw, &tc.outStringRes)
		runtime.KeepAlive(gen)
		return e
	})
	if err != nil {
		return "", err
	}
	// string(...) copies the length-delimited (not NUL-terminated) UTF-8 buffer.
	out := string(unsafe.Slice(tc.outStringRes.data, tc.outStringRes.len))
	_ = tc.syms.GenerateStringFree(0, &tc.outStringRes)
	return out, nil
}

// GenerateDate draws a Gregorian calendar date in [minValue, maxValue].
func (tc *TestCase) GenerateDate(ctx *Context, minValue, maxValue Date) (Date, error) {
	err := ctx.invoke("hegel_generate_date", func(ctx ctxT) Error {
		return tc.syms.GenerateDate(ctx, tc.raw, minValue, maxValue, &tc.outDate)
	})
	return tc.outDate, err
}

// GenerateTime draws a time of day in [minValue, maxValue].
func (tc *TestCase) GenerateTime(ctx *Context, minValue, maxValue Time) (Time, error) {
	err := ctx.invoke("hegel_generate_time", func(ctx ctxT) Error {
		return tc.syms.GenerateTime(ctx, tc.raw, minValue, maxValue, &tc.outTime)
	})
	return tc.outTime, err
}

// GenerateDatetime draws a naive datetime in [minValue, maxValue].
func (tc *TestCase) GenerateDatetime(ctx *Context, minValue, maxValue Datetime) (Datetime, error) {
	err := ctx.invoke("hegel_generate_datetime", func(ctx ctxT) Error {
		return tc.syms.GenerateDatetime(ctx, tc.raw, minValue, maxValue, &tc.outDatetime)
	})
	return tc.outDatetime, err
}

// GenerateUUID draws a UUID as 16 big-endian bytes. When hasVersion is set the
// RFC 4122 version nibble is forced to version.
func (tc *TestCase) GenerateUUID(ctx *Context, version uint8, hasVersion bool) ([16]byte, error) {
	err := ctx.invoke("hegel_generate_uuid", func(ctx ctxT) Error {
		return tc.syms.GenerateUUID(ctx, tc.raw, version, hasVersion, outBuf(&tc.outFixed[0]))
	})
	return tc.outFixed, err
}

// GenerateIPv4 draws an IPv4 address as 4 network-order bytes.
func (tc *TestCase) GenerateIPv4(ctx *Context) ([4]byte, error) {
	err := ctx.invoke("hegel_generate_ipv4", func(ctx ctxT) Error {
		return tc.syms.GenerateIPv4(ctx, tc.raw, outBuf(&tc.outFixed[0]))
	})
	return [4]byte(tc.outFixed[:4]), err
}

// GenerateIPv6 draws an IPv6 address as 16 network-order bytes.
func (tc *TestCase) GenerateIPv6(ctx *Context) ([16]byte, error) {
	err := ctx.invoke("hegel_generate_ipv6", func(ctx ctxT) Error {
		return tc.syms.GenerateIPv6(ctx, tc.raw, outBuf(&tc.outFixed[0]))
	})
	return tc.outFixed, err
}

func (tc *TestCase) StartSpan(ctx *Context, label Label) error {
	return ctx.invoke("hegel_start_span", func(ctx ctxT) Error {
		return tc.syms.StartSpan(ctx, tc.raw, label)
	})
}

func (tc *TestCase) StopSpan(ctx *Context, discard bool) error {
	return ctx.invoke("hegel_stop_span", func(ctx ctxT) Error {
		return tc.syms.StopSpan(ctx, tc.raw, discard)
	})
}

func (tc *TestCase) NewCollection(ctx *Context, min, max uint64) (Collection, error) {
	err := ctx.invoke("hegel_new_collection", func(ctx ctxT) Error {
		return tc.syms.NewCollection(ctx, tc.raw, min, max, &tc.outColl)
	})
	return tc.outColl, err
}

func (tc *TestCase) CollectionMore(ctx *Context, coll Collection) (bool, error) {
	err := ctx.invoke("hegel_collection_more", func(ctx ctxT) Error {
		return tc.syms.CollectionMore(ctx, tc.raw, coll, &tc.outBool)
	})
	return tc.outBool, err
}

func (tc *TestCase) CollectionReject(ctx *Context, coll Collection, why string) error {
	return ctx.invoke("hegel_collection_reject", func(ctx ctxT) Error {
		return tc.syms.CollectionReject(ctx, tc.raw, coll, why)
	})
}

// NewPool creates an engine-managed variable pool for stateful testing.
func (tc *TestCase) NewPool(ctx *Context) (int64, error) {
	err := ctx.invoke("hegel_new_pool", func(ctx ctxT) Error {
		return tc.syms.NewPool(ctx, tc.raw, &tc.outInt)
	})
	return tc.outInt, err
}

// PoolAdd registers a new variable in the pool, returning the engine-assigned id.
func (tc *TestCase) PoolAdd(ctx *Context, pool int64) (int64, error) {
	err := ctx.invoke("hegel_pool_add", func(ctx ctxT) Error {
		return tc.syms.PoolAdd(ctx, tc.raw, pool, &tc.outInt)
	})
	return tc.outInt, err
}

// PoolGenerate draws a variable id from the pool, letting the engine choose and
// shrink which previously-added variable to reuse. When consume is true the
// drawn variable is removed from the pool.
func (tc *TestCase) PoolGenerate(ctx *Context, pool int64, consume bool) (int64, error) {
	err := ctx.invoke("hegel_pool_generate", func(ctx ctxT) Error {
		return tc.syms.PoolGenerate(ctx, tc.raw, pool, consume, &tc.outInt)
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
		return tc.syms.NewStateMachine(
			ctx, tc.raw,
			cStringArrayPtr(slicePtr(rules)), uint64(len(ruleNames)),
			cStringArrayPtr(slicePtr(invariants)), uint64(len(invariantNames)),
			&tc.outSM,
		)
	})
	return tc.outSM, err
}

// StateMachineNextRule draws the index of the next rule to run, in
// [0, num_rules), letting the engine choose and shrink the rule sequence.
func (tc *TestCase) StateMachineNextRule(ctx *Context, machine StateMachine) (int64, error) {
	err := ctx.invoke("hegel_state_machine_next_rule", func(ctx ctxT) Error {
		return tc.syms.StateMachineNextRule(ctx, tc.raw, machine, &tc.outInt)
	})
	return tc.outInt, err
}

func (tc *TestCase) Target(ctx *Context, value float64, label string) error {
	return ctx.invoke("hegel_target", func(ctx ctxT) Error {
		return tc.syms.Target(ctx, tc.raw, value, label)
	})
}

func (tc *TestCase) MarkComplete(ctx *Context, status Status, origin string) error {
	return ctx.invoke("hegel_mark_complete", func(ctx ctxT) Error {
		return tc.syms.MarkComplete(ctx, tc.raw, status, origin)
	})
}

// Result wraps a hegel_run_result_t. The out* fields are reusable out-parameter
// scratch (see [TestCase] for the rationale).
//
// The handle is a caller-owned snapshot, independent of the [Run] it came from:
// it stays valid after the run is freed and is released by its own GC cleanup
// (hegel_run_result_free).
type Result struct {
	*pointer[resultT]

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
		return r.syms.ResultStatus(ctx, r.raw, &r.outStatus)
	})
	return r.outStatus
}

// ErrorMessage returns the run-level error message when [Result.Status] is
// [RUN_STATUS_ERROR], or the empty string otherwise.
func (r *Result) ErrorMessage(ctx *Context) string {
	_ = ctx.invoke("hegel_run_result_error", func(ctx ctxT) Error {
		return r.syms.ResultError(ctx, r.raw, &r.outBytes)
	})
	return goString(r.outBytes)
}

func (r *Result) FailureCount(ctx *Context) uint64 {
	_ = ctx.invoke("hegel_run_result_failure_count", func(ctx ctxT) Error {
		return r.syms.ResultFailureCount(ctx, r.raw, &r.outCount)
	})
	return r.outCount
}

func (r *Result) Failure(ctx *Context, index uint64) (*Failure, error) {
	ptr, err := allocate(ctx, "hegel_run_result_failure", func(ctx ctxT, raw *failureT) Error {
		e := r.syms.ResultFailure(ctx, r.raw, index, raw)
		runtime.KeepAlive(r)
		return e
	}, r.syms.FailureFree)
	return &Failure{pointer: ptr}, err
}

// Failure wraps a hegel_failure_t. outBytes is reusable out-parameter scratch
// (see [TestCase] for the rationale).
//
// Like [Result], the handle is a caller-owned snapshot, independent of the run
// it came from: it stays valid after the run is freed and is released by its
// own GC cleanup (hegel_failure_free).
type Failure struct {
	*pointer[failureT]

	outBytes *byte
}

// Origin returns the failure's origin string — the stable identifier the
// shrinker used to group probes for this bug.
func (f *Failure) Origin(ctx *Context) string {
	_ = ctx.invoke("hegel_failure_origin", func(ctx ctxT) Error {
		return f.syms.FailureOrigin(ctx, f.raw, &f.outBytes)
	})
	return goString(f.outBytes)
}

// ReproductionBlob returns the failure's base64 reproduction blob, suitable
// for deterministic replay via [Settings.TestCaseFromBlob], or the empty
// string if the engine produced no blob for this failure.
func (f *Failure) ReproductionBlob(ctx *Context) string {
	_ = ctx.invoke("hegel_failure_reproduction_blob", func(ctx ctxT) Error {
		return f.syms.FailureReproductionBlob(ctx, f.raw, &f.outBytes)
	})
	return goString(f.outBytes)
}

// StringGenerator wraps a hegel_string_generator_t: the immutable
// alphabet-and-shape spec passed to [TestCase.GenerateString]. Build one with a
// Context constructor; it is released automatically via the GC.
type StringGenerator pointer[stringGenT]

// StringGeneratorText builds a text string generator. A nil categories /
// excludeCategories slice means "no restriction" (a NULL array); a non-nil
// (possibly empty) slice is passed through, so an empty non-nil slice requests
// an empty category set. includeChars / excludeChars are optional UTF-8
// character buffers (nil for none). codec must be non-empty ("utf-8" selects
// all of Unicode, matching the C NULL default).
func (c *Context) StringGeneratorText(minSize, maxSize uint64, codec string, minCodepoint, maxCodepoint uint32, categories, excludeCategories []string, includeChars, excludeChars *string) (*StringGenerator, error) {
	catsPtr, catsLen, err := cStringArrayArg(categories)
	if err != nil {
		return nil, fmt.Errorf("hegel_string_generator_text: categories: %w", err)
	}
	exclPtr, exclLen, err := cStringArrayArg(excludeCategories)
	if err != nil {
		return nil, fmt.Errorf("hegel_string_generator_text: exclude_categories: %w", err)
	}
	incPtr, incLen := cString(includeChars)
	excPtr, excLen := cString(excludeChars)
	ptr, err := allocate(c, "hegel_string_generator_text", func(ctx ctxT, raw *stringGenT) Error {
		return c.syms.StringGeneratorText(ctx, minSize, maxSize, codec, minCodepoint, maxCodepoint,
			catsPtr, catsLen, exclPtr, exclLen, incPtr, incLen, excPtr, excLen, raw)
	}, c.syms.StringGeneratorFree)
	if ptr == nil {
		return nil, err
	}
	return (*StringGenerator)(ptr), err
}

// StringGeneratorRegex builds a regex string generator matching pattern
// (Python-re syntax). When fullmatch is true the whole string matches.
func (c *Context) StringGeneratorRegex(pattern string, fullmatch bool) (*StringGenerator, error) {
	ptr, err := allocate(c, "hegel_string_generator_regex", func(ctx ctxT, raw *stringGenT) Error {
		return c.syms.StringGeneratorRegex(ctx, pattern, fullmatch, 0, raw)
	}, c.syms.StringGeneratorFree)
	if ptr == nil {
		return nil, err
	}
	return (*StringGenerator)(ptr), err
}

// StringGeneratorEmail builds an email-address string generator.
func (c *Context) StringGeneratorEmail() (*StringGenerator, error) {
	ptr, err := allocate(c, "hegel_string_generator_email", func(ctx ctxT, raw *stringGenT) Error {
		return c.syms.StringGeneratorEmail(ctx, raw)
	}, c.syms.StringGeneratorFree)
	if ptr == nil {
		return nil, err
	}
	return (*StringGenerator)(ptr), err
}

// StringGeneratorURL builds a URL string generator.
func (c *Context) StringGeneratorURL() (*StringGenerator, error) {
	ptr, err := allocate(c, "hegel_string_generator_url", func(ctx ctxT, raw *stringGenT) Error {
		return c.syms.StringGeneratorURL(ctx, raw)
	}, c.syms.StringGeneratorFree)
	if ptr == nil {
		return nil, err
	}
	return (*StringGenerator)(ptr), err
}

// StringGeneratorDomain builds a domain-name string generator with total length
// at most maxLength (4..=255).
func (c *Context) StringGeneratorDomain(maxLength uint64) (*StringGenerator, error) {
	ptr, err := allocate(c, "hegel_string_generator_domain", func(ctx ctxT, raw *stringGenT) Error {
		return c.syms.StringGeneratorDomain(ctx, maxLength, raw)
	}, c.syms.StringGeneratorFree)
	if ptr == nil {
		return nil, err
	}
	return (*StringGenerator)(ptr), err
}

// cString returns the (pointer, byte length) pair for an optional
// length-delimited byte buffer argument (a `const uint8_t *` plus its size_t
// length). It is the inverse of goString: goString copies a C buffer into a Go
// string, cString hands a Go string's bytes to C. A nil (or empty) string means
// "absent" and yields a NULL pointer.
func cString(s *string) (ptr *byte, n uint64) {
	if s == nil || len(*s) == 0 {
		return nil, 0
	}
	return unsafe.StringData(*s), uint64(len(*s))
}

// cStringArrayArg builds the (pointer, length) pair for a `const char *const *`
// argument. The returned pointer is a typed cStringArrayPtr (**byte), so the GC
// keeps both the pointer array and the NUL-terminated buffers it references
// reachable across the FFI call without any separate keepalive value.
//
// Returns an error if any of the strings contain a NUL byte.
func cStringArrayArg(ss []string) (ptr cStringArrayPtr, n uint64, err error) {
	if ss == nil {
		return nil, 0, nil
	}
	if len(ss) == 0 {
		// A non-nil but empty slice: a scratch element gives a non-NULL base
		// address with length 0, distinguishing "the empty set" from "absent".
		ptrs := []*byte{nil}
		return cStringArrayPtr(&ptrs[0]), 0, nil
	}
	ptrs := make([]*byte, len(ss))
	for i, s := range ss {
		if strings.IndexByte(s, 0) >= 0 {
			return nil, 0, fmt.Errorf("string %q contains an interior NUL byte", s)
		}
		buf := append([]byte(s), 0) // NUL-terminate for C
		ptrs[i] = &buf[0]
	}
	return cStringArrayPtr(&ptrs[0]), uint64(len(ss)), nil
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

// cStringArrayPtr is the type of a C `const char *const *` argument: a pointer
// to the first element of a NUL-terminated-string pointer array built by
// [cStringArray]. It is a defined type rather than a bare **byte so the
// signature-driven test stub ([Stub]) can distinguish these array *inputs* from
// the **byte *output* parameters other symbols use. purego marshals it
// identically to **byte — it dispatches on reflect.Kind (still Ptr), not on
// type identity.
type cStringArrayPtr **byte

// outBuf is the type of a fixed-width `uint8*` output buffer (the integer_big
// result, and the UUID / IPv4 / IPv6 byte outputs). It is a defined type rather
// than a bare *byte so [Stub] can tell these output buffers apart from the
// *byte *input* buffers (integer_big bounds, string-generator character sets),
// which the stub must leave untouched. purego marshals it identically to *byte.
type outBuf *byte

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
