package hegel

import (
	"errors"
	"io"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

func TestNativeOutputLifecycle(t *testing.T) {
	s := newRealTestCase(t)
	var out strings.Builder
	s.out = &out
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
		_, err = io.WriteString(clone.(*testCase).out, "literal\nlast")
		if err != nil {
			t.Fatal(err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "first\nsecond\nchild\nframework 3\nliteral\nlastparent after clone\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if err := s.tc.Note(s.ctx, "late"); !errors.Is(err, libhegel.E_INVALID_HANDLE) {
		t.Fatalf("late write = %v", err)
	}
}

func TestNativeOutputFlushesOnPanic(t *testing.T) {
	s := newRealTestCase(t)
	var out strings.Builder
	s.out = &out
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

func TestNativeOutputInitializationError(t *testing.T) {
	s := newStubTestCase(t, uintptr(0), libhegel.E_BACKEND, "printer failed")
	s.out = io.Discard
	_, err := s.run(func(TestCase) { t.Fatal("body ran") })
	if !errors.Is(err, libhegel.E_BACKEND) {
		t.Fatal(err)
	}
}

func TestNativeOutputCloneInitializationError(t *testing.T) {
	s := newStubTestCase(t, uintptr(1), libhegel.OK, uintptr(2), libhegel.OK, uintptr(2), uintptr(0), libhegel.E_BACKEND, "clone printer failed")
	s.out = io.Discard
	if err := s.initNativeOutput(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.clone(); !errors.Is(err, libhegel.E_BACKEND) {
		t.Fatal(err)
	}
}

func TestNativeOutputReadErrors(t *testing.T) {
	s := newStubTestCase(t,
		uintptr(1), libhegel.OK, // printer
		libhegel.OK,                           // resolve
		"", libhegel.E_BACKEND, "read failed", // value
	)
	s.out = io.Discard
	if err := s.initNativeOutput(); err != nil {
		t.Fatal(err)
	}
	if err := s.flushNativeOutput(io.Discard); !errors.Is(err, libhegel.E_BACKEND) {
		t.Fatal(err)
	}
}

func TestNativeOutputWithoutDeferredRegions(t *testing.T) {
	s := newRealTestCase(t)
	s.out = io.Discard
	if err := s.initNativeOutput(); err != nil {
		t.Fatal(err)
	}
	s.Note("plain output")
	// Resolve reports NothingToResolve; the value remains readable, including
	// when a sealed document is read again.
	for range 2 {
		var out strings.Builder
		if err := s.flushNativeOutput(&out); err != nil {
			t.Fatal(err)
		}
		if got := out.String(); got != "plain output\n" {
			t.Fatalf("output = %q", got)
		}
	}
}

func TestNativeOutputDeferredLayoutError(t *testing.T) {
	s := newRealTestCase(t)
	s.out = io.Discard
	if err := s.initNativeOutput(); err != nil {
		t.Fatal(err)
	}
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
	if err := s.flushNativeOutput(&out); !errors.Is(err, libhegel.E_INVALID_ARG) {
		t.Fatalf("layout error = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("emitted invalid document: %q", out.String())
	}
}

func TestNativeNoteError(t *testing.T) {
	s := newRealTestCase(t)
	s.out = io.Discard
	if err := s.initNativeOutput(); err != nil {
		t.Fatal(err)
	}
	err := s.invoke(func(tc TestCase) { tc.Note(string([]byte{255})) })
	if !errors.Is(err, libhegel.E_INVALID_ARG) {
		t.Fatalf("note = %v", err)
	}
}

func TestNativeWriterErrors(t *testing.T) {
	for _, newline := range []bool{false, true} {
		t.Run(map[bool]string{false: "text", true: "break"}[newline], func(t *testing.T) {
			ops := []any{uintptr(1), libhegel.OK}
			if newline {
				ops = append(ops, libhegel.OK)
			}
			ops = append(ops, libhegel.E_BACKEND, "write failed")
			s := newStubTestCase(t, ops...)
			s.out = io.Discard
			if err := s.initNativeOutput(); err != nil {
				t.Fatal(err)
			}
			n, err := s.out.Write([]byte("a\n"))
			want := 0
			if newline {
				want = 1
			}
			if n != want || !errors.Is(err, libhegel.E_BACKEND) {
				t.Fatalf("write = %d, %v", n, err)
			}
		})
	}
}
