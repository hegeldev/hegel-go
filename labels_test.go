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
	return hashComparable(h, g.value)
}

func mustLabel(tb testing.TB, generator labelGenerator) libhegel.Label {
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

func checkGenerator(tb testing.TB, generator, equivalent labelGenerator, ignored ...string) {
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
		if got, ok := labelFor(mutated); ok && got == want {
			tb.Errorf("mutating %s did not change label %d", field.name, want)
		}
	}
	for name := range ignoredFields {
		tb.Errorf("ignored field %q does not exist", name)
	}
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

func mutateGenerator(tb testing.TB, generator labelGenerator, field generatorField) labelGenerator {
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
	return clone.Interface().(labelGenerator)
}

func increment(value int) int                             { return value + 1 }
func positive(value int) bool                             { return value > 0 }
func intGenerator(value int) Generator[int]               { return Just(value) }
func countInts(values []int) int                          { return len(values) }
func compositeInt(tc TestCase) int                        { return Draw(tc, Integers(0, 10)) }
func recursiveBranch(child Generator[int]) Generator[int] { return Map(Lists(child), countInts) }

func TestGeneratorHashFields(t *testing.T) {
	t.Parallel()
	date := time.Date(2020, 2, 3, 4, 5, 6, 7, time.FixedZone("one", 3600))
	equivalentDate := time.Date(2020, 2, 3, 4, 5, 6, 7, time.FixedZone("two", -3600))
	pool := &Pool[int]{}
	recursion := &libhegel.Recursion{}

	tests := []struct {
		name       string
		generator  labelGenerator
		equivalent labelGenerator
		ignored    []string
	}{
		{"integer", Integers(1, 9), Integers(1, 9), nil},
		{"float", Floats[float64]().Min(1).Max(2).AllowNaN(false).AllowInfinity(false).ExcludeMin().ExcludeMax(), Floats[float64]().Min(1).Max(2).AllowNaN(false).AllowInfinity(false).ExcludeMin().ExcludeMax(), nil},
		{"text", Text().MinSize(1).MaxSize(2).Codec("ascii").MinCodepoint('a').MaxCodepoint('z').Categories([]string{"L"}).ExcludeCategories([]string{"Lu"}).IncludeCharacters("x").ExcludeCharacters("y"), Text().MinSize(1).MaxSize(2).Codec("ascii").MinCodepoint('a').MaxCodepoint('z').Categories([]string{"L"}).ExcludeCategories([]string{"Lu"}).IncludeCharacters("x").ExcludeCharacters("y"), nil},
		{"characters", Characters().Codec("ascii").MinCodepoint('a').MaxCodepoint('z').Categories([]string{"L"}).ExcludeCategories([]string{"Lu"}).IncludeCharacters("x").ExcludeCharacters("y"), Characters().Codec("ascii").MinCodepoint('a').MaxCodepoint('z').Categories([]string{"L"}).ExcludeCategories([]string{"Lu"}).IncludeCharacters("x").ExcludeCharacters("y"), nil},
		{"domain", Domains().MaxLength(20), Domains().MaxLength(20), nil},
		{"date", Dates().Min(date).Max(date), Dates().Min(equivalentDate).Max(equivalentDate), nil},
		{"datetime", Datetimes().Min(date).Max(date), Datetimes().Min(equivalentDate).Max(equivalentDate), nil},
		{"list", Lists(Integers(0, 10)).MinSize(1).MaxSize(3), Lists(Integers(0, 10)).MinSize(1).MaxSize(3), nil},
		{"map", Maps(Integers(0, 10), Integers(0, 10)).MinSize(1).MaxSize(3), Maps(Integers(0, 10), Integers(0, 10)).MinSize(1).MaxSize(3), nil},
		{"one-of", OneOf(Integers(0, 1), Integers(2, 3)), OneOf(Integers(0, 1), Integers(2, 3)), nil},
		{"optional", Optional(Integers(0, 10)), Optional(Integers(0, 10)), nil},
		{"ip-address", IPAddresses().IPv4(), IPAddresses().IPv4(), nil},
		{"composite", Composite(compositeInt), Composite(compositeInt), nil},
		{"map-function", Map(Integers(0, 10), increment), Map(Integers(0, 10), increment), nil},
		{"filter", Filter(Integers(0, 10), positive), Filter(Integers(0, 10), positive), nil},
		{"flat-map", FlatMap(Integers(0, 10), intGenerator), FlatMap(Integers(0, 10), intGenerator), nil},
		{"recursive", Recursive(Integers(0, 10), recursiveBranch).MaxDepth(2).MaxLeaves(3), Recursive(Integers(0, 10), recursiveBranch).MaxDepth(2).MaxLeaves(3), nil},
		{"subtree", &subtreeGenerator[int]{leaf: Integers(0, 10), branch: recursiveBranch, recursion: recursion, depth: 2}, &subtreeGenerator[int]{leaf: Integers(0, 10), branch: recursiveBranch, recursion: recursion, depth: 2}, []string{"recursion"}},
		{"pool", poolGenerator[int]{pool: pool, consume: true}, poolGenerator[int]{pool: pool, consume: true}, nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkGenerator(t, test.generator, test.equivalent, test.ignored...)
		})
	}
}

