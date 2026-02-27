// collections demonstrates property testing with collection generators and
// combinators in Hegel: lists, dicts, tuples, OneOf, Optional, and Map.
//
// Run it with: go run ./examples/collections
package main

import (
	"fmt"
	"sort"

	hegel "github.com/antithesishq/hegel-go"
)

func main() {
	// Property 1: the length of a generated list is within [minSize, maxSize].
	hegel.RunHegelTest("list_size_bounds", func() {
		lst := hegel.Lists(
			hegel.IntegersUnbounded(),
			hegel.ListsOptions{MinSize: 2, MaxSize: 10},
		).Generate().([]any)

		if len(lst) < 2 || len(lst) > 10 {
			panic(fmt.Sprintf("list length %d out of [2,10]", len(lst)))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ list lengths are within bounds")

	// Property 2: sorting a list of integers is idempotent (sort(sort(x)) == sort(x)).
	hegel.RunHegelTest("sort_idempotent", func() {
		lst := hegel.Lists(
			hegel.Integers(-1000, 1000),
			hegel.ListsOptions{MinSize: 0, MaxSize: 20},
		).Generate().([]any)

		toInts := func(s []any) []int64 {
			out := make([]int64, len(s))
			for i, v := range s {
				n, _ := hegel.ExtractInt(v)
				out[i] = n
			}
			return out
		}
		nums := toInts(lst)
		sorted1 := make([]int64, len(nums))
		copy(sorted1, nums)
		sort.Slice(sorted1, func(i, j int) bool { return sorted1[i] < sorted1[j] })

		sorted2 := make([]int64, len(sorted1))
		copy(sorted2, sorted1)
		sort.Slice(sorted2, func(i, j int) bool { return sorted2[i] < sorted2[j] })

		for i := range sorted1 {
			if sorted1[i] != sorted2[i] {
				panic("sort is not idempotent")
			}
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ sorting is idempotent")

	// Property 3: a dict's size is within the requested bounds.
	hegel.RunHegelTest("dict_size_bounds", func() {
		d := hegel.Dicts(
			hegel.Integers(-100, 100),
			hegel.Integers(-100, 100),
			hegel.DictOptions{MinSize: 1, MaxSize: 5, HasMaxSize: true},
		).Generate().(map[any]any)

		if len(d) < 1 || len(d) > 5 {
			panic(fmt.Sprintf("dict size %d out of [1,5]", len(d)))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ dict sizes are within bounds")

	// Property 4: Tuples2 produces a slice of exactly two elements.
	hegel.RunHegelTest("tuple2_length", func() {
		pair := hegel.Tuples2(
			hegel.Integers(0, 100),
			hegel.Text(0, 10),
		).Generate().([]any)

		if len(pair) != 2 {
			panic(fmt.Sprintf("tuple2 has %d elements, want 2", len(pair)))
		}
	}, hegel.WithTestCases(100))
	fmt.Println("✅ Tuples2 always produces exactly 2 elements")

	// Property 5: OneOf produces values from one of the given generators.
	hegel.RunHegelTest("one_of_membership", func() {
		v := hegel.OneOf(
			hegel.Integers(1, 10),
			hegel.Integers(100, 200),
		).Generate()
		n, err := hegel.ExtractInt(v)
		if err != nil {
			panic(fmt.Sprintf("not an int: %v", v))
		}
		if !((n >= 1 && n <= 10) || (n >= 100 && n <= 200)) {
			panic(fmt.Sprintf("value %d not in either range", n))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ OneOf values are from one of the given generators")

	// Property 6: Optional is either nil or from the inner generator.
	hegel.RunHegelTest("optional_nil_or_value", func() {
		v := hegel.Optional(hegel.Integers(1, 100)).Generate()
		if v == nil {
			return // nil is always acceptable
		}
		n, err := hegel.ExtractInt(v)
		if err != nil {
			panic(fmt.Sprintf("expected int, got %T: %v", v, v))
		}
		if n < 1 || n > 100 {
			panic(fmt.Sprintf("value %d out of range [1, 100]", n))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ Optional produces nil or a value in range")

	// Property 7: Map transforms values correctly.
	hegel.RunHegelTest("map_doubles", func() {
		doubled := hegel.Integers(0, 500).Map(func(v any) any {
			n, _ := hegel.ExtractInt(v)
			return n * 2
		}).Generate()
		n, _ := hegel.ExtractInt(doubled)
		if n%2 != 0 {
			panic(fmt.Sprintf("doubled value %d is not even", n))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ Map(double) always produces even numbers")

	// Property 8: dependent generation — list length matches a generated count.
	hegel.RunHegelTest("list_length_matches_count", func() {
		count, _ := hegel.ExtractInt(hegel.Integers(1, 8).Generate())
		lst := hegel.Lists(
			hegel.IntegersUnbounded(),
			hegel.ListsOptions{MinSize: int(count), MaxSize: int(count)},
		).Generate().([]any)

		if int64(len(lst)) != count {
			panic(fmt.Sprintf("list length %d != count %d", len(lst), count))
		}
	}, hegel.WithTestCases(100))
	fmt.Println("✅ dependent generation: list length matches requested count")
}
