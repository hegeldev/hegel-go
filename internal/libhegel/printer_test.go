package libhegel

import (
	"errors"
	"testing"
)

func TestPrinterDocument(t *testing.T) {
	ctx := NewContext()
	options, err := ctx.PrinterOptionsNew()
	if err != nil {
		t.Fatal(err)
	}
	if err := options.MaxWidth(ctx, 8); err != nil {
		t.Fatal(err)
	}
	p, err := ctx.PrinterNew(options)
	if err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(p.BeginGroup(ctx, 2, "["))
	must(p.Text(ctx, "first"))
	must(p.Text(ctx, ","))
	must(p.Breakable(ctx, " "))
	hole, err := p.Deferred(ctx)
	if err != nil {
		t.Fatal(err)
	}
	must(p.IfBreak(ctx, ","))
	must(p.Breakable(ctx, ""))
	must(p.EndGroup(ctx, "]"))
	must(hole.BeginSpeculative(ctx))
	must(hole.Text(ctx, "discarded"))
	must(hole.AbortSpeculative(ctx))
	must(hole.BeginSpeculative(ctx))
	must(hole.Text(ctx, "second"))
	must(hole.CommitSpeculative(ctx))
	if live, err := hole.IsLive(ctx); err != nil || !live {
		t.Fatalf("live = %v, %v", live, err)
	}
	must(p.Resolve(ctx))
	if live, err := hole.IsLive(ctx); err != nil || live {
		t.Fatalf("resolved live = %v, %v", live, err)
	}
	got, err := p.Value(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := "[first,\n  second,\n  ]"; got != want {
		t.Fatalf("document = %q, want %q", got, want)
	}
	if err := hole.Text(ctx, "late"); !errors.Is(err, E_INVALID_HANDLE) {
		t.Fatalf("late write = %v", err)
	}
	again, err := p.Value(ctx)
	if err != nil || again != got {
		t.Fatalf("second read = %q, %v", again, err)
	}
}

func TestPrinterTextAndNotes(t *testing.T) {
	ctx := NewContext()
	p, err := ctx.PrinterNew(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{p.Text(ctx, "a\x00b"), p.Comment(ctx, " // comment"), p.ShiftIndent(ctx, 2), p.HardBreak(ctx), p.Text(ctx, "c"), p.ShiftIndent(ctx, -2)} {
		if err != nil {
			t.Fatal(err)
		}
	}
	got, err := p.Value(ctx)
	if err != nil || got != "a\x00b // comment\n  c" {
		t.Fatalf("text = %q, %v", got, err)
	}
	settings, err := ctx.SettingsNew()
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Database(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if err := settings.ShowStatistics(ctx, true); err != nil {
		t.Fatal(err)
	}
	run, err := settings.RunStart(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc, err := run.NextTestCase(ctx)
	if err != nil || tc == nil {
		t.Fatalf("case = %v, %v", tc, err)
	}
	if err := tc.Event(ctx, "visited"); err != nil {
		t.Fatal(err)
	}
	if err := tc.EventValue(ctx, 1.25, "size"); err != nil {
		t.Fatal(err)
	}
	if err := tc.Note(ctx, "first\nsecond"); err != nil {
		t.Fatal(err)
	}
	doc, err := tc.Printer(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tc.MarkComplete(ctx, STATUS_VALID, ""); err != nil {
		t.Fatal(err)
	}
	got, err = doc.Value(ctx)
	if err != nil || got != "first\nsecond\n" {
		t.Fatalf("notes = %q, %v", got, err)
	}
}

func TestPrinterConstructorErrors(t *testing.T) {
	for _, constructor := range []string{"options", "printer", "case", "deferred"} {
		t.Run(constructor, func(t *testing.T) {
			ctx := Stub(t, uintptr(0), E_INVALID_ARG, "bad constructor")
			var err error
			switch constructor {
			case "options":
				p, e := ctx.PrinterOptionsNew()
				err = e
				if p != nil {
					t.Fatal("non-nil options")
				}
			case "printer":
				p, e := ctx.PrinterNew(nil)
				err = e
				if p != nil {
					t.Fatal("non-nil printer")
				}
			case "case":
				tc := &TestCase{pointer: &pointer[testCaseT]{syms: ctx.syms, raw: 2}}
				p, e := tc.Printer(ctx, nil)
				err = e
				if p != nil {
					t.Fatal("non-nil printer")
				}
			case "deferred":
				p := &Printer{pointer: &pointer[printerT]{syms: ctx.syms, raw: 2}}
				child, e := p.Deferred(ctx)
				err = e
				if child != nil {
					t.Fatal("non-nil child")
				}
			}
			if !errors.Is(err, E_INVALID_ARG) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestPrinterValueError(t *testing.T) {
	ctx := Stub(t, "", E_INVALID_HANDLE, "dead printer")
	p := &Printer{pointer: &pointer[printerT]{syms: ctx.syms, raw: 2}}
	if got, err := p.Value(ctx); got != "" || !errors.Is(err, E_INVALID_HANDLE) {
		t.Fatalf("value = %q, %v", got, err)
	}
}

func TestNewControlBindings(t *testing.T) {
	for _, result := range []Error{OK, E_STOP_TEST} {
		t.Run(result.String(), func(t *testing.T) {
			returns := []any{result}
			if result != OK {
				returns = append(returns, "stopped")
			}
			ctx := Stub(t, returns...)
			tc := &TestCase{pointer: &pointer[testCaseT]{syms: ctx.syms, raw: 2}}
			r := &Recursion{pointer: pointer[recursionT]{syms: ctx.syms, raw: 3}}
			err := r.Finish(ctx, tc)
			if (result == OK && err != nil) || (result != OK && !errors.Is(err, result)) {
				t.Fatalf("finish = %v", err)
			}
		})
	}
	for _, enabled := range []bool{false, true} {
		ctx := Stub(t, enabled, OK)
		tc := &TestCase{pointer: &pointer[testCaseT]{syms: ctx.syms, raw: 2}}
		machine := &StateMachine{syms: ctx.syms, raw: 3}
		got, err := tc.StateMachineShouldCheckInvariant(ctx, machine, 4)
		if err != nil || got != enabled {
			t.Fatalf("invariant = %v, %v", got, err)
		}
	}
}

func TestNanosecondDrawRoundTrip(t *testing.T) {
	ctx := NewContext()
	settings, err := ctx.SettingsNew()
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Database(ctx, ""); err != nil {
		t.Fatal(err)
	}
	run, err := settings.RunStart(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	tc, err := run.NextTestCase(ctx)
	if err != nil || tc == nil {
		t.Fatalf("case = %v, %v", tc, err)
	}
	bound := Time{Hour: 23, Minute: 59, Second: 59, Nanosecond: 987654321}
	got, err := tc.GenerateTime(ctx, bound, bound)
	if err != nil || got != bound {
		t.Fatalf("time = %#v, %v", got, err)
	}
	dateBound := Datetime{Date: Date{Year: 2026, Month: 9, Day: 14}, Time: bound}
	dt, err := tc.GenerateDatetime(ctx, dateBound, dateBound)
	if err != nil || dt != dateBound || dt.ToTime().Nanosecond() != 987654321 {
		t.Fatalf("datetime = %#v, %v", dt, err)
	}
	if err := tc.MarkComplete(ctx, STATUS_VALID, ""); err != nil {
		t.Fatal(err)
	}
}
