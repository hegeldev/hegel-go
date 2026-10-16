package libhegel

import (
	"errors"
	"hash/fnv"
	"math"
	"testing"
)

func TestStringLabelsPreserveBytes(t *testing.T) {
	ctx := NewContext()
	for _, name := range []string{"", "hegel.日本語", "before\x00after"} {
		got, err := ctx.LabelFromName(name)
		hash := fnv.New64a()
		_, _ = hash.Write([]byte(name))
		if err != nil || uint64(got) != hash.Sum64() {
			t.Fatalf("LabelFromName(%q) = %x, %v; want %x", name, got, err, hash.Sum64())
		}
	}
}

func TestStringDatabasePreservesEmptyAndNUL(t *testing.T) {
	ctx := NewContext()
	settings, err := ctx.SettingsNewForProfile("base")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", "before\x00after", "日本語"} {
		if err := settings.Database(ctx, path); err != nil {
			t.Fatal(err)
		}
		got, present, err := settings.GetDatabase(ctx)
		if err != nil || !present || got != path {
			t.Fatalf("database = %q, %v, %v; want %q present", got, present, err, path)
		}
	}
}

func TestStringArraysAndOwnedResults(t *testing.T) {
	ctx := NewContext()
	settings, err := ctx.SettingsNewForProfile("base")
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{settings.Database(ctx, ""), settings.TestCases(ctx, 1)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	run, err := settings.RunStart(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc, err := run.NextTestCase(ctx)
	if err != nil || tc == nil {
		t.Fatalf("test case = %v, %v", tc, err)
	}
	if _, _, err := tc.NewStateMachine(ctx, []string{"rule\x00suffix"}, []int64{0}, nil, []string{"invariant\x00suffix"}, nil, 1, 1, 1); err != nil {
		t.Fatal(err)
	}
	for _, categories := range [][]string{nil, {"Lu"}} {
		gen, err := ctx.StringGeneratorText(1, 1, "utf-8", 0, math.MaxUint32, categories, []string{"Ll"}, "", "")
		if err != nil {
			t.Fatal(err)
		}
		value, err := tc.GenerateString(ctx, gen)
		if err != nil || value == "" {
			t.Fatalf("generated = %q, %v", value, err)
		}
	}
	if _, err := ctx.StringGeneratorText(1, 1, "utf-8", 0, math.MaxUint32, []string{}, nil, "", ""); !errors.Is(err, E_INVALID_ARG) {
		t.Fatalf("empty categories: %v", err)
	}
	include := "\x00é"
	gen, err := ctx.StringGeneratorText(1, 1, "utf-8", 0, math.MaxUint32, []string{}, nil, include, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := tc.GenerateString(ctx, gen); err != nil || got == "" {
		t.Fatalf("included alphabet = %q, %v", got, err)
	}
	if _, err := ctx.StringGeneratorText(1, 1, "utf-8", 0, math.MaxUint32, []string{}, nil, include, include); !errors.Is(err, E_INVALID_ARG) {
		t.Fatalf("fully excluded alphabet: %v", err)
	}
	for _, want := range []string{"", "a\x00日本語"} {
		gen, err := ctx.StringGeneratorRegex(want, true, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, err := tc.GenerateString(ctx, gen)
		if err != nil || got != want {
			t.Fatalf("generated = %q, %v; want %q", got, err, want)
		}
	}
	const origin = "failure\x00日本語"
	if err := tc.MarkComplete(ctx, STATUS_INTERESTING, origin); err != nil {
		t.Fatal(err)
	}
	for {
		tc, err := run.NextTestCase(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tc == nil {
			break
		}
		if err := tc.MarkComplete(ctx, STATUS_INTERESTING, origin); err != nil {
			t.Fatal(err)
		}
	}
	result, err := run.RunResult(ctx)
	if err != nil {
		t.Fatal(err)
	}
	failure, err := result.Failure(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := failure.Origin(ctx); got != origin {
		t.Fatalf("origin = %q, want %q", got, origin)
	}
}

func TestStringBindingErrors(t *testing.T) {
	ctx := NewContext()
	if err := ctx.SetDefaultProfile("default"); !errors.Is(err, E_INVALID_ARG) {
		t.Fatalf("default profile: %v", err)
	}
	settings, err := ctx.SettingsNewForProfile("base")
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.TestLocation(ctx, "file.go", 42, "\xff", "function"); !errors.Is(err, E_INVALID_ARG) {
		t.Fatalf("invalid UTF-8 location: %v", err)
	}
}

func TestStubBorrowedErrorPreservesBytes(t *testing.T) {
	const want = "error\x00日本語"
	ctx := Stub(t, want, OK)
	result := &Result{pointer: &pointer[resultT]{syms: ctx.syms, raw: 1}}
	if got := result.ErrorMessage(ctx); got != want {
		t.Fatalf("error message = %q, want %q", got, want)
	}
}
