package hegel

// primitives_test.go contains e2e integration tests for the primitive
// generator functions: Integers, Floats, Booleans, Text, Binary — plus unit
// tests for the character-set argument mapping and config validation.

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// =============================================================================
// Integration / e2e tests (run against real hegel binary, 50 test cases each)
// =============================================================================

func TestIntegersFullRangeE2E(t *testing.T) {

	Test(t, func(ht *T) {
		// Full-range integers: just verify the draw completes without error.
		_ = Draw[int](ht, Integers[int](math.MinInt, math.MaxInt))
	}, WithTestCases(20))
}

func TestFloatsE2E_WithBounds(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		fv := Draw[float64](ht, Floats[float64]().Min(0.0).Max(1.0).AllowNaN(false).AllowInfinity(false))
		if math.IsNaN(fv) {
			panic("floats: NaN not allowed when allow_nan=false")
		}
		if math.IsInf(fv, 0) {
			panic("floats: Inf not allowed when allow_infinity=false")
		}
		if fv < 0.0 || fv > 1.0 {
			panic("floats: out of range [0.0, 1.0]")
		}
	}, WithTestCases(50))
}

func TestFloatsE2E_Unbounded(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		// Unbounded floats may produce NaN or Inf -- any float64 is valid.
		_ = Draw(ht, Floats[float64]())
	}, WithTestCases(50))
}

func TestFloatsE2E_OnlyMin(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		fv := Draw(ht, Floats[float64]().Min(0.0))
		// allow_nan is false (has min), allow_infinity is true (no max)
		// Value should be >= 0.0 or Inf; NaN not allowed.
		if math.IsNaN(fv) {
			panic("floats: NaN not expected when min set")
		}
	}, WithTestCases(50))
}

func TestFloatsE2E_Float32(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		fv := Draw(ht, Floats[float32]().Min(0.0).Max(1.0).AllowNaN(false).AllowInfinity(false))
		if fv < 0.0 || fv > 1.0 {
			panic("float32: out of range [0.0, 1.0]")
		}
	}, WithTestCases(50))
}

func TestBooleansE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		b := Draw[bool](ht, Booleans())
		// A valid assertion: b is either true or false.
		if b != true && b != false {
			panic("booleans: expected bool")
		}
	}, WithTestCases(50))
}

func TestTextE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		sv := Draw[string](ht, Text().MinSize(2).MaxSize(8))
		count := utf8.RuneCountInString(sv)
		if count < 2 || count > 8 {
			panic("text: codepoint count out of range [2, 8]")
		}
	}, WithTestCases(50))
}

func TestTextE2E_Unbounded(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		sv := Draw[string](ht, Text())
		if !utf8.ValidString(sv) {
			panic("text: invalid UTF-8 string")
		}
	}, WithTestCases(50))
}

func TestBinaryE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		bv := Draw[[]byte](ht, Binary(1, 10))
		if len(bv) < 1 || len(bv) > 10 {
			panic("binary: byte length out of range [1, 10]")
		}
	}, WithTestCases(50))
}

func TestBinaryE2E_Unbounded(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		_ = Draw[[]byte](ht, Binary(0, -1))
	}, WithTestCases(50))
}

// =============================================================================
// characterFields.textArgs unit tests
// =============================================================================

// TestTextArgsDefaults verifies the default character-set arguments: the utf-8
// codec, no codepoint constraint, and surrogate auto-exclusion.
func TestCharFieldsArgsDefaults(t *testing.T) {
	t.Parallel()
	g := Text().MaxSize(10)
	codec, minCP, maxCP, cats, excl, err := g.charFields.textArgs()
	if err != nil {
		t.Fatal(err)
	}
	if codec != "utf-8" {
		t.Errorf("codec: expected utf-8, got %q", codec)
	}
	if minCP != 0 {
		t.Errorf("minCP: expected 0, got %d", minCP)
	}
	if maxCP != math.MaxUint32 {
		t.Errorf("maxCP: expected math.MaxUint32, got %d", maxCP)
	}
	if cats != nil {
		t.Errorf("categories: expected nil, got %v", cats)
	}
	if len(excl) != 1 || excl[0] != "Cs" {
		t.Errorf("excludeCategories: expected [Cs], got %v", excl)
	}
}

