package hegel

// primitives_test.go contains e2e integration tests for the primitive
// generator functions: Integers, Floats, Booleans, Text, Binary.
// Schema unit tests live in generators_test.go.

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

	if _err := Run(func(s *TestCase) {
		// Full-range integers: just verify the draw completes without error.
		_ = Draw[int](s, Integers[int](math.MinInt, math.MaxInt))
	}, WithTestCases(20)); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_WithBounds(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		fv := Draw[float64](s, Floats[float64]().Min(0.0).Max(1.0).AllowNaN(false).AllowInfinity(false))
		if math.IsNaN(fv) {
			panic("floats: NaN not allowed when allow_nan=false")
		}
		if math.IsInf(fv, 0) {
			panic("floats: Inf not allowed when allow_infinity=false")
		}
		if fv < 0.0 || fv > 1.0 {
			panic("floats: out of range [0.0, 1.0]")
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_Unbounded(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		// Unbounded floats may produce NaN or Inf -- any float64 is valid.
		_ = Draw(s, Floats[float64]())
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_OnlyMin(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		fv := Draw(s, Floats[float64]().Min(0.0))
		// allow_nan is false (has min), allow_infinity is true (no max)
		// Value should be >= 0.0 or Inf; NaN not allowed.
		if math.IsNaN(fv) {
			panic("floats: NaN not expected when min set")
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_Float32(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		fv := Draw(s, Floats[float32]().Min(0.0).Max(1.0).AllowNaN(false).AllowInfinity(false))
		if fv < 0.0 || fv > 1.0 {
			panic("float32: out of range [0.0, 1.0]")
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestFloatsGenerateErrorResponse(t *testing.T) {

	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "error_response")
	err := Run(func(s *TestCase) {
		_ = Floats[float64]().draw(s)
	})
	_ = err
}

func TestBooleansE2E(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		b := Draw[bool](s, Booleans())
		// A valid assertion: b is either true or false.
		if b != true && b != false {
			panic("booleans: expected bool")
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestTextE2E(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		sv := Draw[string](s, Text().MinSize(2).MaxSize(8))
		count := utf8.RuneCountInString(sv)
		if count < 2 || count > 8 {
			panic("text: codepoint count out of range [2, 8]")
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestTextE2E_Unbounded(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		sv := Draw[string](s, Text())
		if !utf8.ValidString(sv) {
			panic("text: invalid UTF-8 string")
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestBinaryE2E(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		bv := Draw[[]byte](s, Binary(1, 10))
		if len(bv) < 1 || len(bv) > 10 {
			panic("binary: byte length out of range [1, 10]")
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestBinaryE2E_Unbounded(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		_ = Draw[[]byte](s, Binary(0, -1))
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestFloatGeneratorBuildsBasicGenerator(t *testing.T) {
	t.Parallel()
	gen := Floats[float64]().Min(0).Max(1).AllowNaN(false).AllowInfinity(false)

	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Floats should be basic")
	}
	if bg.schema["type"] != "float" {
		t.Fatalf("schema type: expected 'float', got %v", bg.schema["type"])
	}
}

func TestListsFloatBuilderUsesBasicPath(t *testing.T) {
	t.Parallel()
	gen := Lists(Floats[float64]().Min(0).Max(1).AllowNaN(false).AllowInfinity(false)).MaxSize(3)

	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Lists(Floats(...)) should be basic")
	}

	elemSchema, ok := bg.schema["elements"].(map[string]any)
	if !ok {
		t.Fatalf("schema elements: expected map[string]any, got %T", bg.schema["elements"])
	}
	if elemSchema["type"] != "float" {
		t.Fatalf("elements type: expected 'float', got %v", elemSchema["type"])
	}
}

// =============================================================================
// TextGenerator builder methods
// =============================================================================

func TestTextCodecSchema(t *testing.T) {
	t.Parallel()
	bg, _, err := Text().MaxSize(10).Codec("ascii").asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if bg.schema["codec"] != "ascii" {
		t.Errorf("codec: expected 'ascii', got %v", bg.schema["codec"])
	}
}

func TestTextMinCodepointSchema(t *testing.T) {
	t.Parallel()
	bg, _, err := Text().MaxSize(10).MinCodepoint(32).asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if bg.schema["min_codepoint"] != int64(32) {
		t.Errorf("min_codepoint: expected 32, got %v", bg.schema["min_codepoint"])
	}
}

func TestTextMaxCodepointSchema(t *testing.T) {
	t.Parallel()
	bg, _, err := Text().MaxSize(10).MaxCodepoint(127).asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if bg.schema["max_codepoint"] != int64(127) {
		t.Errorf("max_codepoint: expected 127, got %v", bg.schema["max_codepoint"])
	}
}

func TestTextCategoriesSchema(t *testing.T) {
	t.Parallel()
	bg, _, err := Text().MaxSize(10).Categories([]string{"L"}).asBasic()
	if err != nil {
		t.Fatal(err)
	}
	cats, ok := bg.schema["categories"].([]any)
	if !ok {
		t.Fatal("categories should be present")
	}
	if len(cats) != 1 || cats[0] != "L" {
		t.Errorf("categories: expected [L], got %v", cats)
	}
}

func TestTextExcludeCategoriesSchema(t *testing.T) {
	t.Parallel()
	bg, _, err := Text().MaxSize(10).ExcludeCategories([]string{"Zs"}).asBasic()
	if err != nil {
		t.Fatal(err)
	}
	cats, ok := bg.schema["exclude_categories"].([]any)
	if !ok {
		t.Fatal("exclude_categories should be present")
	}
	found := map[string]bool{}
	for _, c := range cats {
		found[c.(string)] = true
	}
	if !found["Zs"] {
		t.Error("exclude_categories should contain Zs")
	}
	if !found["Cs"] {
		t.Error("exclude_categories should auto-add Cs")
	}
}

func TestTextIncludeCharactersSchema(t *testing.T) {
	t.Parallel()
	bg, _, err := Text().MaxSize(10).IncludeCharacters("!@#").asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if bg.schema["include_characters"] != "!@#" {
		t.Errorf("include_characters: expected '!@#', got %v", bg.schema["include_characters"])
	}
}

func TestTextExcludeCharactersSchema(t *testing.T) {
	t.Parallel()
	bg, _, err := Text().MaxSize(10).ExcludeCharacters("xyz").asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if bg.schema["exclude_characters"] != "xyz" {
		t.Errorf("exclude_characters: expected 'xyz', got %v", bg.schema["exclude_characters"])
	}
}

func TestTextExcludeCategoriesNoDuplicateCs(t *testing.T) {
	t.Parallel()
	bg, _, err := Text().MaxSize(10).ExcludeCategories([]string{"Cs", "Zs"}).asBasic()
	if err != nil {
		t.Fatal(err)
	}
	cats := bg.schema["exclude_categories"].([]any)
	csCount := 0
	for _, c := range cats {
		if c.(string) == "Cs" {
			csCount++
		}
	}
	if csCount != 1 {
		t.Errorf("Cs should appear exactly once, got %d", csCount)
	}
}

func TestTextAlphabetSchema(t *testing.T) {
	t.Parallel()
	bg, _, err := Text().MaxSize(10).Alphabet("abc").asBasic()
	if err != nil {
		t.Fatal(err)
	}
	cats, ok := bg.schema["categories"].([]any)
	if !ok {
		t.Fatal("categories should be present (Alphabet sets empty categories list)")
	}
	if len(cats) != 0 {
		t.Errorf("categories: expected empty list, got %v", cats)
	}
	if bg.schema["include_characters"] != "abc" {
		t.Errorf("include_characters: expected 'abc', got %v", bg.schema["include_characters"])
	}
}

func TestTextAlphabetConflictsWithCharParams(t *testing.T) {
	t.Parallel()
	_, _, err := Text().MaxSize(10).Codec("ascii").Alphabet("abc").asBasic()
	assertErrorContains(t, "cannot combine", err)
}

func TestTextNegativeMaxSize(t *testing.T) {
	t.Parallel()
	_, _, err := Text().MaxSize(-1).asBasic()
	assertErrorContains(t, "max_size=-1 must be non-negative", err)
}

// =============================================================================
// CharactersGenerator tests
// =============================================================================

func TestCharactersSchema(t *testing.T) {
	t.Parallel()
	bg, _, err := Characters().asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if bg.schema["type"] != "string" {
		t.Errorf("type: expected 'string', got %v", bg.schema["type"])
	}
	if bg.schema["min_size"] != int64(1) {
		t.Errorf("min_size: expected 1, got %v", bg.schema["min_size"])
	}
	if bg.schema["max_size"] != int64(1) {
		t.Errorf("max_size: expected 1, got %v", bg.schema["max_size"])
	}
}

func TestCharactersAsBasic(t *testing.T) {
	t.Parallel()
	_, ok, err := Characters().asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Characters() should be basic")
	}
}

func TestTextAsBasic(t *testing.T) {
	t.Parallel()
	_, ok, err := Text().MaxSize(10).asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Text() should be basic")
	}
}

func TestCharactersE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Characters().Codec("ascii"))
		if utf8.RuneCountInString(v) != 1 {
			panic("Characters: expected exactly one character")
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// TextGenerator E2E with character filtering
// =============================================================================

func TestTextCodecE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Text().MinSize(1).MaxSize(10).Codec("ascii"))
		for _, r := range v {
			if r > 127 {
				panic("Text with Codec(ascii): non-ASCII character found")
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestTextAlphabetE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Text().MinSize(1).MaxSize(5).Alphabet("abc"))
		for _, r := range v {
			if r != 'a' && r != 'b' && r != 'c' {
				panic("Text with Alphabet(abc): unexpected character")
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestTextSingleCharAlphabetE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Text().MinSize(1).MaxSize(5).Alphabet("x"))
		for _, r := range v {
			if r != 'x' {
				panic(fmt.Sprintf("Text with Alphabet(x): expected 'x', got %q", r))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestTextCodepointRangeE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Text().MinSize(1).MaxSize(20).MinCodepoint(0x41).MaxCodepoint(0x5A))
		for _, r := range v {
			if r < 0x41 || r > 0x5A {
				panic(fmt.Sprintf("Text codepoint range: %U outside [U+0041, U+005A]", r))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestTextCategoriesE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Text().MinSize(1).MaxSize(20).Categories([]string{"Lu"}))
		for _, r := range v {
			if !unicode.In(r, unicode.Lu) {
				panic(fmt.Sprintf("Text with Categories([Lu]): %q is not in category Lu", r))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestTextExcludeCategoriesE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Text().MinSize(1).MaxSize(20).ExcludeCategories([]string{"Lu"}))
		for _, r := range v {
			if unicode.In(r, unicode.Lu) {
				panic(fmt.Sprintf("Text with ExcludeCategories([Lu]): %q is in category Lu", r))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestTextIncludeCharactersE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Text().MinSize(1).MaxSize(20).Categories([]string{}).IncludeCharacters("xyz"))
		for _, r := range v {
			if !strings.ContainsRune("xyz", r) {
				panic(fmt.Sprintf("Text with IncludeCharacters(xyz): %q not in allowed set", r))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestTextExcludeCharactersE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		excluded := "aeiou"
		v := Draw[string](s, Text().MinSize(1).MaxSize(20).Codec("ascii").ExcludeCharacters(excluded))
		for _, r := range v {
			if strings.ContainsRune(excluded, r) {
				panic(fmt.Sprintf("Text with ExcludeCharacters: %q should be excluded", r))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// CharactersGenerator E2E with character filtering
// =============================================================================

func TestCharactersCodepointRangeE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Characters().MinCodepoint(0x41).MaxCodepoint(0x5A))
		r, _ := utf8.DecodeRuneInString(v)
		if r < 0x41 || r > 0x5A {
			panic(fmt.Sprintf("Characters codepoint range: %U outside [U+0041, U+005A]", r))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestCharactersCategoriesLuE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Characters().Categories([]string{"Lu"}))
		r, _ := utf8.DecodeRuneInString(v)
		if !unicode.In(r, unicode.Lu) {
			panic(fmt.Sprintf("Characters with Categories([Lu]): %q is not in category Lu", r))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestCharactersExcludeCategoriesE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Characters().ExcludeCategories([]string{"Lu"}))
		r, _ := utf8.DecodeRuneInString(v)
		if unicode.In(r, unicode.Lu) {
			panic(fmt.Sprintf("Characters with ExcludeCategories([Lu]): %q is in category Lu", r))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestCharactersIncludeCharactersE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		v := Draw[string](s, Characters().Categories([]string{}).IncludeCharacters("xyz"))
		r, _ := utf8.DecodeRuneInString(v)
		if !strings.ContainsRune("xyz", r) {
			panic(fmt.Sprintf("Characters with IncludeCharacters(xyz): %q not in allowed set", r))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

func TestCharactersExcludeCharactersE2E(t *testing.T) {
	t.Parallel()

	if _err := runHegel(func(s *TestCase) {
		excluded := "aeiou"
		v := Draw[string](s, Characters().Codec("ascii").ExcludeCharacters(excluded))
		r, _ := utf8.DecodeRuneInString(v)
		if strings.ContainsRune(excluded, r) {
			panic(fmt.Sprintf("Characters with ExcludeCharacters: %q should be excluded", r))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}
