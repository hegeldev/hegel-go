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
		sv := Draw[string](s, Text(2, 8))
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
		sv := Draw[string](s, Text(0, -1))
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

func TestTextWithAlphabetSchema(t *testing.T) {
	t.Parallel()
	gen := TextWithAlphabet(1, 10, map[string]any{
		"min_codepoint": int64(65),
		"max_codepoint": int64(90),
	})
	bg := gen.(*basicGenerator[string])
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

func TestTextWithAlphabetUnbounded(t *testing.T) {
	t.Parallel()
	gen := TextWithAlphabet(0, -1, map[string]any{})
	bg := gen.(*basicGenerator[string])
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when maxSize < 0")
	}
}

func TestTextWithAlphabetNegativeMinPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for negative min_size")
		}
	}()
	TextWithAlphabet(-1, 10, map[string]any{})
}

func TestTextWithAlphabetMinGtMaxPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic for min_size > max_size")
		}
	}()
	TextWithAlphabet(10, 5, map[string]any{})
}

func TestTextWithAlphabetE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		val := Draw(s, TextWithAlphabet(1, 20, map[string]any{
			"min_codepoint": int64(65),
			"max_codepoint": int64(90),
		}))
		if len(val) == 0 {
			panic("TextWithAlphabet: got empty string with min_size=1")
		}
		for _, r := range val {
			if r < 'A' || r > 'Z' {
				panic("TextWithAlphabet: got character outside A-Z range")
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
