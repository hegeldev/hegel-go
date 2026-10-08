package hegel

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"hegel.dev/go/hegel/internal/libhegel"
)

type defaultNamedInt int16
type defaultNamedString string
type defaultNamedPointer *defaultTree
type defaultLeafPointer *defaultLeaf

type defaultRecord struct {
	Flag   bool
	Count  defaultNamedInt
	Name   defaultNamedString
	Values []uint8
	Lookup map[string]int
	Pair   [2]float32
	Float  float64
	Int8   int8
	Int32  int32
	Int64  int64
	Uint   uint
	Uint16 uint16
	Uint32 uint32
	Uint64 uint64
	Ptr    uintptr
}

type defaultTree struct {
	Value int
	Next  defaultNamedPointer
}

type defaultA struct{ B *defaultB }
type defaultB struct{ A *defaultA }
type defaultList struct{ Children []defaultList }
type defaultMap map[string]defaultMap
type defaultBinary struct{ Left, Right *defaultBinary }
type defaultKey struct{ Next *defaultKey }
type defaultKeyMap map[defaultKey]int
type defaultLeaf struct{ Value int }
type defaultShared struct{ Left, Right *defaultLeaf }

func TestDefaultCachesGenerator(t *testing.T) {
	gen := Default[defaultRecord]()
	if Default[defaultRecord]() != gen {
		t.Fatal("Default returned a different generator")
	}
	allocs := testing.AllocsPerRun(1000, func() {
		Default[defaultRecord]()
	})
	if allocs != 0 {
		t.Fatalf("Default allocated %g times per call, want zero", allocs)
	}
}

func TestDefaultCachesSharedAcyclicShape(t *testing.T) {
	shape, err := buildDefault(reflect.TypeFor[defaultShared](), make(map[reflect.Type]*defaultShape), make(map[reflect.Type]bool), nil)
	if err != nil {
		t.Fatal(err)
	}
	if shape.fields[0].elem != shape.fields[1].elem {
		t.Fatal("shared field type has separate plans")
	}
}

func TestDefaultCompositeTypes(t *testing.T) {
	gen := Default[defaultRecord]()
	sawValues, sawLookup, sawCount := false, false, false
	Test(t, func(tc *T) {
		v := Draw(tc, gen)
		if v.Values == nil || v.Lookup == nil {
			tc.Fatal("collections were not constructed")
		}
		sawValues = sawValues || len(v.Values) > 0
		sawLookup = sawLookup || len(v.Lookup) > 0
		sawCount = sawCount || v.Count != 0
	}, WithTestCases(50))
	if !sawValues || !sawLookup || !sawCount {
		t.Fatalf("generation lacked variety: values=%v lookup=%v count=%v", sawValues, sawLookup, sawCount)
	}
}

func TestDefaultRejectsRecursiveTypes(t *testing.T) {
	for _, test := range []struct {
		name string
		make func()
	}{
		{"named pointer", func() { Default[defaultTree]() }},
		{"pointer root", func() { Default[*defaultTree]() }},
		{"mutual", func() { Default[defaultA]() }},
		{"slice", func() { Default[defaultList]() }},
		{"map", func() { Default[defaultMap]() }},
		{"siblings", func() { Default[defaultBinary]() }},
		{"map key", func() { Default[defaultKeyMap]() }},
		{"array", func() { Default[[1]defaultTree]() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				p := recover()
				if p == nil || !strings.Contains(p.(string), "recursive types are unsupported") {
					t.Fatalf("panic = %v, want recursive type rejection", p)
				}
			}()
			test.make()
		})
	}
}

func TestDefaultRejectsUnsupportedFields(t *testing.T) {
	for _, test := range []struct {
		name string
		make func()
		want string
	}{
		{"interface", func() { Default[struct{ Value any }]() }, "unsupported kind interface"},
		{"private", func() { Default[struct{ value int }]() }, "field value is unexported"},
		{"channel", func() { Default[chan int]() }, "unsupported kind chan"},
		{"function", func() { Default[func()]() }, "unsupported kind func"},
		{"complex", func() { Default[complex128]() }, "unsupported kind complex128"},
		{"unsafe pointer", func() { Default[unsafe.Pointer]() }, "unsupported kind unsafe.Pointer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				p := recover()
				if p == nil || !strings.Contains(p.(string), test.want) {
					t.Fatalf("panic = %v, want %q", p, test.want)
				}
			}()
			test.make()
		})
	}
}

