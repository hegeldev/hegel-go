package hegel

import (
	"errors"
	"io"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

func reproductionBlob(t *testing.T) string {
	t.Helper()
	var out strings.Builder
	err := run(1, func(tc TestCase) {
		_ = Draw(tc, Integers[int](1, 10))
		tc.Errorf("original failure")
	}, WithDatabase(""), WithTestCases(1), WithReproductionBlob(true), withOutput(&out))
	if !errors.Is(err, errPropTestFailed) {
		t.Fatalf("generating blob: %v", err)
	}
	_, rest, ok := strings.Cut(out.String(), "reproduction blob: ")
	if !ok {
		t.Fatalf("no blob in %q", out.String())
	}
	blob, _, _ := strings.Cut(rest, "\n")
	if !strings.Contains(out.String(), "hegel.WithReplay(\""+blob+"\")") {
		t.Fatalf("missing replay option: %s", &out)
	}
	return blob
}

func TestReplayBlob(t *testing.T) {
	blob := reproductionBlob(t)
	var out strings.Builder
	calls := 0
	err := run(1, func(tc TestCase) {
		calls++
		n := Draw(tc, Integers[int](1, 10))
		if n != 1 {
			t.Errorf("replayed value = %d, want shrunk value 1", n)
		}
		tc.Note("replay note")
		tc.Errorf("replayed failure")
	}, WithReplay(blob), WithReplay("invalid ignored blob"), WithDatabase(""), WithReproductionBlob(true), withOutput(&out))
	if !errors.Is(err, errPropTestFailed) || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	for _, want := range []string{"replay note", "replayed failure", "hegel.WithReplay(\"" + blob + "\")", "1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output %q missing %q", out.String(), want)
		}
	}
}

func TestReplayStaleAndInvalid(t *testing.T) {
	blob := reproductionBlob(t)
	for _, invalid := range []string{"", "not-base64"} {
		if err := Run(func(TestCase) { t.Error("invalid blob ran body") }, WithReplay(invalid)); err == nil {
			t.Fatal("invalid blob succeeded")
		}
	}
	err := Run(func(tc TestCase) { _ = Draw(tc, Integers[int](1, 10)) }, WithReplay(blob), WithReproductionBlob(false))
	if !errors.Is(err, errPropTestFailed) || !strings.Contains(err.Error(), "no longer reproduces") {
		t.Fatalf("stale blob: %v", err)
	}
}

func TestReplayUserPanic(t *testing.T) {
	blob := reproductionBlob(t)
	var out strings.Builder
	defer func() {
		if got := recover(); got != "user panic" {
			t.Errorf("panic = %v", got)
		}
		if !strings.Contains(out.String(), "hegel.WithReplay(\""+blob+"\")") {
			t.Errorf("missing blob after panic: %s", &out)
		}
	}()
	_ = run(1, func(tc TestCase) { _ = Draw(tc, Integers[int](1, 10)); panic("user panic") }, WithReplay(blob), WithReproductionBlob(true), withOutput(&out))
}

