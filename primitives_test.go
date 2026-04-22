package hegel

// primitives_test.go contains e2e integration tests for the primitive
// generator functions: Integers, Floats, Booleans, Text, Binary.
// Schema unit tests live in generators_test.go.

import (
	"math"
	"testing"
	"unicode/utf8"
)

// =============================================================================
// Integration / e2e tests (run against real hegel binary, 50 test cases each)
// =============================================================================

func TestIntegersFullRangeE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		// Full-range integers: just verify the draw completes without error.
		_ = Draw[int](s, Integers[int](math.MinInt, math.MaxInt))
	}, WithTestCases(20)); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_WithBounds(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
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
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		// Unbounded floats may produce NaN or Inf -- any float64 is valid.
		_ = Draw(s, Floats[float64]())
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_OnlyMin(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
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
	hegelBinPath(t)
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
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "error_response")
	err := Run(func(s *TestCase) {
		_ = Floats[float64]().draw(s)
	})
	_ = err
}

func TestBooleansE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
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
	hegelBinPath(t)
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
	hegelBinPath(t)
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
	hegelBinPath(t)
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
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		_ = Draw[[]byte](s, Binary(0, -1))
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestFloatGeneratorBuildsBasicGenerator(t *testing.T) {
	t.Parallel()
	gen := Floats[float64]().Min(0).Max(1).AllowNaN(false).AllowInfinity(false)

	bg, ok := gen.buildGenerator().(*basicGenerator[float64])
	if !ok {
		t.Fatalf("Floats should build *basicGenerator[float64], got %T", gen.buildGenerator())
	}
	if bg.transform == nil {
		t.Fatal("Floats basic generator should set a transform")
	}
	if bg.schema["type"] != "float" {
		t.Fatalf("schema type: expected 'float', got %v", bg.schema["type"])
	}
}

func TestTextGeneratorSchema(t *testing.T) {
	t.Parallel()
	gen := Text().MinSize(1).MaxSize(10).MinCodepoint(65).MaxCodepoint(90)
	bg := gen.buildGenerator().(*basicGenerator[string])
	if bg.schema["type"] != "string" {
		t.Errorf("schema type: expected 'string', got %v", bg.schema["type"])
	}
	minV, _ := extractCBORInt(bg.schema["min_size"])
	if minV != 1 {
		t.Errorf("min_size: expected 1, got %d", minV)
	}
	maxV, _ := extractCBORInt(bg.schema["max_size"])
	if maxV != 10 {
		t.Errorf("max_size: expected 10, got %d", maxV)
	}
	minCP, _ := extractCBORInt(bg.schema["min_codepoint"])
	if minCP != 65 {
		t.Errorf("min_codepoint: expected 65, got %d", minCP)
	}
}

func TestTextGeneratorExcludesSurrogatesByDefault(t *testing.T) {
	t.Parallel()
	gen := Text()
	bg := gen.buildGenerator().(*basicGenerator[string])
	excl, ok := bg.schema["exclude_categories"].([]string)
	if !ok {
		t.Fatalf("exclude_categories: expected []string, got %T", bg.schema["exclude_categories"])
	}
	found := false
	for _, c := range excl {
		if c == "Cs" {
			found = true
		}
	}
	if !found {
		t.Error("Text() should exclude Cs by default")
	}
}

func TestTextGeneratorCategoriesOverridesDefault(t *testing.T) {
	t.Parallel()
	gen := Text().Categories("Lu", "Ll")
	bg := gen.buildGenerator().(*basicGenerator[string])
	if _, hasExcl := bg.schema["exclude_categories"]; hasExcl {
		t.Error("exclude_categories should not be present when Categories is set")
	}
	cats, ok := bg.schema["categories"].([]string)
	if !ok {
		t.Fatalf("categories: expected []string, got %T", bg.schema["categories"])
	}
	if len(cats) != 2 || cats[0] != "Lu" || cats[1] != "Ll" {
		t.Errorf("categories: expected [Lu Ll], got %v", cats)
	}
}

func TestTextGeneratorSurrogateCategoryPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for Cs category")
		}
	}()
	Text().Categories("Lu", "Cs").buildGenerator()
}