func TestDefaultPointers(t *testing.T) {
	plain := Default[*int]()
	named := Default[defaultLeafPointer]()
	sawNil, sawPresent := false, false
	Test(t, func(tc *T) {
		value := Draw(tc, plain)
		sawNil = sawNil || value == nil
		sawPresent = sawPresent || value != nil
		_ = Draw(tc, named)
		_ = Draw(tc, Default[defaultShared]())
	}, WithTestCases(50))
	if !sawNil || !sawPresent {
		t.Fatalf("pointer generation lacked variety: nil=%v present=%v", sawNil, sawPresent)
	}
}

func TestDefaultZeroLengthArrays(t *testing.T) {
	type zeroRecursive struct{ Next [0]*zeroRecursive }
	Test(t, func(tc *T) {
		_ = Draw(tc, Default[[0]chan int]())
		_ = Draw(tc, Default[[0]defaultTree]())
		_ = Draw(tc, Default[zeroRecursive]())
	}, WithTestCases(1))
}

func defaultDrawError[T any](tc TestCase) error {
	_, err := Default[T]().draw(tc)
	return err
}

func TestDefaultPropagatesDrawErrors(t *testing.T) {
	for _, test := range []struct {
		name string
		draw func(TestCase) error
		ops  []any
	}{
		{"primitive", defaultDrawError[int], []any{libhegel.OK, int64(0), libhegel.E_BACKEND, "boom"}},
		{"struct field", defaultDrawError[struct{ Value int }], []any{libhegel.OK, int64(0), libhegel.E_BACKEND, "boom"}},
		{"array element", defaultDrawError[[1]int], []any{libhegel.OK, int64(0), libhegel.E_BACKEND, "boom"}},
		{"pointer choice", defaultDrawError[*int], []any{false, libhegel.E_BACKEND, "boom"}},
		{"pointer element", defaultDrawError[*int], []any{true, libhegel.OK, libhegel.OK, int64(0), libhegel.E_BACKEND, "boom"}},
		{"slice construction", defaultDrawError[[]int], []any{uintptr(0), libhegel.E_BACKEND, "boom"}},
		{"slice element", defaultDrawError[[]int], []any{uintptr(1), libhegel.OK, true, libhegel.OK, libhegel.OK, int64(0), libhegel.E_BACKEND, "boom"}},
		{"slice iteration", defaultDrawError[[]int], []any{uintptr(1), libhegel.OK, false, libhegel.E_BACKEND, "boom"}},
		{"map construction", defaultDrawError[map[int]int], []any{uintptr(0), libhegel.E_BACKEND, "boom"}},
		{"map key", defaultDrawError[map[int]int], []any{uintptr(1), libhegel.OK, true, libhegel.OK, libhegel.OK, int64(0), libhegel.E_BACKEND, "boom"}},
		{"map value", defaultDrawError[map[int]int], []any{uintptr(1), libhegel.OK, true, libhegel.OK, libhegel.OK, int64(0), libhegel.OK, libhegel.OK, libhegel.OK, int64(0), libhegel.E_BACKEND, "boom"}},
		{"map iteration", defaultDrawError[map[int]int], []any{uintptr(1), libhegel.OK, false, libhegel.E_BACKEND, "boom"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := test.draw(newStubTestCase(t, test.ops...))
			if !errors.Is(err, libhegel.E_BACKEND) || !strings.Contains(err.Error(), "boom") {
				t.Fatalf("error = %v, want backend error with its message", err)
			}
		})
	}
}

func TestDefaultRejectsDuplicateMapKeys(t *testing.T) {
	tc := newStubTestCase(t,
		uintptr(1), libhegel.OK,
		true, libhegel.OK,
		libhegel.OK, int64(2), libhegel.OK, libhegel.OK,
		libhegel.OK, int64(3), libhegel.OK, libhegel.OK,
		true, libhegel.OK,
		libhegel.OK, int64(2), libhegel.OK, libhegel.OK,
		libhegel.OK,
		true, libhegel.OK,
		libhegel.OK, int64(4), libhegel.OK, libhegel.OK,
		libhegel.OK, int64(5), libhegel.OK, libhegel.OK,
		false, libhegel.OK,
	)
	got, err := Default[map[int]int]().draw(tc)
	if err != nil || !reflect.DeepEqual(got, map[int]int{2: 3, 4: 5}) {
		t.Fatalf("got %v, %v; want map[2:3 4:5]", got, err)
	}
}

