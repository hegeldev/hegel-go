package hegel

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
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
	result := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			Default[defaultRecord]()
		}
	})
	if result.AllocsPerOp() != 0 {
		t.Fatalf("Default allocated %d times per call, want zero", result.AllocsPerOp())
	}
}

func TestDefaultCachesGeneratorConcurrently(t *testing.T) {
	type record struct{ Value int }
	const workers = 32
	var generators [workers]Generator[record]
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range generators {
		wg.Go(func() {
			<-start
			generators[i] = Default[record]()
		})
	}
	close(start)
	wg.Wait()
	for _, gen := range generators {
		if gen != generators[0] {
			t.Fatal("concurrent calls returned different generators")
		}
	}
}

func TestDefaultCachesSharedAcyclicShape(t *testing.T) {
	shape, err := buildDefault(reflect.TypeFor[defaultShared](), make(map[reflect.Type]*defaultShape), make(map[reflect.Type]bool))
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
