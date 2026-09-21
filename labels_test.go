package hegel

import (
	"fmt"
	"hash/maphash"
	"math"
	"reflect"
	"slices"
	"testing"
	"time"
	"unsafe"

	"hegel.dev/go/hegel/internal/libhegel"
)

type labelRecordingTestCase struct {
	TestCase
	labels []libhegel.Label
	depth  int
}

func (tc *labelRecordingTestCase) startSpan(l libhegel.Label) error {
	tc.labels = append(tc.labels, l)
	tc.depth++
	return nil
}

func (tc *labelRecordingTestCase) stopSpan(bool) error {
	tc.depth--
	return nil
}

func (tc *labelRecordingTestCase) inSpan() bool { return tc.depth > 0 }

func (tc *labelRecordingTestCase) reportDraw(int, any) {}

type labelProbeGenerator struct{ value int }

func (g labelProbeGenerator) draw(tc TestCase) (int, error) {
	if !tc.inSpan() {
		return 0, fmt.Errorf("generator draw is not inside a span")
	}
	return g.value, nil
}

func (g labelProbeGenerator) hashFields(h *maphash.Hash) bool {
	return hashValue(h, g.value)
}

func mustLabel[T any](tb testing.TB, generator Generator[T]) libhegel.Label {
	tb.Helper()
	label, ok := labelFor(generator)
	if !ok {
		tb.Fatal("generator has no label")
	}
	return label
}

func TestDrawLabelsGeneratorSpan(t *testing.T) {
	t.Parallel()
	tc := &labelRecordingTestCase{}

	if got := Draw[int](tc, labelProbeGenerator{value: 1}); got != 1 {
		t.Fatalf("first draw = %d, want 1", got)
	}
	Draw[int](tc, labelProbeGenerator{value: 1})
	Draw[int](tc, labelProbeGenerator{value: 2})

	if tc.depth != 0 {
		t.Fatalf("span depth = %d, want 0", tc.depth)
	}
	if len(tc.labels) != 3 {
		t.Fatalf("span labels = %v, want three", tc.labels)
	}
	if tc.labels[0] != tc.labels[1] {
		t.Fatalf("equal generators have labels %d and %d", tc.labels[0], tc.labels[1])
	}
	if tc.labels[0] == tc.labels[2] {
		t.Fatalf("different generators share label %d", tc.labels[0])
	}
}

func TestDrawLabelsInnerGenerator(t *testing.T) {
	t.Parallel()
	tc := &labelRecordingTestCase{}
	inner := labelProbeGenerator{value: 1}
	outer := Map(inner, increment)

	if got := Draw(tc, outer); got != 2 {
		t.Fatalf("draw = %d, want 2", got)
	}
	want := []libhegel.Label{mustLabel(t, outer), mustLabel(t, inner)}
	if !slices.Equal(tc.labels, want) {
		t.Fatalf("span labels = %v, want %v", tc.labels, want)
	}
}

type generatorField struct {
	name  string
	index []int
}

func checkGenerator[T any](tb testing.TB, generator, equivalent Generator[T], ignored ...string) {
	tb.Helper()
	want := mustLabel(tb, generator)
	if got := mustLabel(tb, generator); got != want {
		tb.Fatalf("label is unstable: got %d, want %d", got, want)
	}
	if got := mustLabel(tb, equivalent); got != want {
		tb.Fatalf("equivalent generator label = %d, want %d", got, want)
	}

	ignoredFields := make(map[string]bool, len(ignored))
	for _, name := range ignored {
		ignoredFields[name] = true
	}
	for _, field := range generatorFields(reflect.TypeOf(generator)) {
		if ignoredFields[field.name] {
			delete(ignoredFields, field.name)
			continue
		}
		mutated := mutateGenerator(tb, generator, field)
		if got, ok := recoverLabel(mutated); ok && got == want {
			tb.Errorf("mutating %s did not change label %d", field.name, want)
		}
	}
	for name := range ignoredFields {
		tb.Errorf("ignored field %q does not exist", name)
	}
}