func TestGeneratorTypeSeparatesLabels(t *testing.T) {
	t.Parallel()
	if a, b := mustLabel(t, Domains()), mustLabel(t, IPAddresses()); a == b {
		t.Fatalf("different generator types share label %d", a)
	}
}

func TestFunctionGeneratorConfigurationChangesLabels(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		left  labelGenerator
		right labelGenerator
	}{
		{"integer minimum", Integers(0, 10), Integers(1, 10)},
		{"integer maximum", Integers(0, 10), Integers(0, 11)},
		{"boolean weight", WeightedBooleans(0.25), WeightedBooleans(0.75)},
		{"binary minimum", Binary(0, 10), Binary(1, 10)},
		{"binary maximum", Binary(0, 10), Binary(0, 11)},
		{"regex pattern", FromRegex("a", true), FromRegex("b", true)},
		{"regex mode", FromRegex("a", true), FromRegex("a", false)},
		{"constant", Just(1), Just(2)},
		{"sampled value", SampledFrom([]int{1, 2}), SampledFrom([]int{1, 3})},
		{"function body", Emails(), URLs()},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if left, right := mustLabel(t, test.left), mustLabel(t, test.right); left == right {
				t.Fatalf("different configurations share label %d", left)
			}
		})
	}
}

func TestUnhashableGeneratorHasNoLabel(t *testing.T) {
	t.Parallel()
	if _, ok := labelFor(Just([]int{1})); ok {
		t.Fatal("Just with an unhashable value has a label")
	}
}

func TestNaNGeneratorHasNoLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		generator labelGenerator
	}{
		{"direct", Just(math.NaN())},
		{"struct", Just(struct{ value float64 }{math.NaN()})},
		{"interface", Just(struct{ value any }{math.NaN()})},
		{"array", Just([1]float64{math.NaN()})},
		{"complex", Just(complex(math.NaN(), 0))},
		{"sampled", SampledFrom([]float64{math.NaN()})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := labelFor(test.generator); ok {
				t.Fatal("generator containing NaN has a label")
			}
		})
	}
}

func TestHashHelpersRejectUnsupportedValues(t *testing.T) {
	t.Parallel()
	var h maphash.Hash
	h.SetSeed(maphash.MakeSeed())
	if hashFunction(&h, 1) {
		t.Fatal("non-function was hashable as a function")
	}
	if hashValues(&h, []int{1}) {
		t.Fatal("non-comparable value was hashable")
	}
	if _, ok := labelFor(OneOf(Just([]int{1}))); ok {
		t.Fatal("OneOf with an unlabelable child has a label")
	}
}

func TestUnhashableGeneratorDrawHasNoSpan(t *testing.T) {
	t.Parallel()
	tc := &labelRecordingTestCase{}
	if got := Draw(tc, Just([]int{1})); !slices.Equal(got, []int{1}) {
		t.Fatalf("draw = %v, want [1]", got)
	}
	if len(tc.labels) != 0 {
		t.Fatalf("labels = %v, want none", tc.labels)
	}
}
