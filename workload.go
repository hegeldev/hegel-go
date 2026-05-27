package hegel

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// Seams so tests can observe the exit code from [Workload] and redirect its
// output without terminating the test process or polluting the test log.
var (
	workloadExit             = os.Exit
	workloadStdout io.Writer = os.Stdout
	workloadStderr io.Writer = os.Stderr
)

// Workload runs a property test as a standalone CLI binary, parsing flags
// from [os.Args].
//
// Runs for an unbounded amount of time unless overridden via an option or flag.
//
// opts are overridden by flags.
func Workload(fn func(TestCase), opts ...Option) {
	err := workload(os.Args, workloadStdout, workloadStderr, fn, opts)
	if err == nil {
		return
	}
	fmt.Fprintln(workloadStderr, "Error:", err)
	workloadExit(1)
}

// workload is the testable core of [Workload]. Note output (interesting cases,
// single-test-case mode) goes to stdout; flag-package output (parse errors,
// usage, --help) goes to stderr. Returns nil on success (including --help) or
// a non-nil error for flag-parse problems and property-test failures.
func workload(args []string, stdout io.Writer, stderr io.Writer, fn func(TestCase), opts []Option) error {
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)

	fs.Var(&testCasesFlag{}, "test-cases", "maximum `number` of test cases to run")
	fs.Var(&derandomizeFlag{}, "derandomize", "use a fixed seed for reproducible runs")
	fs.Var(&databaseFlag{}, "database", "example database `path`; empty string disables persistence")
	fs.Var(&suppressHealthCheckFlag{}, "suppress-health-check", "`name` of health check to suppress (repeatable)")
	fs.Var(&singleTestCaseFlag{}, "single-test-case", "run exactly one test case with no shrinking, replay, or example database")

	if err := fs.Parse(args[1:]); errors.Is(err, flag.ErrHelp) {
		return nil
	} else if err != nil {
		return err
	}

	allOpts := []Option{
		WithTestCases(math.MaxInt),
	}
	allOpts = append(allOpts, opts...)
	fs.Visit(func(f *flag.Flag) {
		if ov, ok := f.Value.(optionValue); ok {
			allOpts = append(allOpts, ov.Option())
		}
	})
	allOpts = append(allOpts, withOutput(stdout))
	return run(fn, allOpts...)
}

type optionValue interface {
	flag.Value
	Option() Option
}

// testCasesFlag wires --test-cases to [WithTestCases].
type testCasesFlag struct {
	n int
}

func (f *testCasesFlag) String() string {
	return strconv.Itoa(f.n)
}

func (f *testCasesFlag) Set(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	f.n = n
	return nil
}

// Option returns the option corresponding to this flag.
func (f *testCasesFlag) Option() Option {
	return WithTestCases(f.n)
}

// derandomizeFlag wires --derandomize to [WithDerandomize].
type derandomizeFlag struct {
	derandomize bool
}

func (f *derandomizeFlag) String() string {
	return strconv.FormatBool(f.derandomize)
}

func (f *derandomizeFlag) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	f.derandomize = b
	return nil
}

// IsBoolFlag tells the flag package that --derandomize parses without a value.
func (f *derandomizeFlag) IsBoolFlag() bool { return true }

// Option returns the option corresponding to this flag.
func (f *derandomizeFlag) Option() Option {
	return WithDerandomize(f.derandomize)
}

// singleTestCaseFlag wires --single-test-case to [WithSingleTestCase].
type singleTestCaseFlag struct {
	singleTestCase bool
}

func (f *singleTestCaseFlag) String() string {
	return strconv.FormatBool(f.singleTestCase)
}

func (f *singleTestCaseFlag) Set(s string) error {
	b, err := strconv.ParseBool(s)
	if err != nil {
		return err
	}
	f.singleTestCase = b
	return nil
}

// IsBoolFlag tells the flag package that --single-test-case parses without a value.
func (f *singleTestCaseFlag) IsBoolFlag() bool { return true }

// Option returns the option corresponding to this flag.
func (f *singleTestCaseFlag) Option() Option {
	if !f.singleTestCase {
		return func(*runOptions) {}
	}
	return WithSingleTestCase()
}

// databaseFlag wires --database to [WithDatabase]. A non-empty value selects
// [Database]; an empty value (--database=) selects [DatabaseDisabled].
type databaseFlag struct {
	path string
}

func (f *databaseFlag) String() string {
	return f.path
}

func (f *databaseFlag) Set(s string) error {
	f.path = s
	return nil
}

// Option returns the option corresponding to this flag.
func (f *databaseFlag) Option() Option {
	if f.path == "" {
		return WithDatabase(DatabaseDisabled())
	}
	return WithDatabase(Database(f.path))
}

// suppressHealthCheckFlag wires --suppress-health-check to
// [SuppressHealthCheck]. The flag is repeatable; each occurrence accumulates a
// single [HealthCheck] value.
type suppressHealthCheckFlag struct {
	checks []HealthCheck
}

func (f *suppressHealthCheckFlag) String() string {
	names := make([]string, len(f.checks))
	for i, hc := range f.checks {
		names[i] = hc.String()
	}
	return strings.Join(names, ",")
}

func (f *suppressHealthCheckFlag) Set(s string) error {
	hc, err := parseHealthCheck(s)
	if err != nil {
		return err
	}
	f.checks = append(f.checks, hc)
	return nil
}

// Option returns the option corresponding to this flag.
func (f *suppressHealthCheckFlag) Option() Option {
	return SuppressHealthCheck(f.checks...)
}

// parseHealthCheck is the inverse of [HealthCheck.String]. Unknown names
// return an error naming the bad value.
func parseHealthCheck(name string) (HealthCheck, error) {
	for _, hc := range AllHealthChecks() {
		if hc.String() == name {
			return hc, nil
		}
	}
	return 0, fmt.Errorf("unknown health check %q", name)
}
