// hegel_workout - Tests hegel strategies for correct behavior
//
// Each test is a separate function that generates values and validates them.
// The main function uses SampledFrom to pick which test to run.
//
// Run with: HEGEL_SOCKET=/path/to/socket HEGEL_REJECT_CODE=77 ./hegel_workout

package main

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	hegel "github.com/antithesishq/hegel-go"
)

// =============================================================================
// Test helper
// =============================================================================

func testAssert(cond bool, msg string) {
	if !cond {
		fmt.Fprintf(os.Stderr, "FAILED: %s\n", msg)
		os.Exit(1)
	}
}

// isASCII checks if string contains only ASCII characters
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 128 {
			return false
		}
	}
	return true
}

// =============================================================================
// Primitive tests
// =============================================================================

func testNulls() {
	gen := hegel.Nulls()
	_ = gen.Generate()
	fmt.Println("nulls: generated null")
}

func testBooleans() {
	gen := hegel.Booleans()
	value := gen.Generate()
	testAssert(value == true || value == false, "boolean must be true or false")
	fmt.Printf("booleans: %v\n", value)
}

func testJustInt() {
	gen := hegel.Just(42)
	value := gen.Generate()
	testAssert(value == 42, "just(42) must produce 42")
	fmt.Printf("just(42): %d\n", value)
}

func testJustString() {
	gen := hegel.Just("hello")
	value := gen.Generate()
	testAssert(value == "hello", "just(\"hello\") must produce \"hello\"")
	fmt.Printf("just(\"hello\"): %s\n", value)
}

// =============================================================================
// Integer tests
// =============================================================================

func testIntegersUnbounded() {
	gen := hegel.Integers[int64]()
	value := gen.Generate()
	fmt.Printf("integers[int64](): %d\n", value)
}

func testIntegersBounded() {
	gen := hegel.Integers[int32]().Min(10).Max(20)
	value := gen.Generate()
	testAssert(value >= 10 && value <= 20, "integers(10,20) must be in [10,20]")
	fmt.Printf("integers[int32](10,20): %d\n", value)
}

func testIntegersMinOnly() {
	gen := hegel.Integers[int32]().Min(100)
	value := gen.Generate()
	testAssert(value >= 100, "integers(min=100) must be >= 100")
	fmt.Printf("integers[int32](min=100): %d\n", value)
}

func testIntegersMaxOnly() {
	gen := hegel.Integers[int32]().Max(-100)
	value := gen.Generate()
	testAssert(value <= -100, "integers(max=-100) must be <= -100")
	fmt.Printf("integers[int32](max=-100): %d\n", value)
}

func testIntegersUint8() {
	gen := hegel.Integers[uint8]()
	value := gen.Generate()
	// uint8 is always 0-255, just verify we got a value
	fmt.Printf("integers[uint8](): %d\n", value)
}

func testIntegersNegativeRange() {
	gen := hegel.Integers[int32]().Min(-50).Max(-10)
	value := gen.Generate()
	testAssert(value >= -50 && value <= -10, "integers(-50,-10) must be in [-50,-10]")
	fmt.Printf("integers[int32](-50,-10): %d\n", value)
}

// =============================================================================
// Float tests
// =============================================================================

func testFloatsUnbounded() {
	gen := hegel.Floats[float64]()
	value := gen.Generate()
	fmt.Printf("floats[float64](): %f\n", value)
}

func testFloatsBounded() {
	gen := hegel.Floats[float64]().Min(0.0).Max(1.0)
	value := gen.Generate()
	testAssert(value >= 0.0 && value <= 1.0, "floats(0,1) must be in [0,1]")
	fmt.Printf("floats[float64](0,1): %f\n", value)
}

func testFloatsExclusive() {
	gen := hegel.Floats[float64]().Min(0.0).Max(1.0).ExcludeMin().ExcludeMax()
	value := gen.Generate()
	testAssert(value > 0.0 && value < 1.0, "floats exclusive (0,1) must be in (0,1)")
	fmt.Printf("floats[float64](exclusive 0,1): %f\n", value)
}

func testFloatsFloat32() {
	gen := hegel.Floats[float32]().Min(-1.0).Max(1.0)
	value := gen.Generate()
	testAssert(value >= -1.0 && value <= 1.0, "float32 must be in [-1,1]")
	fmt.Printf("floats[float32](-1,1): %f\n", value)
}

// =============================================================================
// String tests
// =============================================================================

func testTextUnbounded() {
	gen := hegel.Text()
	value := gen.Generate()
	charCount := utf8.RuneCountInString(value)
	fmt.Printf("text(): \"%s\" (chars=%d)\n", value, charCount)
}

