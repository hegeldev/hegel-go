package hegel

import (
	"errors"
	"io"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

func newEmittingTestCase(t *testing.T, out io.Writer) *testCase {
	t.Helper()
	s := newRealTestCase(t)
	s, err := newTestCase(s.ctx, s.tc, out, s.panicPolicy)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNativeOutputLifecycle(t *testing.T) {
	var out strings.Builder
	s := newEmittingTestCase(t, &out)
	_, err := s.run(func(tc TestCase) {
		tc.Note("first\nsecond")
		if out.Len() != 0 {
			t.Fatal("output escaped before completion")
		}
		clone, err := tc.clone()
		if err != nil {
			t.Fatal(err)
		}
		tc.Note("parent after clone")
		clone.Note("child")
		clone.log("framework %d", 3)
		grandchild, err := clone.clone()
		if err != nil {
			t.Fatal(err)
		}
		if nested := grandchild.(*testCase); nested.out != nil || nested.printer == nil {
			t.Fatal("nested clone lost its printer or acquired output ownership")
		}
		grandchild.Note("grandchild")
		child := clone.(*testCase)
		if child.out != nil || child.printer == nil || s.out != &out {
			t.Fatal("only the root should own the output destination")
		}
		if out.Len() != 0 {
			t.Fatal("clone flushed the document")
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "first\nsecond\nchild\nframework 3\ngrandchild\nparent after clone\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if err := s.tc.Note(s.ctx, "late"); !errors.Is(err, libhegel.E_INVALID_HANDLE) {
		t.Fatalf("late write = %v", err)
	}
}

func TestNativeOutputFlushesOnPanic(t *testing.T) {
	var out strings.Builder
	s := newEmittingTestCase(t, &out)
	s.panicPolicy = propagateUserPanics
	defer func() {
		if got := recover(); got != "boom" {
			t.Fatalf("panic = %v", got)
		}
		if got := out.String(); got != "before panic\n" {
			t.Fatalf("output = %q", got)
		}
	}()
	_, _ = s.run(func(tc TestCase) { tc.Note("before panic"); panic("boom") })
}

func TestNativeOutputDiscardsRejectedCases(t *testing.T) {
	for name, reject := range map[string]func(TestCase){
		"assume": func(tc TestCase) { tc.Assume(false) },
		"overrun": func(tc TestCase) {
			tc.(*testCase).abort(libhegel.E_STOP_TEST)
		},
	} {
		t.Run(name, func(t *testing.T) {
			var out strings.Builder
			s := newEmittingTestCase(t, &out)
			_, err := s.run(func(tc TestCase) {
				tc.Note("rejected")
				reject(tc)
			})
			if err != nil {
				t.Fatal(err)
			}
			if out.Len() != 0 {
				t.Fatalf("rejected output = %q", out.String())
			}
		})
	}
}

func TestNativeOutputInitializationError(t *testing.T) {
	s := newStubTestCase(t, uintptr(0), libhegel.E_BACKEND, "printer failed")
	_, err := newTestCase(s.ctx, s.tc, io.Discard, s.panicPolicy)
	if !errors.Is(err, libhegel.E_BACKEND) {
		t.Fatal(err)
	}
}

func TestNativeOutputCloneInitializationError(t *testing.T) {
	s := newStubTestCase(t, uintptr(1), libhegel.OK, uintptr(2), libhegel.OK, uintptr(2), uintptr(0), libhegel.E_BACKEND, "clone printer failed")
	s.out = io.Discard
	if printer, err := s.tc.Printer(s.ctx, nil); err != nil {
		t.Fatal(err)
	} else {
		s.printer = printer
	}
	if _, err := s.clone(); !errors.Is(err, libhegel.E_BACKEND) {
		t.Fatal(err)
	}
}

func TestNativeOutputReadErrors(t *testing.T) {
	s := newStubTestCase(t,
		uintptr(1), libhegel.OK, // printer
		libhegel.OK,                           // mark complete
		libhegel.OK,                           // resolve
		"", libhegel.E_BACKEND, "read failed", // value
	)
	s.out = io.Discard
	if printer, err := s.tc.Printer(s.ctx, nil); err != nil {
		t.Fatal(err)
	} else {
		s.printer = printer
	}
	if _, err := s.run(func(TestCase) {}); !errors.Is(err, libhegel.E_BACKEND) {
		t.Fatal(err)
	}
}

func TestNativeOutputWithoutDeferredRegions(t *testing.T) {
	s := newEmittingTestCase(t, io.Discard)
	s.Note("plain output")
	// Resolve reports NothingToResolve; the value remains readable.
	var out strings.Builder
	s.out = &out
	if _, err := s.run(func(TestCase) {}); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "plain output\n" {
		t.Fatalf("output = %q", got)
	}
}

func TestNativeOutputDeferredLayoutError(t *testing.T) {
	s := newEmittingTestCase(t, io.Discard)
	hole, err := s.printer.Deferred(s.ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Deferred writes are replayed only at resolution. An unmatched EndGroup
	// fails then, and Value must still report that failure after Resolve.
	if err := hole.EndGroup(s.ctx, "}"); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	s.out = &out
	if _, err := s.run(func(TestCase) {}); !errors.Is(err, libhegel.E_INVALID_ARG) {
		t.Fatalf("layout error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("emitted invalid document: %q", out.String())
	}
}

func TestNativeNoteError(t *testing.T) {
	s := newEmittingTestCase(t, io.Discard)
	err := s.invoke(func(tc TestCase) { tc.Note(string([]byte{255})) })
	if !errors.Is(err, libhegel.E_INVALID_ARG) {
		t.Fatalf("note = %v", err)
	}
}

func TestNativeOutputWriteError(t *testing.T) {
	want := errors.New("write failed")
	s := newEmittingTestCase(t, failingNativeOutputWriter{want})
	s.Note("output")
	if _, err := s.run(func(TestCase) {}); !errors.Is(err, want) {
		t.Fatal(err)
	}
	if err := s.tc.Note(s.ctx, "late"); !errors.Is(err, libhegel.E_INVALID_HANDLE) {
		t.Fatalf("test case remained live after write error: %v", err)
	}
}

type failingNativeOutputWriter struct{ err error }

func (w failingNativeOutputWriter) Write([]byte) (int, error) { return 0, w.err }

func TestNativeOutputDisabled(t *testing.T) {
	s := newStubTestCase(t,
		uintptr(2), libhegel.OK, // clone
		uintptr(2),  // context clone
		libhegel.OK, // mark complete
	)
	_, err := s.run(func(tc TestCase) {
		clone, err := tc.clone()
		if err != nil {
			t.Fatal(err)
		}
		if child := clone.(*testCase); child.out != nil || child.printer != nil {
			t.Fatal("disabled output acquired a writer or printer")
		}
		clone.Note("silent child")
		tc.Note("silent root")
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRunPrinterInitializationErrors(t *testing.T) {
	for _, replay := range []bool{false, true} {
		t.Run(map[bool]string{false: "nondeterministic", true: "replay"}[replay], func(t *testing.T) {
			ops := []any{
				uintptr(1), libhegel.OK, // settings
				libhegel.OK,             // derandomize
				uintptr(1), libhegel.OK, // run start
			}
			if replay {
				ops = append(ops,
					uintptr(0), libhegel.OK, // no next case
					uintptr(1), libhegel.OK, // result
					libhegel.RUN_STATUS_FAILED, libhegel.OK,
					uint64(1), libhegel.OK, // failure count
					uintptr(1), libhegel.OK, // failure
					"blob", libhegel.OK,
					false, libhegel.OK, // print blob
					uintptr(1), libhegel.OK, // case from blob
				)
			} else {
				ops = append(ops, uintptr(1), libhegel.OK, true, libhegel.OK)
			}
			ops = append(ops, uintptr(0), libhegel.E_BACKEND, "printer failed")
			ctx := libhegel.Stub(t, ops...)
			opts := applyOpts([]Option{WithDerandomize(false)})
			opts.output = io.Discard
			err := runWithContext(ctx, func(TestCase) { t.Fatal("body ran after constructor failed") }, opts)
			if !errors.Is(err, libhegel.E_BACKEND) {
				t.Fatal(err)
			}
		})
	}
}