func TestReplayErrors(t *testing.T) {
	cases := []struct {
		name    string
		returns []any
		output  io.Writer
		want    string
	}{
		{"start", []any{uintptr(0), libhegel.E_BACKEND, "start failed"}, nil, "start failed"},
		{"print setting", []any{uintptr(1), libhegel.OK, false, libhegel.E_BACKEND, "print failed"}, nil, "print failed"},
		{"next", []any{uintptr(1), libhegel.OK, false, libhegel.OK, uintptr(0), libhegel.E_BACKEND, "next failed"}, nil, "next failed"},
		{"printer", []any{uintptr(1), libhegel.OK, false, libhegel.OK, uintptr(1), libhegel.OK, uintptr(0), libhegel.E_BACKEND, "printer failed"}, io.Discard, "printer failed"},
		{"complete", []any{uintptr(1), libhegel.OK, false, libhegel.OK, uintptr(1), libhegel.OK, libhegel.E_BACKEND, "complete failed"}, nil, "complete failed"},
		{"result", []any{uintptr(1), libhegel.OK, false, libhegel.OK, uintptr(0), libhegel.OK, uintptr(0), libhegel.E_BACKEND, "result failed"}, nil, "result failed"},
		{"engine", []any{uintptr(1), libhegel.OK, false, libhegel.OK, uintptr(0), libhegel.OK, uintptr(1), libhegel.OK, libhegel.RUN_STATUS_ERROR, libhegel.OK, "engine failed", libhegel.OK}, nil, "engine failed"},
		{"write", []any{uintptr(1), libhegel.OK, true, libhegel.OK, uintptr(0), libhegel.OK, uintptr(1), libhegel.OK, libhegel.RUN_STATUS_PASSED, libhegel.OK}, nonemptyWriteError{err: errors.New("write failed")}, "write failed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			returns := append([]any{uintptr(1), libhegel.OK}, c.returns...)
			opts := applyOpts([]Option{WithReplay("blob"), withOutput(c.output)})
			err := runWithContext(libhegel.Stub(t, returns...), func(TestCase) {}, opts)
			if c.name == "engine" && !errors.Is(err, errPropTestFailed) {
				t.Fatalf("engine error lost property-failure sentinel: %v", err)
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want %s", err, c.want)
			}
		})
	}
}

func TestWorkloadReplay(t *testing.T) {
	blob := reproductionBlob(t)
	var out strings.Builder
	calls := 0
	err := workload(1, []string{"workload", "--replay", blob, "--replay", "invalid"}, &out, io.Discard, func(tc TestCase) {
		calls++
		_ = Draw(tc, Integers[int](1, 10))
		tc.Errorf("CLI failure")
	}, []Option{WithReplay("overridden invalid"), WithReproductionBlob(false)})
	if !errors.Is(err, errPropTestFailed) || calls != 1 || !strings.Contains(out.String(), "CLI failure") {
		t.Fatalf("err=%v calls=%d output=%q", err, calls, out.String())
	}
	if strings.Contains(out.String(), "hegel.WithReplay(") {
		t.Fatalf("blob printed while disabled: %s", &out)
	}
	f := &replayFlag{}
	if f.String() != "" {
		t.Fatal("nonempty default")
	}
	_ = f.Set(blob)
	_ = f.Set("ignored")
	if f.String() != blob {
		t.Fatal("first flag did not win")
	}
}

func TestReplayPrintingWithoutWriter(t *testing.T) {
	blob := reproductionBlob(t)
	err := run(1, func(tc TestCase) {
		_ = Draw(tc, Integers[int](1, 10))
		tc.Errorf("silent failure")
	}, WithReplay(blob), WithReproductionBlob(true))
	if !errors.Is(err, errPropTestFailed) {
		t.Fatalf("replay without writer: %v", err)
	}
}

func TestReplayHidesPassingAttempts(t *testing.T) {
	blob := reproductionBlob(t)
	var out strings.Builder
	calls := 0
	err := run(1, func(tc TestCase) {
		calls++
		_ = Draw(tc, Integers[int](1, 10))
		if calls == 1 {
			tc.Note("passing attempt must stay private")
			return
		}
		tc.Note("failing attempt is visible")
		tc.Errorf("retry failure")
	}, WithReplay(blob), WithReproductionBlob(false), withOutput(&out))
	if !errors.Is(err, errPropTestFailed) || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	if strings.Contains(out.String(), "passing attempt") || !strings.Contains(out.String(), "failing attempt is visible") {
		t.Fatalf("retry output: %s", &out)
	}
}

func TestReplayFailureOutputWriteError(t *testing.T) {
	blob := reproductionBlob(t)
	want := errors.New("failure output write error")
	err := run(1, func(tc TestCase) {
		_ = Draw(tc, Integers[int](1, 10))
		tc.Errorf("replayed failure")
	}, WithReplay(blob), WithReproductionBlob(false), withOutput(nonemptyWriteError{want}))
	if !errors.Is(err, want) {
		t.Fatalf("error=%v, want %v", err, want)
	}
}
