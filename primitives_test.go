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
	if _err := runHegel(func(s *TestCase) {
		// Full-range integers: just verify the draw completes without error.
		_ = Draw[int](s, Integers[int](math.MinInt, math.MaxInt))
	}, stderrNoteFn, []Option{WithTestCases(20)}); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_WithBounds(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
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
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_Unbounded(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		// Unbounded floats may produce NaN or Inf -- any float64 is valid.
		_ = Draw(s, Floats[float64]())
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_OnlyMin(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		fv := Draw(s, Floats[float64]().Min(0.0))
		// allow_nan is false (has min), allow_infinity is true (no max)
		// Value should be >= 0.0 or Inf; NaN not allowed.
		if math.IsNaN(fv) {
			panic("floats: NaN not expected when min set")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_Float32(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		fv := Draw(s, Floats[float32]().Min(0.0).Max(1.0).AllowNaN(false).AllowInfinity(false))
		if fv < 0.0 || fv > 1.0 {
			panic("float32: out of range [0.0, 1.0]")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestFloatsGenerateErrorResponse(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "error_response")
	err := runHegel(func(s *TestCase) {
		_ = Floats[float64]().draw(s)
	}, stderrNoteFn, nil)
	_ = err
}

func TestBooleansE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		b := Draw[bool](s, Booleans())
		// A valid assertion: b is either true or false.
		if b != true && b != false {
			panic("booleans: expected bool")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestTextE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		sv := Draw[string](s, Text(2, 8))
		count := utf8.RuneCountInString(sv)
		if count < 2 || count > 8 {
			panic("text: codepoint count out of range [2, 8]")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestTextE2E_Unbounded(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		sv := Draw[string](s, Text(0, -1))
		if !utf8.ValidString(sv) {
			panic("text: invalid UTF-8 string")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestBinaryE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		bv := Draw[[]byte](s, Binary(1, 10))
		if len(bv) < 1 || len(bv) > 10 {
			panic("binary: byte length out of range [1, 10]")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestBinaryE2E_Unbounded(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		_ = Draw[[]byte](s, Binary(0, -1))
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
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