func recoverLabel[T any](generator Generator[T]) (label libhegel.Label, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return labelFor(generator)
}

func generatorFields(generatorType reflect.Type) []generatorField {
	if generatorType.Kind() == reflect.Pointer {
		generatorType = generatorType.Elem()
	}
	var result []generatorField
	var walk func(reflect.Type, string, []int)
	walk = func(current reflect.Type, prefix string, index []int) {
		for i := range current.NumField() {
			field := current.Field(i)
			name := field.Name
			if prefix != "" {
				name = prefix + "." + name
			}
			fieldIndex := append(append([]int(nil), index...), i)
			if field.Type.Kind() == reflect.Struct && field.Type.PkgPath() == current.PkgPath() {
				walk(field.Type, name, fieldIndex)
				continue
			}
			result = append(result, generatorField{name: name, index: fieldIndex})
		}
	}
	walk(generatorType, "", nil)
	return result
}

func mutateGenerator[T any](tb testing.TB, generator Generator[T], field generatorField) Generator[T] {
	tb.Helper()
	original := reflect.ValueOf(generator)
	var clone, root reflect.Value
	if original.Kind() == reflect.Pointer {
		clone = reflect.New(original.Type().Elem())
		clone.Elem().Set(original.Elem())
		root = clone.Elem()
	} else {
		holder := reflect.New(original.Type())
		holder.Elem().Set(original)
		clone = holder.Elem()
		root = holder.Elem()
	}
	for _, index := range field.index {
		root = root.Field(index)
	}
	root = reflect.NewAt(root.Type(), unsafe.Pointer(root.UnsafeAddr())).Elem()

	switch root.Kind() {
	case reflect.Bool:
		root.SetBool(!root.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		root.SetInt(root.Int() + 1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		root.SetUint(root.Uint() + 1)
	case reflect.Float32, reflect.Float64:
		root.SetFloat(math.Nextafter(root.Float(), math.Inf(1)))
	case reflect.String:
		root.SetString(root.String() + "x")
	case reflect.Func, reflect.Interface, reflect.Pointer:
		root.SetZero()
	case reflect.Slice:
		if root.IsNil() {
			root.Set(reflect.MakeSlice(root.Type(), 1, 1))
		} else {
			root.SetZero()
		}
	default:
		tb.Fatalf("no mutation heuristic for %s field %s", root.Kind(), field.name)
	}
	return clone.Interface().(Generator[T])
}

func increment(value int) int                             { return value + 1 }
func positive(value int) bool                             { return value > 0 }
func intGenerator(value int) Generator[int]               { return Just(value) }
func countInts(values []int) int                          { return len(values) }
func compositeInt(tc TestCase) int                        { return Draw(tc, Integers(0, 10)) }
func recursiveBranch(child Generator[int]) Generator[int] { return Map(Lists(child), countInts) }

func generatorCheck[T any](generator, equivalent Generator[T], ignored ...string) func(*testing.T) {
	return func(t *testing.T) {
		checkGenerator(t, generator, equivalent, ignored...)
	}
}

func TestGeneratorHashFields(t *testing.T) {
	t.Parallel()
	date := time.Date(2020, 2, 3, 4, 5, 6, 7, time.FixedZone("one", 3600))
	equivalentDate := time.Date(2020, 2, 3, 4, 5, 6, 7, time.FixedZone("two", -3600))
	pool := &Pool[int]{}
	recursion := &libhegel.Recursion{}

	tests := []struct {
		name  string
		check func(*testing.T)
	}{
		{"integer", generatorCheck(Integers(1, 9), Integers(1, 9), "drawFunc")},
		{"float", generatorCheck(Floats[float64]().Min(1).Max(2).AllowNaN(false).AllowInfinity(false).ExcludeMin().ExcludeMax(), Floats[float64]().Min(1).Max(2).AllowNaN(false).AllowInfinity(false).ExcludeMin().ExcludeMax())},
		{"text", generatorCheck(Text().MinSize(1).MaxSize(2).Codec("ascii").MinCodepoint('a').MaxCodepoint('z').Categories([]string{"L"}).ExcludeCategories([]string{"Lu"}).IncludeCharacters("x").ExcludeCharacters("y"), Text().MinSize(1).MaxSize(2).Codec("ascii").MinCodepoint('a').MaxCodepoint('z').Categories([]string{"L"}).ExcludeCategories([]string{"Lu"}).IncludeCharacters("x").ExcludeCharacters("y"))},
		{"characters", generatorCheck(Characters().Codec("ascii").MinCodepoint('a').MaxCodepoint('z').Categories([]string{"L"}).ExcludeCategories([]string{"Lu"}).IncludeCharacters("x").ExcludeCharacters("y"), Characters().Codec("ascii").MinCodepoint('a').MaxCodepoint('z').Categories([]string{"L"}).ExcludeCategories([]string{"Lu"}).IncludeCharacters("x").ExcludeCharacters("y"))},
		{"domain", generatorCheck(Domains().MaxLength(20), Domains().MaxLength(20))},
		{"date", generatorCheck(Dates().Min(date).Max(date), Dates().Min(equivalentDate).Max(equivalentDate))},
		{"datetime", generatorCheck(Datetimes().Min(date).Max(date), Datetimes().Min(equivalentDate).Max(equivalentDate))},
		{"list", generatorCheck(Lists(Integers(0, 10)).MinSize(1).MaxSize(3), Lists(Integers(0, 10)).MinSize(1).MaxSize(3))},
		{"map", generatorCheck(Maps(Integers(0, 10), Integers(0, 10)).MinSize(1).MaxSize(3), Maps(Integers(0, 10), Integers(0, 10)).MinSize(1).MaxSize(3))},
		{"one-of", generatorCheck(OneOf(Integers(0, 1), Integers(2, 3)), OneOf(Integers(0, 1), Integers(2, 3)))},
		{"optional", generatorCheck(Optional(Integers(0, 10)), Optional(Integers(0, 10)))},
		{"ip-address", generatorCheck(IPAddresses().IPv4(), IPAddresses().IPv4())},
		{"composite", generatorCheck(Composite(compositeInt), Composite(compositeInt))},
		{"map-function", generatorCheck(Map(Integers(0, 10), increment), Map(Integers(0, 10), increment))},
		{"filter", generatorCheck(Filter(Integers(0, 10), positive), Filter(Integers(0, 10), positive))},
		{"flat-map", generatorCheck(FlatMap(Integers(0, 10), intGenerator), FlatMap(Integers(0, 10), intGenerator))},
		{"recursive", generatorCheck(Recursive(Integers(0, 10), recursiveBranch).MaxDepth(2).MaxLeaves(3), Recursive(Integers(0, 10), recursiveBranch).MaxDepth(2).MaxLeaves(3))},
		{"subtree", generatorCheck(&subtreeGenerator[int]{leaf: Integers(0, 10), branch: recursiveBranch, recursion: recursion, depth: 2}, &subtreeGenerator[int]{leaf: Integers(0, 10), branch: recursiveBranch, recursion: recursion, depth: 2}, "recursion")},
		{"pool", generatorCheck(poolGenerator[int]{pool: pool, consume: true}, poolGenerator[int]{pool: pool, consume: true})},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.check(t)
		})
	}
}

