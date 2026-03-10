// collections demonstrates property testing with collection generators and
// combinators in Hegel: lists, dicts, OneOf, Optional, and Map.
//
// Run it with: go run ./examples/collections
package main

import (
	"fmt"
	"math"
	"sort"

	hegel "github.com/antithesishq/hegel-go"
)

func main() {
	// Property 1: the length of a generated list is within [minSize, maxSize].
	hegel.MustRun("list_size_bounds", func(s *hegel.TestCase) {
		lst := hegel.Draw(s, hegel.Lists(
			hegel.Integers[int](math.MinInt, math.MaxInt),
			hegel.ListMinSize(2), hegel.ListMaxSize(10),
		))

		if len(lst) < 2 || len(lst) > 10 {
			panic(fmt.Sprintf("list length %d out of [2,10]", len(lst)))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("list lengths are within bounds")

	// Property 2: sorting a list of integers is idempotent (sort(sort(x)) == sort(x)).
	hegel.MustRun("sort_idempotent", func(s *hegel.TestCase) {
		nums := hegel.Draw(s, hegel.Lists(
			hegel.Integers[int](-1000, 1000),
			hegel.ListMaxSize(20),
		))

		sorted1 := make([]int, len(nums))
		copy(sorted1, nums)
		sort.Slice(sorted1, func(i, j int) bool { return sorted1[i] < sorted1[j] })

		sorted2 := make([]int, len(sorted1))
		copy(sorted2, sorted1)
		sort.Slice(sorted2, func(i, j int) bool { return sorted2[i] < sorted2[j] })

		for i := range sorted1 {
			if sorted1[i] != sorted2[i] {
				panic("sort is not idempotent")
			}
		}
	}, hegel.WithTestCases(200))
	fmt.Println("sorting is idempotent")

	// Property 3: a dict's size is within the requested bounds.
	hegel.MustRun("dict_size_bounds", func(s *hegel.TestCase) {
		d := hegel.Draw(s, hegel.Dicts(
			hegel.Integers[int](-100, 100),
			hegel.Integers[int](-100, 100),
			hegel.DictMinSize(1), hegel.DictMaxSize(5),
		))

		if len(d) < 1 || len(d) > 5 {
			panic(fmt.Sprintf("dict size %d out of [1,5]", len(d)))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("dict sizes are within bounds")

	// Property 4: drawing two values independently produces a pair.
	hegel.MustRun("independent_draws", func(s *hegel.TestCase) {
		n := hegel.Draw(s, hegel.Integers[int](0, 100))
		str := hegel.Draw(s, hegel.Text(0, 10))

		// Verify we got the expected types and ranges.
		if n < 0 || n > 100 {
			panic(fmt.Sprintf("integer %d out of [0, 100]", n))
		}
		if len([]rune(str)) > 10 {
			panic(fmt.Sprintf("string %q exceeds max length 10", str))
		}
	}, hegel.WithTestCases(100))
	fmt.Println("independent draws produce values in range")

	// Property 5: OneOf produces values from one of the given generators.
	hegel.MustRun("one_of_membership", func(s *hegel.TestCase) {
		n := hegel.Draw(s, hegel.OneOf(
			hegel.Integers[int](1, 10),
			hegel.Integers[int](100, 200),
		))
		if !((n >= 1 && n <= 10) || (n >= 100 && n <= 200)) {
			panic(fmt.Sprintf("value %d not in either range", n))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("OneOf values are from one of the given generators")

	// Property 6: Optional is either nil or from the inner generator.
	hegel.MustRun("optional_nil_or_value", func(s *hegel.TestCase) {
		v := hegel.Draw(s, hegel.Optional(hegel.Integers[int](1, 100)))
		if v == nil {
			return // nil is always acceptable
		}
		if *v < 1 || *v > 100 {
			panic(fmt.Sprintf("value %d out of range [1, 100]", *v))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("Optional produces nil or a value in range")

	// Property 7: Map transforms values correctly.
	hegel.MustRun("map_doubles", func(s *hegel.TestCase) {
		n := hegel.Draw(s, hegel.Map(hegel.Integers[int](0, 500), func(v int) int {
			return v * 2
		}))
		if n%2 != 0 {
			panic(fmt.Sprintf("doubled value %d is not even", n))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("Map(double) always produces even numbers")

	// Property 8: dependent generation -- list length matches a generated count.
	hegel.MustRun("list_length_matches_count", func(s *hegel.TestCase) {
		count := hegel.Draw(s, hegel.Integers[int](1, 8))
		lst := hegel.Draw(s, hegel.Lists(
			hegel.Integers[int](math.MinInt, math.MaxInt),
			hegel.ListMinSize(count), hegel.ListMaxSize(count),
		))

		if len(lst) != count {
			panic(fmt.Sprintf("list length %d != count %d", len(lst), count))
		}
	}, hegel.WithTestCases(100))
	fmt.Println("dependent generation: list length matches requested count")
}