func TestTextGeneratorUnbounded(t *testing.T) {
	t.Parallel()
	gen := Text()
	bg := gen.buildGenerator().(*basicGenerator[string])
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present by default")
	}
}

func TestTextGeneratorNegativeMinPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for negative min_size")
		}
	}()
	Text().MinSize(-1).buildGenerator()
}

func TestTextGeneratorMinGtMaxPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for min_size > max_size")
		}
	}()
	Text().MinSize(10).MaxSize(5).buildGenerator()
}

func TestTextGeneratorBothCategoriesPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for both Categories and ExcludeCategories")
		}
	}()
	Text().Categories("Lu").ExcludeCategories("Cs").buildGenerator()
}

func TestTextGeneratorNegativeMaxPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for negative max_size")
		}
	}()
	Text().MaxSize(-1).buildGenerator()
}

func TestTextGeneratorCodec(t *testing.T) {
	t.Parallel()
	gen := Text().Codec("ascii")
	bg := gen.buildGenerator().(*basicGenerator[string])
	if bg.schema["codec"] != "ascii" {
		t.Errorf("codec: expected 'ascii', got %v", bg.schema["codec"])
	}
}

func TestTextGeneratorIncludeExcludeChars(t *testing.T) {
	t.Parallel()
	gen := Text().IncludeCharacters("abc").ExcludeCharacters("xyz")
	bg := gen.buildGenerator().(*basicGenerator[string])
	if bg.schema["include_characters"] != "abc" {
		t.Errorf("include_characters: expected 'abc', got %v", bg.schema["include_characters"])
	}
	if bg.schema["exclude_characters"] != "xyz" {
		t.Errorf("exclude_characters: expected 'xyz', got %v", bg.schema["exclude_characters"])
	}
}

func TestTextGeneratorExcludeCategoriesPreservesCs(t *testing.T) {
	t.Parallel()
	gen := Text().ExcludeCategories("Zs")
	bg := gen.buildGenerator().(*basicGenerator[string])
	excl, ok := bg.schema["exclude_categories"].([]string)
	if !ok {
		t.Fatalf("exclude_categories: expected []string, got %T", bg.schema["exclude_categories"])
	}
	hasZs, hasCs := false, false
	for _, c := range excl {
		if c == "Zs" {
			hasZs = true
		}
		if c == "Cs" {
			hasCs = true
		}
	}
	if !hasZs || !hasCs {
		t.Errorf("expected both Zs and Cs in exclude_categories, got %v", excl)
	}
}

func TestTextGeneratorExcludeCategoriesWithExplicitCs(t *testing.T) {
	t.Parallel()
	gen := Text().ExcludeCategories("Zs", "Cs")
	bg := gen.buildGenerator().(*basicGenerator[string])
	excl, ok := bg.schema["exclude_categories"].([]string)
	if !ok {
		t.Fatalf("exclude_categories: expected []string, got %T", bg.schema["exclude_categories"])
	}
	csCount := 0
	for _, c := range excl {
		if c == "Cs" {
			csCount++
		}
	}
	if csCount != 1 {
		t.Errorf("expected exactly one Cs in exclude_categories, got %d in %v", csCount, excl)
	}
}

func TestTextGeneratorE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		val := Draw(s, Text().MinSize(1).MaxSize(20).MinCodepoint(65).MaxCodepoint(90))
		if len(val) == 0 {
			panic("Text: got empty string with min_size=1")
		}
		for _, r := range val {
			if r < 'A' || r > 'Z' {
				panic("Text: got character outside A-Z range")
			}
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

func TestListsFloatBuilderUsesBasicPath(t *testing.T) {
	t.Parallel()
	gen := Lists(Floats[float64]().Min(0).Max(1).AllowNaN(false).AllowInfinity(false)).MaxSize(3)

	bg, ok := gen.buildGenerator().(*basicGenerator[[]float64])
	if !ok {
		t.Fatalf("Lists(Floats(...)) should build *basicGenerator[[]float64], got %T", gen.buildGenerator())
	}

	elemSchema, ok := bg.schema["elements"].(map[string]any)
	if !ok {
		t.Fatalf("schema elements: expected map[string]any, got %T", bg.schema["elements"])
	}
	if elemSchema["type"] != "float" {
		t.Fatalf("elements type: expected 'float', got %v", elemSchema["type"])
	}
}