func TestDefaultInvalidInternalShape(t *testing.T) {
	for _, makeDraw := range []func() defaultDraw{
		func() defaultDraw {
			return compileDefault(&defaultShape{typ: reflect.TypeFor[chan int]()}, make(map[*defaultShape]defaultDraw))
		},
		func() defaultDraw { return defaultPrimitive(reflect.TypeFor[chan int]()) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid internal shape did not panic")
				}
			}()
			makeDraw()
		}()
	}
}

func TestDefaultEmptyCollectionBounds(t *testing.T) {
	for _, test := range []struct {
		name string
		draw func(TestCase) error
	}{
		{"slice", defaultDrawError[[]int]},
		{"map", defaultDrawError[map[int]int]},
	} {
		t.Run(test.name, func(t *testing.T) {
			tc := &collectionTestCase{TestCase: newStubTestCase(t,
				uintptr(1), libhegel.OK, false, libhegel.OK,
			)}
			if err := test.draw(tc); err != nil {
				t.Fatal(err)
			}
			if tc.minSize != 0 || tc.maxSize != nil {
				t.Fatalf("collection bounds = %d, %v; want 0, nil", tc.minSize, tc.maxSize)
			}
		})
	}
}

func TestDefaultUnsignedShrinksToZero(t *testing.T) {
	checkDefaultUnsignedZero[uint](t)
	checkDefaultUnsignedZero[uint8](t)
	checkDefaultUnsignedZero[uint16](t)
	checkDefaultUnsignedZero[uint32](t)
	checkDefaultUnsignedZero[uint64](t)
	checkDefaultUnsignedZero[uintptr](t)
}

func checkDefaultUnsignedZero[T integer](t *testing.T) {
	t.Helper()
	gen := Default[T]()
	var minimal T
	err := run(1, func(tc TestCase) {
		minimal = Draw(tc, gen)
		tc.FailNow()
	}, WithTestCases(1))
	if err == nil || minimal != 0 {
		t.Fatalf("%s: minimal value = %v, error = %v; want zero and failure", reflect.TypeFor[T](), minimal, err)
	}
}

func TestDefaultOverridesByExactType(t *testing.T) {
	type record struct {
		Value  int
		Named  defaultNamedInt
		Pair   [2]int
		Values []int
		Lookup map[int]defaultNamedInt
		Ptr    *int
		Left   defaultLeaf
		Right  defaultLeaf
	}
	gen := Default[record](
		WithGenerator(Just(7)),
		WithGenerator(Just(defaultNamedInt(9))),
	)
	sawSlice, sawMap, sawPointer := false, false, false
	Test(t, func(tc *T) {
		v := Draw(tc, gen)
		if v.Value != 7 || v.Named != 9 || v.Pair != [2]int{7, 7} || v.Left.Value != 7 || v.Right.Value != 7 {
			tc.Fatalf("overrides did not apply to fields: %+v", v)
		}
		for _, value := range v.Values {
			if value != 7 {
				tc.Fatalf("slice element = %d, want 7", value)
			}
		}
		for key, value := range v.Lookup {
			if key != 7 || value != 9 {
				tc.Fatalf("map entry = %d: %d, want 7: 9", key, value)
			}
		}
		if v.Ptr != nil && *v.Ptr != 7 {
			tc.Fatalf("pointer element = %d, want 7", *v.Ptr)
		}
		sawSlice = sawSlice || len(v.Values) > 0
		sawMap = sawMap || len(v.Lookup) > 0
		sawPointer = sawPointer || v.Ptr != nil
	}, WithTestCases(50))
	if !sawSlice || !sawMap || !sawPointer {
		t.Fatalf("missing collection or pointer coverage: slice=%v map=%v pointer=%v", sawSlice, sawMap, sawPointer)
	}
}