func TestGeneratorTypeSeparatesLabels(t *testing.T) {
	t.Parallel()
	if a, b := mustLabel(t, Domains()), mustLabel(t, IPAddresses()); a == b {
		t.Fatalf("different generator types share label %d", a)
	}
}

func differentLabelCheck[T any](left, right Generator[T]) func(*testing.T) {
	return func(t *testing.T) {
		if left, right := mustLabel(t, left), mustLabel(t, right); left == right {
			t.Fatalf("different configurations share label %d", left)
		}
	}
}

func TestFunctionGeneratorConfigurationChangesLabels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		check func(*testing.T)
	}{
		{"integer minimum", differentLabelCheck(Integers(0, 10), Integers(1, 10))},
		{"integer maximum", differentLabelCheck(Integers(0, 10), Integers(0, 11))},
		{"boolean weight", differentLabelCheck(WeightedBooleans(0.25), WeightedBooleans(0.75))},
		{"binary minimum", differentLabelCheck(Binary(0, 10), Binary(1, 10))},
		{"binary maximum", differentLabelCheck(Binary(0, 10), Binary(0, 11))},
		{"regex pattern", differentLabelCheck(FromRegex("a", true), FromRegex("b", true))},
		{"regex mode", differentLabelCheck(FromRegex("a", true), FromRegex("a", false))},
		{"function body", differentLabelCheck(Emails(), URLs())},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.check(t)
		})
	}
}