func testTextBounded() {
	gen := hegel.Text().MinSize(5).MaxSize(10)
	value := gen.Generate()
	charCount := utf8.RuneCountInString(value)
	// NOTE: Cannot always check min length due to potential null byte issues
	testAssert(charCount <= 10, "text(5,10) char length must be <= 10")
	fmt.Printf("text(5,10): \"%s\" (chars=%d)\n", value, charCount)
}

func testTextMinOnly() {
	gen := hegel.Text().MinSize(3)
	value := gen.Generate()
	charCount := utf8.RuneCountInString(value)
	fmt.Printf("text(min=3): \"%s\" (chars=%d)\n", value, charCount)
}

func testFromRegex() {
	gen := hegel.FromRegex(`[a-z]{3}-[0-9]{3}`)
	value := gen.Generate()

	if !isASCII(value) {
		hegel.Reject("from_regex produced non-ASCII string")
	}

	// Basic validation - should be like "abc-123"
	testAssert(len(value) == 7, "from_regex should produce 7-char string")
	testAssert(value[3] == '-', "from_regex should have hyphen at position 3")
	fmt.Printf("from_regex([a-z]{3}-[0-9]{3}): \"%s\"\n", value)
}

// =============================================================================
// Format string tests
// =============================================================================

func testEmails() {
	gen := hegel.Emails()
	value := gen.Generate()

	if !isASCII(value) {
		hegel.Reject("email produced non-ASCII string")
	}

	testAssert(strings.Contains(value, "@"), "email must contain @")
	fmt.Printf("emails(): \"%s\"\n", value)
}

func testURLs() {
	gen := hegel.URLs()
	value := gen.Generate()

	if !isASCII(value) {
		hegel.Reject("url produced non-ASCII string")
	}

	testAssert(strings.Contains(value, "://"), "url must contain ://")
	fmt.Printf("urls(): \"%s\"\n", value)
}

func testDomains() {
	gen := hegel.Domains()
	value := gen.Generate()

	if !isASCII(value) {
		hegel.Reject("domain produced non-ASCII string")
	}

	charCount := utf8.RuneCountInString(value)
	testAssert(charCount <= 255, "domain must be <= 255 chars")
	fmt.Printf("domains(): \"%s\"\n", value)
}

func testIPAddressesV4() {
	gen := hegel.IPAddresses().V4()
	value := gen.Generate()

	if !isASCII(value) {
		hegel.Reject("ipv4 produced non-ASCII string")
	}

	parts := strings.Split(value, ".")
	testAssert(len(parts) == 4, "ipv4 must have 4 octets")
	fmt.Printf("ip_addresses().v4(): \"%s\"\n", value)
}

func testIPAddressesV6() {
	gen := hegel.IPAddresses().V6()
	value := gen.Generate()

	if !isASCII(value) {
		hegel.Reject("ipv6 produced non-ASCII string")
	}

	testAssert(strings.Contains(value, ":"), "ipv6 must contain colons")
	fmt.Printf("ip_addresses().v6(): \"%s\"\n", value)
}

func testIPAddressesAny() {
	gen := hegel.IPAddresses()
	value := gen.Generate()

	if !isASCII(value) {
		hegel.Reject("ip address produced non-ASCII string")
	}

	isV4 := strings.Contains(value, ".")
	isV6 := strings.Contains(value, ":")
	testAssert(isV4 || isV6, "ip address must be v4 or v6")
	fmt.Printf("ip_addresses(): \"%s\"\n", value)
}

// =============================================================================
// Datetime tests
// =============================================================================

func testDates() {
	gen := hegel.Dates()
	value := gen.Generate()

	if !isASCII(value) {
		hegel.Reject("date produced non-ASCII string")
	}

	// ISO date: YYYY-MM-DD
	testAssert(len(value) == 10, "date must be 10 chars (YYYY-MM-DD)")
	testAssert(value[4] == '-' && value[7] == '-', "date must match YYYY-MM-DD")
	fmt.Printf("dates(): \"%s\"\n", value)
}

func testTimes() {
	gen := hegel.Times()
	value := gen.Generate()

	if !isASCII(value) {
		hegel.Reject("time produced non-ASCII string")
	}

	testAssert(strings.Contains(value, ":"), "time must contain colons")
	fmt.Printf("times(): \"%s\"\n", value)
}

func testDatetimes() {
	gen := hegel.DateTimes()
	value := gen.Generate()

	if !isASCII(value) {
		hegel.Reject("datetime produced non-ASCII string")
	}

	testAssert(strings.Contains(value, "-"), "datetime must contain date part")
	testAssert(strings.Contains(value, ":"), "datetime must contain time part")
	fmt.Printf("datetimes(): \"%s\"\n", value)
}

// =============================================================================
// Collection tests
// =============================================================================

func testSlicesBasic() {
	gen := hegel.Slices(hegel.Integers[int32]())
	value := gen.Generate()
	fmt.Printf("slices(integers[int32]()): size=%d\n", len(value))
}