func TestDefaultOverridesBypassCache(t *testing.T) {
	cached := Default[int]()
	option := WithGenerator(Composite(func(TestCase) int { return 42 }))
	first := Default[int](option)
	second := Default[int](WithGenerator(Just(99)))
	repeated := Default[int](option)
	if first == repeated {
		t.Fatal("calls with options reused a generator")
	}
	if Default[int]() != cached {
		t.Fatal("overrides replaced the cached generator")
	}
	Test(t, func(tc *T) {
		if got := Draw(tc, first); got != 42 {
			tc.Fatalf("first override = %d, want 42", got)
		}
		if got := Draw(tc, second); got != 99 {
			tc.Fatalf("second override = %d, want 99", got)
		}
		if got := Draw(tc, repeated); got != 42 {
			tc.Fatalf("repeated override = %d, want 42", got)
		}
	}, WithTestCases(1))
}

func TestDefaultOverridePrecedence(t *testing.T) {
	want := time.Date(2000, 1, 2, 3, 4, 5, 0, time.UTC)
	gen := Default[time.Time](WithGenerator(Just(time.Time{})), WithGenerator(Just(want)))
	Test(t, func(tc *T) {
		if got := Draw(tc, gen); got != want {
			tc.Fatalf("time = %v, want %v", got, want)
		}
	}, WithTestCases(1))
}

func TestDefaultOverridesUnsupportedTypes(t *testing.T) {
	type private struct{ value int }
	type record struct {
		Tree    defaultTree
		Private private
		Channel chan int
		Value   any
	}
	channel := make(chan int)
	want := record{Tree: defaultTree{Value: 12}, Private: private{value: 34}, Channel: channel, Value: "custom"}
	gen := Default[record](
		WithGenerator(Just(want.Tree)),
		WithGenerator(Just(want.Private)),
		WithGenerator(Just(channel)),
		WithGenerator(Just[any](want.Value)),
	)
	Test(t, func(tc *T) {
		if got := Draw(tc, gen); got != want {
			tc.Fatalf("record = %+v, want %+v", got, want)
		}
		if got := Draw(tc, Default[any](WithGenerator(Just[any](nil)))); got != nil {
			tc.Fatalf("interface = %v, want nil", got)
		}
		if got := Draw(tc, Default[struct{ Value any }](WithGenerator(Just[any](nil)))); got.Value != nil {
			tc.Fatalf("interface field = %v, want nil", got.Value)
		}
	}, WithTestCases(1))
}

func TestDefaultRejectsInvalidOptions(t *testing.T) {
	for _, test := range []struct {
		name string
		make func()
		want string
	}{
		{"zero option", func() { Default[int](DefaultOption{}) }, "invalid zero-value option"},
		{"unhandled type", func() { Default[chan int](WithGenerator(Just(1))) }, "unsupported kind chan"},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				p := recover()
				if p == nil || !strings.Contains(p.(string), test.want) {
					t.Fatalf("panic = %v, want %q", p, test.want)
				}
			}()
			test.make()
		})
	}
}

func TestDefaultOverridePropagatesDrawError(t *testing.T) {
	gen := Default[int](WithGenerator(Integers(0, 10)))
	tc := newStubTestCase(t, libhegel.OK, int64(0), libhegel.E_BACKEND, "override failed")
	_, err := gen.draw(tc)
	if !errors.Is(err, libhegel.E_BACKEND) || !strings.Contains(err.Error(), "override failed") {
		t.Fatalf("error = %v, want backend error with its message", err)
	}
}

func TestDefaultFieldOverrides(t *testing.T) {
	type other struct{ Value int }
	type record struct {
		Left, Right defaultLeaf
		Other       other
	}
	for _, options := range [][]DefaultOption{
		{WithGenerator(Just(3)), WithFieldGenerator[defaultLeaf]("Value", Just(7)), WithFieldGenerator[defaultLeaf]("Value", Just(9))},
		{WithFieldGenerator[defaultLeaf]("Value", Just(7)), WithFieldGenerator[defaultLeaf]("Value", Just(9)), WithGenerator(Just(3))},
	} {
		gen := Default[record](options...)
		Test(t, func(tc *T) {
			want := record{Left: defaultLeaf{3}, Right: defaultLeaf{3}, Other: other{3}}
			if got := Draw(tc, gen); got != want {
				tc.Fatalf("record = %+v, want %+v", got, want)
			}
		}, WithTestCases(1))
	}
	fieldGen := Default[record](WithFieldGenerator[defaultLeaf]("Value", Just(7)), WithFieldGenerator[defaultLeaf]("Value", Just(9)), WithFieldGenerator[other]("Value", Just(5)))
	Test(t, func(tc *T) {
		want := record{Left: defaultLeaf{9}, Right: defaultLeaf{9}, Other: other{5}}
		if got := Draw(tc, fieldGen); got != want {
			tc.Fatalf("field overrides = %+v, want %+v", got, want)
		}
	}, WithTestCases(1))
	gen := Default[defaultLeaf](WithFieldGenerator[defaultLeaf]("Value", Just(9)), WithGenerator(Just(defaultLeaf{42})))
	Test(t, func(tc *T) {
		if got := Draw(tc, gen); got.Value != 42 {
			tc.Fatalf("whole-struct override = %+v, want Value 42", got)
		}
	}, WithTestCases(1))
}

