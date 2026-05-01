package hegel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func readValues(t *testing.T, dir, label string) []int64 {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, label))
	if err != nil {
		t.Fatalf("read values %s: %v", label, err)
	}
	var out []int64
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		n, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			t.Fatalf("parse %q: %v", line, err)
		}
		out = append(out, n)
	}
	return out
}

func TestDatabasePersistsFailingExamples(t *testing.T) {
	clearCIEnv(t)
	dbDir := t.TempDir()

	entries, err := os.ReadDir(dbDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatal("expected empty database directory before the run")
	}

	err = runHegel(func(_ *TestCase) {
		panic("")
	}, stdoutNoteFn, []Option{
		WithDatabase(Database(dbDir)),
		withDatabaseKey([]byte("test_database_persists")),
	})
	if err == nil {
		t.Fatal("expected property test failure")
	}
	if !strings.Contains(err.Error(), "property test failed") {
		t.Errorf("error %q does not contain 'property test failed'", err.Error())
	}

	entries, err = os.ReadDir(dbDir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected database directory to contain saved examples")
	}
}

func TestDatabaseKeyReplaysFailure(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "database")
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatalf("mkdir db: %v", err)
	}
	valuesPath := filepath.Join(tempDir, "values")
	if err := os.MkdirAll(valuesPath, 0o755); err != nil {
		t.Fatalf("mkdir values: %v", err)
	}
	dbStr := strings.ReplaceAll(dbPath, "\\", "/")

	clearCIEnv(t)
	t.Setenv("VALUES_DIR", valuesPath)

	testCode := fmt.Sprintf(`package temptest

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"hegel.dev/go/hegel"
)

func recordTestCase(label string, n int64) {
	path := filepath.Join(os.Getenv("VALUES_DIR"), label)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	fmt.Fprintf(f, "%%d\n", n)
}

func TestReplay1(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		n := hegel.Draw[int64](ht, hegel.Integers[int64](math.MinInt64, math.MaxInt64))
		recordTestCase("TestReplay1", n)
		if n >= 1_000_000 {
			panic(fmt.Sprintf("n=%%d", n))
		}
	}, hegel.WithDatabase(hegel.Database(%q)))
}

func TestReplay2(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		n := hegel.Draw[int64](ht, hegel.Integers[int64](math.MinInt64, math.MaxInt64))
		recordTestCase("TestReplay2", n)
		if n >= 1_000_000 {
			panic(fmt.Sprintf("n=%%d", n))
		}
	}, hegel.WithDatabase(hegel.Database(%q)))
}
`, dbStr, dbStr)

	project := newTempGoProject(t)
	project.writeFile("hegel_test.go", testCode)
	project.expectFailure(`property test failed`)

	// run TestReplay1: database now has a failing entry for it.
	project.goTest("-run", "^TestReplay1$")
	values := readValues(t, valuesPath, "TestReplay1")
	shrunk := values[len(values)-1]
	if shrunk != 1_000_000 {
		t.Fatalf("shrunk value = %d, want 1000000", shrunk)
	}

	// clear the log file
	if err := os.Remove(filepath.Join(valuesPath, "TestReplay1")); err != nil {
		t.Fatalf("remove values: %v", err)
	}

	// run TestReplay1 again. It should replay the shrunk test case immediately.
	project.goTest("-run", "^TestReplay1$")
	values = readValues(t, valuesPath, "TestReplay1")
	if values[0] != shrunk {
		t.Fatalf("expected to replay shrunk test case %d first, got %d", shrunk, values[0])
	}

	// run TestReplay2. It should not replay the TestReplay1 shrunk test case.
	project.goTest("-run", "^TestReplay2$")
	values = readValues(t, valuesPath, "TestReplay2")
	if values[0] == shrunk {
		t.Fatalf("expected NOT to replay %d for TestReplay2, but got it", shrunk)
	}
}