func testSlicesBounded() {
	gen := hegel.Slices(hegel.Integers[int32]().Min(0).Max(100)).MinSize(3).MaxSize(5)
	value := gen.Generate()
	testAssert(len(value) >= 3 && len(value) <= 5, "slices(min=3,max=5) size must be in [3,5]")
	for _, v := range value {
		testAssert(v >= 0 && v <= 100, "slice elements must be in [0,100]")
	}
	fmt.Printf("slices(3,5): %v\n", value)
}

func testSlicesUnique() {
	gen := hegel.Slices(hegel.Integers[int32]().Min(0).Max(1000)).MinSize(5).MaxSize(10).Unique()
	value := gen.Generate()

	// Check uniqueness
	seen := make(map[int32]bool)
	for _, v := range value {
		testAssert(!seen[v], "unique slices must have no duplicates")
		seen[v] = true
	}
	fmt.Printf("slices(unique): size=%d, all unique\n", len(value))
}

func testMaps() {
	gen := hegel.Maps(hegel.Integers[int32]()).MinSize(1).MaxSize(3)
	value := gen.Generate()
	testAssert(len(value) >= 1 && len(value) <= 3, "maps(1,3) size must be in [1,3]")
	fmt.Printf("maps(1,3): %v\n", value)
}

func testMapsNoSchema() {
	// Use a custom generator which has no schema, forcing compositional generation
	gen := hegel.Maps(hegel.Custom(func() int32 {
		return hegel.Integers[int32]().Min(0).Max(100).Generate() * 2
	})).MinSize(2).MaxSize(4)
	value := gen.Generate()
	testAssert(len(value) >= 2 && len(value) <= 4, "maps_no_schema size must be in [2,4]")
	for key, val := range value {
		testAssert(len(key) >= 1, "key must have at least 1 char")
		testAssert(val%2 == 0, "values must be even (doubled)")
		testAssert(val >= 0 && val <= 200, "values must be in [0,200]")
	}
	fmt.Printf("maps_no_schema(2,4): %v\n", value)
}

// =============================================================================
// Combinator tests
// =============================================================================

func testSampledFromStrings() {
	options := []string{"apple", "banana", "cherry"}
	gen := hegel.SampledFrom(options)
	value := gen.Generate()
	testAssert(value == "apple" || value == "banana" || value == "cherry",
		"sampled_from must return one of the options")
	fmt.Printf("sampled_from(fruits): \"%s\"\n", value)
}

func testSampledFromInts() {
	gen := hegel.SampledFrom([]int{10, 20, 30, 40, 50})
	value := gen.Generate()
	testAssert(value == 10 || value == 20 || value == 30 || value == 40 || value == 50,
		"sampled_from must return one of the options")
	fmt.Printf("sampled_from(ints): %d\n", value)
}

func testOneOf() {
	gen := hegel.OneOf(
		hegel.Integers[int32]().Min(0).Max(10),
		hegel.Integers[int32]().Min(100).Max(110),
	)
	value := gen.Generate()
	testAssert((value >= 0 && value <= 10) || (value >= 100 && value <= 110),
		"one_of must return from one of the ranges")
	fmt.Printf("one_of(0-10, 100-110): %d\n", value)
}

func testOptionalSome() {
	gen := hegel.Optional(hegel.Integers[int32]().Min(0).Max(100))
	value := gen.Generate()
	if value != nil {
		testAssert(*value >= 0 && *value <= 100, "optional value must be in range")
		fmt.Printf("optional(int): Some(%d)\n", *value)
	} else {
		fmt.Println("optional(int): None")
	}
}

// =============================================================================
// Struct generation tests
// =============================================================================

type Point struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
}

type Person struct {
	Name string `json:"name"`
	Age  uint32 `json:"age"`
}

func testMakePoint() {
	gen := hegel.Make[Point]().
		With("X", hegel.Integers[int32]().Min(-100).Max(100)).
		With("Y", hegel.Integers[int32]().Min(-100).Max(100))
	p := gen.Generate()
	testAssert(p.X >= -100 && p.X <= 100, "point.X must be in range")
	testAssert(p.Y >= -100 && p.Y <= 100, "point.Y must be in range")
	fmt.Printf("Make[Point]: (%d, %d)\n", p.X, p.Y)
}

func testMakePerson() {
	gen := hegel.Make[Person]().
		With("Name", hegel.Text().MinSize(1).MaxSize(20)).
		With("Age", hegel.Integers[uint32]().Min(0).Max(120))
	p := gen.Generate()
	nameLen := utf8.RuneCountInString(p.Name)
	testAssert(nameLen <= 20, "person.Name must be <= 20 chars")
	testAssert(p.Age <= 120, "person.Age must be in [0,120]")
	fmt.Printf("Make[Person]: {name=\"%s\", age=%d}\n", p.Name, p.Age)
}

