package hegel

import (
	"go/types"
	"net/netip"
	"testing"
	"time"

	"golang.org/x/tools/go/packages"
	"hegel.dev/go/hegel/internal/libhegel"
)

// Every concrete named output of an exported generator constructor needs an
// explicit default. Multiple constructors for one type share that default.
func TestDefaultCoversGeneratorTypes(t *testing.T) {
	pkgs, err := packages.Load(&packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps}, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || len(pkgs[0].Errors) != 0 {
		t.Fatalf("load package: %v", pkgs)
	}
	registered := make(map[string]bool)
	for typ := range defaultKnownTypes {
		registered[typ.PkgPath()+"."+typ.Name()] = true
	}
	scope := pkgs[0].Types.Scope()
	for _, name := range scope.Names() {
		fn, ok := scope.Lookup(name).(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}
		sig := fn.Type().(*types.Signature)
		for v := range sig.Results().Variables() {
			method, _, _ := types.LookupFieldOrMethod(v.Type(), false, pkgs[0].Types, "draw")
			if method == nil {
				continue
			}
			output := method.Type().(*types.Signature).Results().At(0).Type()
			named, ok := types.Unalias(output).(*types.Named)
			if !ok {
				continue
			}
			obj := named.Obj()
			if !registered[obj.Pkg().Path()+"."+obj.Name()] {
				t.Errorf("%s generates %s: add its default to defaultKnownTypes", name, output)
			}
		}
	}
}

func TestDefaultKnownTypes(t *testing.T) {
	type record struct {
		Time      time.Time
		Address   netip.Addr
		Times     [2]time.Time
		Addresses []netip.Addr
		Lookup    map[netip.Addr]time.Time
		Pointer   *time.Time
	}
	Test(t, func(tc *T) {
		v := Draw(tc, Default[record]())
		if v.Time.Location() != time.UTC || v.Time.Year() < 1 || v.Time.Year() > 9999 {
			tc.Fatalf("invalid datetime: %v", v.Time)
		}
		if !v.Address.IsValid() {
			tc.Fatalf("invalid IP address: %v", v.Address)
		}
		for _, address := range v.Addresses {
			if !address.IsValid() {
				tc.Fatalf("invalid nested address: %v", address)
			}
		}
	}, WithTestCases(10))
}

func TestDefaultDatetimeUsesDatetimes(t *testing.T) {
	want := libhegel.Datetime{Date: libhegel.Date{Year: 2026, Month: 10, Day: 8}, Time: libhegel.Time{Hour: 12, Minute: 34, Second: 56, Nanosecond: 123}}
	got, err := Default[time.Time]().draw(newStubTestCase(t, libhegel.OK, want, libhegel.OK, libhegel.OK))
	if err != nil || got != want.ToTime() {
		t.Fatalf("got %v, %v; want %v", got, err, want.ToTime())
	}
}

func TestDefaultIPAddressUsesIPAddresses(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		var ops []any
		var want netip.Addr
		if ipv6 {
			bytes := [16]byte{0x20, 1, 0xd, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
			ops = []any{int64(1), libhegel.OK, bytes[:], libhegel.OK}
			want = netip.AddrFrom16(bytes)
		} else {
			bytes := [4]byte{192, 0, 2, 1}
			ops = []any{int64(0), libhegel.OK, bytes[:], libhegel.OK}
			want = netip.AddrFrom4(bytes)
		}
		got, err := Default[netip.Addr]().draw(newStubTestCase(t, append(append([]any{libhegel.OK}, ops...), libhegel.OK)...))
		if err != nil || got != want {
			t.Fatalf("got %v, %v; want %v", got, err, want)
		}
	}
}

func TestDefaultKnownTypeErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		draw func(TestCase) error
		ops  []any
	}{
		{"datetime", defaultDrawError[time.Time], []any{libhegel.OK, libhegel.Datetime{}, libhegel.E_BACKEND, "boom", libhegel.OK}},
		{"address", defaultDrawError[netip.Addr], []any{libhegel.OK, int64(0), libhegel.E_BACKEND, "boom", libhegel.OK}},
	} {
		t.Run(test.name, func(t *testing.T) { assertErrorContains(t, "boom", test.draw(newStubTestCase(t, test.ops...))) })
	}
}

func TestDefaultKnownTypesRequireExactType(t *testing.T) {
	type customTime time.Time
	defer func() {
		if recover() == nil {
			t.Fatal("a distinct type with private fields must still be rejected")
		}
	}()
	Default[customTime]()
}

func TestDefaultKnownTypeAliases(t *testing.T) {
	type datetime = time.Time
	if Default[datetime]() != Default[time.Time]() {
		t.Fatal("alias did not share its default generator")
	}
}