func TestDefaultFieldOverridesAreIndependent(t *testing.T) {
	type record struct{ Left, Right int }
	cached := Default[record]()
	first := Default[record](WithFieldGenerator[record]("Left", Just(1)), WithFieldGenerator[record]("Right", Just(2)))
	second := Default[record](WithFieldGenerator[record]("Left", Just(3)), WithFieldGenerator[record]("Right", Just(4)))
	if Default[record]() != cached {
		t.Fatal("field overrides replaced the cached generator")
	}
	Test(t, func(tc *T) {
		if got := Draw(tc, first); got != (record{1, 2}) {
			tc.Fatalf("first = %+v, want {1 2}", got)
		}
		if got := Draw(tc, second); got != (record{3, 4}) {
			tc.Fatalf("second = %+v, want {3 4}", got)
		}
	}, WithTestCases(1))
}

func TestDefaultFieldOverridesUnsupportedTypes(t *testing.T) {
	type record struct {
		Value any
		Next  *record
		Named defaultNamedInt
	}
	gen := Default[record](
		WithFieldGenerator[record]("Value", Just[any](nil)),
		WithFieldGenerator[record]("Next", Just[*record](nil)),
		WithFieldGenerator[record]("Named", Just(defaultNamedInt(7))),
	)
	Test(t, func(tc *T) {
		if got := Draw(tc, gen); got != (record{Named: 7}) {
			tc.Fatalf("record = %+v, want nil interface, nil recursive pointer, and Named 7", got)
		}
	}, WithTestCases(1))
}

func TestWithFieldGeneratorRejectsInvalidFields(t *testing.T) {
	type record struct {
		Value   int
		Named   defaultNamedInt
		private int
	}
	_ = record{private: 1}
	type embedded struct{ defaultLeaf }
	for _, test := range []struct {
		name string
		make func()
		want string
	}{
		{"non-struct", func() { WithFieldGenerator[int]("Value", Just(1)) }, "expected a struct"},
		{"pointer", func() { WithFieldGenerator[*record]("Value", Just(1)) }, "expected a struct"},
		{"missing", func() { WithFieldGenerator[record]("Missing", Just(1)) }, "not a direct field"},
		{"empty", func() { WithFieldGenerator[record]("", Just(1)) }, "not a direct field"},
		{"promoted", func() { WithFieldGenerator[embedded]("Value", Just(1)) }, "not a direct field"},
		{"private", func() { WithFieldGenerator[record]("private", Just(1)) }, "is unexported"},
		{"wrong type", func() { WithFieldGenerator[record]("Value", Just("value")) }, "has type int, want string"},
		{"convertible type", func() { WithFieldGenerator[record]("Named", Just(int16(1))) }, "has type hegel.defaultNamedInt, want int16"},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				p := recover()
				if p == nil || !strings.Contains(p.(string), test.want) {
					t.Fatalf("panic = %v, want %q", p, test.want)
				}
			}()
			test.make()
		})
	}
}

func TestDefaultFieldOverridePropagatesDrawError(t *testing.T) {
	gen := Default[defaultLeaf](WithFieldGenerator[defaultLeaf]("Value", Integers(0, 10)))
	tc := newStubTestCase(t, libhegel.OK, int64(0), libhegel.E_BACKEND, "field override failed")
	_, err := gen.draw(tc)
	if !errors.Is(err, libhegel.E_BACKEND) || !strings.Contains(err.Error(), "field override failed") {
		t.Fatalf("error = %v, want backend error with its message", err)
	}
}
