// strings demonstrates property testing of string-processing functions with Hegel.
//
// It tests a small set of real-world string invariants: round-trip encoding,
// length bounds, palindrome detection, and regular-expression matching.
//
// Run it with: go run ./examples/strings
package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	hegel "github.com/antithesishq/hegel-go"
)

// reverseString returns s with its Unicode codepoints in reverse order.
func reverseString(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// isPalindrome reports whether s reads the same forwards and backwards.
func isPalindrome(s string) bool {
	return s == reverseString(s)
}

func main() {
	// Property 1: reversing twice gives the original string.
	hegel.RunHegelTest("reverse_twice", func() {
		s, _ := hegel.Text(0, 50).Generate().(string)
		if reverseString(reverseString(s)) != s {
			panic(fmt.Sprintf("reverse(reverse(%q)) != %q", s, s))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ reverse(reverse(s)) == s")

	// Property 2: len([]rune(s)) matches utf8.RuneCountInString.
	hegel.RunHegelTest("rune_count_consistent", func() {
		s, _ := hegel.Text(0, 100).Generate().(string)
		if len([]rune(s)) != utf8.RuneCountInString(s) {
			panic(fmt.Sprintf("rune count mismatch for %q", s))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ len([]rune(s)) == utf8.RuneCountInString(s)")

	// Property 3: joining and splitting round-trips correctly.
	hegel.RunHegelTest("join_split_roundtrip", func() {
		// Generate a list of non-empty words (no commas so the separator is unambiguous).
		words := hegel.Lists(
			hegel.FromRegex(`[a-z]+`, true),
			hegel.ListsOptions{MinSize: 1, MaxSize: 8},
		).Generate().([]any)

		strs := make([]string, len(words))
		for i, w := range words {
			s, _ := w.(string)
			strs[i] = s
		}

		joined := strings.Join(strs, ",")
		parts := strings.Split(joined, ",")

		if len(parts) != len(strs) {
			panic(fmt.Sprintf("split gave %d parts, want %d", len(parts), len(strs)))
		}
		for i := range strs {
			if parts[i] != strs[i] {
				panic(fmt.Sprintf("part[%d]: got %q, want %q", i, parts[i], strs[i]))
			}
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ strings.Join / strings.Split round-trip is lossless")

	// Property 4: ToUpper is idempotent (upper(upper(s)) == upper(s)).
	hegel.RunHegelTest("to_upper_idempotent", func() {
		s, _ := hegel.FromRegex(`[a-z ]+`, true).Generate().(string)
		u1 := strings.ToUpper(s)
		u2 := strings.ToUpper(u1)
		if u1 != u2 {
			panic(fmt.Sprintf("ToUpper not idempotent: ToUpper(%q)=%q, ToUpper(ToUpper)=%q", s, u1, u2))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ strings.ToUpper is idempotent")

	// Property 5: Note and Target — use Note to capture the palindrome under test
	// and Target to push Hegel toward longer strings (making failures more vivid).
	hegel.RunHegelTest("palindrome_detection", func() {
		s, _ := hegel.Text(0, 30).Generate().(string)
		hegel.Note(fmt.Sprintf("testing %q (is palindrome: %v)", s, isPalindrome(s)))
		hegel.Target(float64(utf8.RuneCountInString(s)), "string_length")

		// A string is a palindrome iff its reverse equals itself.
		if isPalindrome(s) != (reverseString(s) == s) {
			panic(fmt.Sprintf("isPalindrome inconsistent for %q", s))
		}
	}, hegel.WithTestCases(300))
	fmt.Println("✅ isPalindrome is consistent with manual reverse comparison")

	// Property 6: SampledFrom picks a value from the given set.
	hegel.RunHegelTest("sampled_from_membership", func() {
		options := []any{"alpha", "beta", "gamma", "delta"}
		v, _ := hegel.MustSampledFrom(options).Generate().(string)
		found := false
		for _, o := range options {
			if v == o.(string) {
				found = true
				break
			}
		}
		if !found {
			panic(fmt.Sprintf("%q not in options", v))
		}
	}, hegel.WithTestCases(100))
	fmt.Println("✅ SampledFrom always picks from the provided options")
}