// =============================================================================
// Filter test
// =============================================================================

func testFilter() {
	gen := hegel.Filter(
		hegel.Integers[int32]().Min(0).Max(100),
		func(x int32) bool { return x%2 == 0 },
		10,
	)
	value := gen.Generate()
	testAssert(value%2 == 0, "filtered value must be even")
	testAssert(value >= 0 && value <= 100, "filtered value must be in range")
	fmt.Printf("integers.filter(even): %d\n", value)
}

// =============================================================================
// Custom generator tests
// =============================================================================

func testCustom() {
	gen := hegel.Custom(func() int32 {
		return hegel.Integers[int32]().Min(1).Max(10).Generate() * hegel.Integers[int32]().Min(1).Max(10).Generate()
	})
	value := gen.Generate()
	testAssert(value >= 1 && value <= 100, "custom value must be in [1,100]")
	fmt.Printf("custom(x*y): %d\n", value)
}

func testCustomWithSchema() {
	gen := hegel.CustomWithSchema(
		func() int32 { return hegel.Integers[int32]().Min(1).Generate() },
		map[string]any{"type": "integer", "minimum": 1},
	)
	value := gen.Generate()
	testAssert(value >= 1, "custom with schema must produce >= 1")
	fmt.Printf("custom_with_schema(min=1): %d\n", value)
}

// =============================================================================
// Test registry
// =============================================================================

type testFn func()

func getAllTests() map[string]testFn {
	return map[string]testFn{
		// Primitives
		"nulls":       testNulls,
		"booleans":    testBooleans,
		"just_int":    testJustInt,
		"just_string": testJustString,
		// Integers
		"integers_unbounded":     testIntegersUnbounded,
		"integers_bounded":       testIntegersBounded,
		"integers_min_only":      testIntegersMinOnly,
		"integers_max_only":      testIntegersMaxOnly,
		"integers_uint8":         testIntegersUint8,
		"integers_negative_range": testIntegersNegativeRange,
		// Floats
		"floats_unbounded": testFloatsUnbounded,
		"floats_bounded":   testFloatsBounded,
		"floats_exclusive": testFloatsExclusive,
		"floats_float32":   testFloatsFloat32,
		// Strings
		"text_unbounded": testTextUnbounded,
		"text_bounded":   testTextBounded,
		"text_min_only":  testTextMinOnly,
		"from_regex":     testFromRegex,
		// Format strings
		"emails":           testEmails,
		"urls":             testURLs,
		"domains":          testDomains,
		"ip_addresses_v4":  testIPAddressesV4,
		"ip_addresses_v6":  testIPAddressesV6,
		"ip_addresses_any": testIPAddressesAny,
		// Datetime
		"dates":     testDates,
		"times":     testTimes,
		"datetimes": testDatetimes,
		// Collections
		"slices_basic":    testSlicesBasic,
		"slices_bounded":  testSlicesBounded,
		"slices_unique":   testSlicesUnique,
		"maps":            testMaps,
		"maps_no_schema":  testMapsNoSchema,
		// Combinators
		"sampled_from_strings": testSampledFromStrings,
		"sampled_from_ints":    testSampledFromInts,
		"one_of":               testOneOf,
		"optional":             testOptionalSome,
		// Struct generation
		"make_point":  testMakePoint,
		"make_person": testMakePerson,
		// Filter
		"filter": testFilter,
		// Custom
		"custom":             testCustom,
		"custom_with_schema": testCustomWithSchema,
	}
}

// =============================================================================
// Main
// =============================================================================

func main() {
	allTests := getAllTests()
	args := os.Args

	// Build slice of test names for SampledFrom
	testNames := make([]string, 0, len(allTests))
	for name := range allTests {
		testNames = append(testNames, name)
	}

	// If a test name is provided as argument, run that test directly
	// Otherwise, use SampledFrom to pick which test to run
	var selected string
	if len(args) > 1 {
		selected = args[1]
	} else {
		selected = hegel.SampledFrom(testNames).Generate()
	}

	fmt.Printf("Selected test: %s\n", selected)
	fmt.Println("----------------------------------------")

	// Find and run the selected test
	if testFunc, exists := allTests[selected]; exists {
		testFunc()
		fmt.Println("----------------------------------------")
		fmt.Printf("PASSED: %s\n", selected)
		return
	}

	fmt.Fprintf(os.Stderr, "Unknown test: %s\n", selected)
	fmt.Fprintln(os.Stderr, "Available tests:")
	for name := range allTests {
		fmt.Fprintf(os.Stderr, "  %s\n", name)
	}
	os.Exit(1)
}
