package hegel

// ci_test.go pins the CI defaults documented on WithDatabase and
// WithDerandomize: libhegel's hegel_settings_new detects CI environments and
// then disables the example database and derandomizes runs by default, and
// explicit user options always beat those defaults (settings appliers run
// after the engine defaults; see settingApplier).

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// knownCIEnvVars covers the environment variables probed for CI detection
// across the Hegel implementations (see is_in_ci in hegel-rust's
// hegel-c/src/settings.rs for the set the engine uses); clearing all of them
// puts the engine in a deterministic "not CI" state.
var knownCIEnvVars = []string{
	"CI",
	"BITBUCKET_COMMIT",
	"BUILDKITE",
	"CIRCLECI",
	"CIRRUS_CI",
	"CODEBUILD_BUILD_ID",
	"GITHUB_ACTIONS",
	"GITLAB_CI",
	"HEROKU_TEST_RUN_ID",
	"TEAMCITY_VERSION",
	"TF_BUILD",
	"bamboo.buildKey",
}

// clearCIEnv unsets every known CI environment variable for the duration of
// the test (t.Setenv registers the restore; the value itself is removed).
func clearCIEnv(t *testing.T) {
	t.Helper()
	for _, name := range knownCIEnvVars {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
}

// simulateCI makes the process look like it is running under CI.
func simulateCI(t *testing.T) {
	t.Helper()
	clearCIEnv(t)
	t.Setenv("CI", "true")
}

// failingBody is a property that always fails, so the engine persists a
// counterexample whenever the database is enabled.
func failingBody(tc TestCase) {
	tc.Fail()
}

// defaultDatabaseDir is where libhegel's default database lands, relative to
// the working directory, when no WithDatabase option is given.
const defaultDatabaseDir = ".hegel"

func TestCIDefaultDisablesDatabase(t *testing.T) {
	simulateCI(t)
	t.Chdir(t.TempDir())

	err := run(failingBody, withDatabaseKey("ci_disables_database"))
	if err == nil {
		t.Fatal("expected property test failure")
	}
	if _, statErr := os.Stat(defaultDatabaseDir); !os.IsNotExist(statErr) {
		t.Errorf("expected no %s directory in CI, stat error = %v", defaultDatabaseDir, statErr)
	}
}

func TestNoCIDefaultDatabaseEnabled(t *testing.T) {
	clearCIEnv(t)
	t.Chdir(t.TempDir())

	err := run(failingBody, withDatabaseKey("no_ci_database_enabled"))
	if err == nil {
		t.Fatal("expected property test failure")
	}
	if _, statErr := os.Stat(defaultDatabaseDir); statErr != nil {
		t.Errorf("expected default database directory %s outside CI: %v", defaultDatabaseDir, statErr)
	}
}

// TestCIExplicitDatabaseWins verifies that a user's WithDatabase overrides the
// CI default of disabling the database.
func TestCIExplicitDatabaseWins(t *testing.T) {
	simulateCI(t)
	dbDir := t.TempDir()

	err := run(failingBody,
		WithDatabase(dbDir),
		withDatabaseKey("ci_explicit_database_wins"),
	)
	if err == nil {
		t.Fatal("expected property test failure")
	}
	entries, readErr := os.ReadDir(dbDir)
	if readErr != nil {
		t.Fatalf("readdir: %v", readErr)
	}
	if len(entries) == 0 {
		t.Error("expected explicit WithDatabase directory to contain saved examples despite CI")
	}
}

// drawSequence runs a passing property and records the first integer drawn in
// each test case.
func drawSequence(t *testing.T, opts ...Option) []int64 {
	t.Helper()
	var seq []int64
	opts = append(opts, WithTestCases(10))
	err := run(func(tc TestCase) {
		seq = append(seq, Draw(tc, Integers[int64](math.MinInt64, math.MaxInt64)))
	}, opts...)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return seq
}

func sequencesEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestCIDefaultDerandomizes verifies that in CI two identical runs draw the
// same values (the run is derandomized by default).
func TestCIDefaultDerandomizes(t *testing.T) {
	simulateCI(t)
	first := drawSequence(t)
	second := drawSequence(t)
	if !sequencesEqual(first, second) {
		t.Errorf("expected identical draw sequences in CI, got %v then %v", first, second)
	}
}

// TestCIExplicitDerandomizeFalseWins verifies that a user's
// WithDerandomize(false) overrides the CI default. Two full-range 64-bit draw
// sequences colliding by chance is vanishingly unlikely.
func TestCIExplicitDerandomizeFalseWins(t *testing.T) {
	simulateCI(t)
	first := drawSequence(t, WithDerandomize(false))
	second := drawSequence(t, WithDerandomize(false))
	if sequencesEqual(first, second) {
		t.Errorf("expected differing draw sequences with WithDerandomize(false), got %v twice", first)
	}
}

// TestCIDefaultDatabaseKeyIgnored: the database key is applied even when the
// CI default disables the database; libhegel must ignore it. This is the
// exact option layering Test() produces (user opts, then key) in CI.
func TestCIDatabaseKeyWithDisabledDatabase(t *testing.T) {
	simulateCI(t)
	t.Chdir(t.TempDir())

	err := run(failingBody, withDatabaseKey("TestCIDatabaseKeyWithDisabledDatabase"))
	if err == nil {
		t.Fatal("expected property test failure")
	}
	if _, statErr := os.Stat(filepath.Join(defaultDatabaseDir, "examples")); !os.IsNotExist(statErr) {
		t.Errorf("expected no default database in CI, stat error = %v", statErr)
	}
}