func TestConstantAndSampledLabelsIgnoreValues(t *testing.T) {
	t.Parallel()
	if left, right := mustLabel(t, Just(1)), mustLabel(t, Just(2)); left != right {
		t.Fatalf("constants of the same type have labels %d and %d", left, right)
	}
	if left, right := mustLabel(t, SampledFrom([]int{1})), mustLabel(t, SampledFrom([]int{2, 3})); left != right {
		t.Fatalf("samples of the same type have labels %d and %d", left, right)
	}
	if left, right := mustLabel(t, Just(1)), mustLabel(t, Just(int64(1))); left == right {
		t.Fatalf("constants of different types share label %d", left)
	}
}

func valueHash(t *testing.T, value any) uint64 {
	t.Helper()
	var h maphash.Hash
	h.SetSeed(labelSeed)
	if !hashValue(&h, value) {
		t.Fatalf("%T is not hashable", value)
	}
	return h.Sum64()
}

func TestHashValueUsesFloatBits(t *testing.T) {
	t.Parallel()
	for _, value := range []any{
		float32(math.NaN()),
		math.NaN(),
		complex(float32(math.NaN()), float32(1)),
		complex(math.NaN(), 1),
	} {
		if left, right := valueHash(t, value), valueHash(t, value); left != right {
			t.Fatalf("%T hash is unstable: %d != %d", value, left, right)
		}
	}
	if positive, negative := valueHash(t, 0.0), valueHash(t, math.Copysign(0, -1)); positive == negative {
		t.Fatalf("positive and negative zero share hash %d", positive)
	}
}

type unhashableGenerator struct{ value []int }

func (g unhashableGenerator) draw(TestCase) ([]int, error) { return g.value, nil }

func (g unhashableGenerator) hashFields(h *maphash.Hash) bool { return hashValue(h, g.value) }

func TestHashValueRejectsUnsupportedValues(t *testing.T) {
	t.Parallel()
	var h maphash.Hash
	h.SetSeed(labelSeed)
	_ = valueHash(t, nil)
	if hashValue(&h, []int{1}) {
		t.Fatal("non-comparable value was hashable")
	}
	if hashValues(&h, []int{1}) {
		t.Fatal("hashValues accepted a non-comparable value")
	}
	if hashSlice(&h, [][]int{{1}}) {
		t.Fatal("hashSlice accepted a non-comparable element")
	}
	if _, ok := labelFor(OneOf[[]int](unhashableGenerator{value: []int{1}})); ok {
		t.Fatal("OneOf with an unlabelable child has a label")
	}
}

func TestUnhashableGeneratorDrawHasNoSpan(t *testing.T) {
	t.Parallel()
	tc := &labelRecordingTestCase{}
	if got := Draw(tc, unhashableGenerator{value: []int{1}}); !slices.Equal(got, []int{1}) {
		t.Fatalf("draw = %v, want [1]", got)
	}
	if len(tc.labels) != 0 {
		t.Fatalf("labels = %v, want none", tc.labels)
	}
}