// TestTextArgsCodecAndCodepoints verifies that Codec / MinCodepoint /
// MaxCodepoint flow through to the returned arguments.
func TestCharFieldsArgsCodecCodepoints(t *testing.T) {
	t.Parallel()
	g := Text().MaxSize(10).Codec("ascii").MinCodepoint(32).MaxCodepoint(127)
	codec, minCP, maxCP, _, _, err := g.charFields.textArgs()
	if err != nil {
		t.Fatal(err)
	}
	if codec != "ascii" {
		t.Errorf("codec: expected ascii, got %q", codec)
	}
	if minCP != 32 {
		t.Errorf("minCP: expected 32, got %d", minCP)
	}
	if maxCP != 127 {
		t.Errorf("maxCP: expected 127, got %d", maxCP)
	}
}

// TestTextArgsCategories verifies that an explicit Categories set is passed
// through and no exclude set is produced.
func TestCharFieldsArgsCategories(t *testing.T) {
	t.Parallel()
	g := Text().MaxSize(10).Categories([]string{"L", "Nd"})
	_, _, _, cats, excl, err := g.charFields.textArgs()
	if err != nil {
		t.Fatal(err)
	}
	if len(cats) != 2 || cats[0] != "L" || cats[1] != "Nd" {
		t.Errorf("categories: expected [L Nd], got %v", cats)
	}
	if excl != nil {
		t.Errorf("excludeCategories: expected nil, got %v", excl)
	}
}

// TestTextArgsExcludeCategoriesAddsCs verifies that Cs is auto-added to the
// exclude set exactly once (never duplicated).
func TestCharFieldsArgsExcludeAddsCs(t *testing.T) {
	t.Parallel()
	g := Text().MaxSize(10).ExcludeCategories([]string{"Cs", "Zs"})
	_, _, _, _, excl, err := g.charFields.textArgs()
	if err != nil {
		t.Fatal(err)
	}
	csCount := 0
	hasZs := false
	for _, c := range excl {
		if c == "Cs" {
			csCount++
		}
		if c == "Zs" {
			hasZs = true
		}
	}
	if csCount != 1 {
		t.Errorf("Cs should appear exactly once, got %d in %v", csCount, excl)
	}
	if !hasZs {
		t.Errorf("excludeCategories should contain Zs, got %v", excl)
	}
}

// TestTextArgsSurrogateCategoryRejected verifies that requesting a category
// that includes surrogate codepoints is an error (Go strings are UTF-8).
func TestCharFieldsArgsSurrogateRejected(t *testing.T) {
	t.Parallel()
	g := Text().MaxSize(10).Categories([]string{"Cs"})
	_, _, _, _, _, err := g.charFields.textArgs()
	assertErrorContains(t, "surrogate", err)
}

// =============================================================================
// Config validation (returned before the engine is touched)
// =============================================================================

func TestTextAlphabetConflictsWithCharParams(t *testing.T) {
	t.Parallel()
	_, err := Text().MaxSize(10).Codec("ascii").Alphabet("abc").draw(newStubTestCase(t))
	assertErrorContains(t, "cannot combine", err)
}

func TestTextNegativeMaxSize(t *testing.T) {
	t.Parallel()
	_, err := Text().MaxSize(-1).draw(newStubTestCase(t))
	assertErrorContains(t, "Text: MaxSize -1 must be non-negative", err)
}

// =============================================================================
// CharactersGenerator E2E
// =============================================================================

func TestCharactersE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Characters().Codec("ascii"))
		if utf8.RuneCountInString(v) != 1 {
			panic("Characters: expected exactly one character")
		}
	}, WithTestCases(30))
}

// =============================================================================
// TextGenerator E2E with character filtering
// =============================================================================

func TestTextCodecE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Text().MinSize(1).MaxSize(10).Codec("ascii"))
		for _, r := range v {
			if r > 127 {
				panic("Text with Codec(ascii): non-ASCII character found")
			}
		}
	}, WithTestCases(30))
}

func TestTextAlphabetE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Text().MinSize(1).MaxSize(5).Alphabet("abc"))
		for _, r := range v {
			if r != 'a' && r != 'b' && r != 'c' {
				panic("Text with Alphabet(abc): unexpected character")
			}
		}
	}, WithTestCases(30))
}

func TestTextSingleCharAlphabetE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Text().MinSize(1).MaxSize(5).Alphabet("x"))
		for _, r := range v {
			if r != 'x' {
				panic(fmt.Sprintf("Text with Alphabet(x): expected 'x', got %q", r))
			}
		}
	}, WithTestCases(30))
}

func TestTextCodepointRangeE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Text().MinSize(1).MaxSize(20).MinCodepoint(0x41).MaxCodepoint(0x5A))
		for _, r := range v {
			if r < 0x41 || r > 0x5A {
				panic(fmt.Sprintf("Text codepoint range: %U outside [U+0041, U+005A]", r))
			}
		}
	}, WithTestCases(30))
}

func TestTextCategoriesE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Text().MinSize(1).MaxSize(20).Categories([]string{"Cc"}))
		for _, r := range v {
			if !unicode.In(r, unicode.Cc) {
				panic(fmt.Sprintf("Text with Categories([Cc]): %q is not in category Cc", r))
			}
		}
	}, WithTestCases(30))
}

func TestTextExcludeCategoriesE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Text().MinSize(1).MaxSize(20).ExcludeCategories([]string{"Cc"}))
		for _, r := range v {
			if unicode.In(r, unicode.Cc) {
				panic(fmt.Sprintf("Text with ExcludeCategories([Cc]): %q is in category Cc", r))
			}
		}
	}, WithTestCases(30))
}

func TestTextIncludeCharactersE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Text().MinSize(1).MaxSize(20).Categories([]string{}).IncludeCharacters("xyz"))
		for _, r := range v {
			if !strings.ContainsRune("xyz", r) {
				panic(fmt.Sprintf("Text with IncludeCharacters(xyz): %q not in allowed set", r))
			}
		}
	}, WithTestCases(30))
}

func TestTextExcludeCharactersE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		excluded := "aeiou"
		v := Draw[string](ht, Text().MinSize(1).MaxSize(20).Codec("ascii").ExcludeCharacters(excluded))
		for _, r := range v {
			if strings.ContainsRune(excluded, r) {
				panic(fmt.Sprintf("Text with ExcludeCharacters: %q should be excluded", r))
			}
		}
	}, WithTestCases(30))
}

// =============================================================================
// CharactersGenerator E2E with character filtering
// =============================================================================

func TestCharactersCodepointRangeE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Characters().MinCodepoint(0x41).MaxCodepoint(0x5A))
		r, _ := utf8.DecodeRuneInString(v)
		if r < 0x41 || r > 0x5A {
			panic(fmt.Sprintf("Characters codepoint range: %U outside [U+0041, U+005A]", r))
		}
	}, WithTestCases(30))
}

func TestCharactersCategoriesCcE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Characters().Categories([]string{"Cc"}))
		r, _ := utf8.DecodeRuneInString(v)
		if !unicode.In(r, unicode.Cc) {
			panic(fmt.Sprintf("Characters with Categories([Cc]): %q is not in category Cc", r))
		}
	}, WithTestCases(30))
}

func TestCharactersExcludeCategoriesE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Characters().ExcludeCategories([]string{"Cc"}))
		r, _ := utf8.DecodeRuneInString(v)
		if unicode.In(r, unicode.Cc) {
			panic(fmt.Sprintf("Characters with ExcludeCategories([Cc]): %q is in category Cc", r))
		}
	}, WithTestCases(30))
}

func TestCharactersIncludeCharactersE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, Characters().Categories([]string{}).IncludeCharacters("xyz"))
		r, _ := utf8.DecodeRuneInString(v)
		if !strings.ContainsRune("xyz", r) {
			panic(fmt.Sprintf("Characters with IncludeCharacters(xyz): %q not in allowed set", r))
		}
	}, WithTestCases(30))
}

func TestCharactersExcludeCharactersE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		excluded := "aeiou"
		v := Draw[string](ht, Characters().Codec("ascii").ExcludeCharacters(excluded))
		r, _ := utf8.DecodeRuneInString(v)
		if strings.ContainsRune(excluded, r) {
			panic(fmt.Sprintf("Characters with ExcludeCharacters: %q should be excluded", r))
		}
	}, WithTestCases(30))
}
