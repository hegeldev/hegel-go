package hegel

import (
	"testing"

	cbor "github.com/fxamacker/cbor/v2"
)

// cborEncode is a test helper that encodes a value to CBOR bytes.
func cborEncode(t *testing.T, v any) []byte {
	t.Helper()
	b, err := cbor.Marshal(v)
	if err != nil {
		t.Fatalf("cbor.Marshal(%v): %v", v, err)
	}
	return b
}

// cborDecodeAny decodes CBOR bytes to any (interface{}).
func cborDecodeAny(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	dm, _ := cbor.DecOptions{}.DecMode()
	if err := dm.Unmarshal(b, &v); err != nil {
		t.Fatalf("cbor.Unmarshal: %v", err)
	}
	return v
}

// --- CBOR round-trip ---

func TestCBORRoundtripInt(t *testing.T) {
	for _, v := range []int64{0, 1, -1, 127, -128, 1<<31 - 1, -(1 << 31)} {
		b := cborEncode(t, v)
		var got int64
		if err := cbor.Unmarshal(b, &got); err != nil {
			t.Errorf("Unmarshal int64 %d: %v", v, err)
		}
		if got != v {
			t.Errorf("int64 round-trip: got %d, want %d", got, v)
		}
	}
}

func TestCBORRoundtripString(t *testing.T) {
	for _, v := range []string{"", "hello", "unicode: \u00e9", "a longer string with spaces"} {
		b := cborEncode(t, v)
		var got string
		if err := cbor.Unmarshal(b, &got); err != nil {
			t.Errorf("Unmarshal string %q: %v", v, err)
		}
		if got != v {
			t.Errorf("string round-trip: got %q, want %q", got, v)
		}
	}
}

func TestCBORRoundtripBool(t *testing.T) {
	for _, v := range []bool{true, false} {
		b := cborEncode(t, v)
		var got bool
		if err := cbor.Unmarshal(b, &got); err != nil {
			t.Errorf("Unmarshal bool %v: %v", v, err)
		}
		if got != v {
			t.Errorf("bool round-trip: got %v, want %v", got, v)
		}
	}
}

func TestCBORRoundtripFloat(t *testing.T) {
	for _, v := range []float64{0.0, 1.5, -3.14, 1e100} {
		b := cborEncode(t, v)
		var got float64
		if err := cbor.Unmarshal(b, &got); err != nil {
			t.Errorf("Unmarshal float64 %v: %v", v, err)
		}
		if got != v {
			t.Errorf("float64 round-trip: got %v, want %v", got, v)
		}
	}
}

func TestCBORRoundtripBytes(t *testing.T) {
	v := []byte{0x00, 0xFF, 0x42, 0xAB}
	b := cborEncode(t, v)
	var got []byte
	if err := cbor.Unmarshal(b, &got); err != nil {
		t.Errorf("Unmarshal bytes: %v", err)
	}
	if len(got) != len(v) {
		t.Errorf("bytes round-trip length: got %d, want %d", len(got), len(v))
	}
	for i := range v {
		if got[i] != v[i] {
			t.Errorf("bytes round-trip[%d]: got 0x%02X, want 0x%02X", i, got[i], v[i])
		}
	}
}

func TestCBORRoundtripNull(t *testing.T) {
	b := cborEncode(t, nil)
	var got any
	if err := cbor.Unmarshal(b, &got); err != nil {
		t.Errorf("Unmarshal null: %v", err)
	}
	if got != nil {
		t.Errorf("null round-trip: got %v, want nil", got)
	}
}

func TestCBORRoundtripList(t *testing.T) {
	v := []any{int64(1), "two", true}
	b := cborEncode(t, v)
	var got []any
	if err := cbor.Unmarshal(b, &got); err != nil {
		t.Errorf("Unmarshal list: %v", err)
	}
	if len(got) != len(v) {
		t.Errorf("list round-trip length: got %d, want %d", len(got), len(v))
	}
}

func TestCBORRoundtripDict(t *testing.T) {
	v := map[string]any{"key": "value", "num": int64(42)}
	b := cborEncode(t, v)
	var got map[string]any
	if err := cbor.Unmarshal(b, &got); err != nil {
		t.Errorf("Unmarshal dict: %v", err)
	}
	if len(got) != len(v) {
		t.Errorf("dict round-trip length: got %d, want %d", len(got), len(v))
	}
}

func TestCBORRoundtripNested(t *testing.T) {
	v := []any{map[string]any{"x": int64(1)}, map[string]any{"y": []any{int64(2), int64(3)}}}
	b := cborEncode(t, v)
	var got []any
	if err := cbor.Unmarshal(b, &got); err != nil {
		t.Errorf("Unmarshal nested: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("nested round-trip length: got %d, want 2", len(got))
	}
}

// --- CBOR extractor helper tests ---

func TestExtractCBORInt(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, int64(42)))
	v, err := extractCBORInt(decoded)
	if err != nil {
		t.Fatalf("extractCBORInt: %v", err)
	}
	if v != 42 {
		t.Errorf("extractCBORInt = %d, want 42", v)
	}
}

func TestExtractCBORIntWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, "not an int"))
	_, err := extractCBORInt(decoded)
	if err == nil {
		t.Fatal("extractCBORInt with string: expected error")
	}
}

func TestExtractCBORFloat(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, 3.14))
	v, err := extractCBORFloat(decoded)
	if err != nil {
		t.Fatalf("extractCBORFloat: %v", err)
	}
	if v != 3.14 {
		t.Errorf("extractCBORFloat = %v, want 3.14", v)
	}
}

func TestExtractCBORFloatFromInt(t *testing.T) {
	// Integers should also be extractable as floats (common protocol pattern)
	decoded := cborDecodeAny(t, cborEncode(t, int64(7)))
	v, err := extractCBORFloat(decoded)
	if err != nil {
		t.Fatalf("extractCBORFloat from int: %v", err)
	}
	if v != 7.0 {
		t.Errorf("extractCBORFloat from int = %v, want 7.0", v)
	}
}

func TestExtractCBORFloatWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, "not a float"))
	_, err := extractCBORFloat(decoded)
	if err == nil {
		t.Fatal("extractCBORFloat with string: expected error")
	}
}

func TestExtractCBORString(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, "hello"))
	v, err := extractCBORString(decoded)
	if err != nil {
		t.Fatalf("extractCBORString: %v", err)
	}
	if v != "hello" {
		t.Errorf("extractCBORString = %q, want \"hello\"", v)
	}
}

func TestExtractCBORStringWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, int64(42)))
	_, err := extractCBORString(decoded)
	if err == nil {
		t.Fatal("extractCBORString with int: expected error")
	}
}

func TestExtractCBORBool(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, true))
	v, err := extractCBORBool(decoded)
	if err != nil {
		t.Fatalf("extractCBORBool: %v", err)
	}
	if !v {
		t.Errorf("extractCBORBool = %v, want true", v)
	}
}

func TestExtractCBORBoolWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, int64(1)))
	_, err := extractCBORBool(decoded)
	if err == nil {
		t.Fatal("extractCBORBool with int: expected error")
	}
}

func TestExtractCBORBytes(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, []byte{0xDE, 0xAD}))
	v, err := extractCBORBytes(decoded)
	if err != nil {
		t.Fatalf("extractCBORBytes: %v", err)
	}
	if len(v) != 2 || v[0] != 0xDE || v[1] != 0xAD {
		t.Errorf("extractCBORBytes = %v, want [0xDE 0xAD]", v)
	}
}

func TestExtractCBORBytesWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, "not bytes"))
	_, err := extractCBORBytes(decoded)
	if err == nil {
		t.Fatal("extractCBORBytes with string: expected error")
	}
}

func TestExtractCBORList(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, []any{int64(1), int64(2)}))
	v, err := extractCBORList(decoded)
	if err != nil {
		t.Fatalf("extractCBORList: %v", err)
	}
	if len(v) != 2 {
		t.Errorf("extractCBORList length = %d, want 2", len(v))
	}
}

func TestExtractCBORListWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, "not a list"))
	_, err := extractCBORList(decoded)
	if err == nil {
		t.Fatal("extractCBORList with string: expected error")
	}
}

func TestExtractCBORDict(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, map[string]any{"k": "v"}))
	v, err := extractCBORDict(decoded)
	if err != nil {
		t.Fatalf("extractCBORDict: %v", err)
	}
	if len(v) != 1 {
		t.Errorf("extractCBORDict length = %d, want 1", len(v))
	}
}

func TestExtractCBORDictWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, int64(42)))
	_, err := extractCBORDict(decoded)
	if err == nil {
		t.Fatal("extractCBORDict with int: expected error")
	}
}

func TestExtractCBORNullInput(t *testing.T) {
	// Each extractor with nil input should return an error
	if _, err := extractCBORInt(nil); err == nil {
		t.Error("extractCBORInt(nil): expected error")
	}
	if _, err := extractCBORFloat(nil); err == nil {
		t.Error("extractCBORFloat(nil): expected error")
	}
	if _, err := extractCBORString(nil); err == nil {
		t.Error("extractCBORString(nil): expected error")
	}
	if _, err := extractCBORBool(nil); err == nil {
		t.Error("extractCBORBool(nil): expected error")
	}
	if _, err := extractCBORBytes(nil); err == nil {
		t.Error("extractCBORBytes(nil): expected error")
	}
	if _, err := extractCBORList(nil); err == nil {
		t.Error("extractCBORList(nil): expected error")
	}
	if _, err := extractCBORDict(nil); err == nil {
		t.Error("extractCBORDict(nil): expected error")
	}
}

// --- decodeCBOR / encodeCBOR ---

func TestDecodeCBOR(t *testing.T) {
	b := cborEncode(t, int64(99))
	v, err := decodeCBOR(b)
	if err != nil {
		t.Fatalf("decodeCBOR: %v", err)
	}
	n, err := extractCBORInt(v)
	if err != nil || n != 99 {
		t.Errorf("decodeCBOR result: %v, %v", v, err)
	}
}

func TestDecodeCBORError(t *testing.T) {
	// 0xFF is not valid CBOR (break code without indefinite-length context)
	_, err := decodeCBOR([]byte{0xFF})
	if err == nil {
		t.Fatal("decodeCBOR(invalid): expected error")
	}
}

func TestEncodeCBOR(t *testing.T) {
	b, err := encodeCBOR("hello")
	if err != nil {
		t.Fatalf("encodeCBOR: %v", err)
	}
	if len(b) == 0 {
		t.Error("encodeCBOR returned empty bytes")
	}
}

// --- Additional extractor branches ---

func TestExtractCBORIntUint64(t *testing.T) {
	// Directly pass a uint64 (positive CBOR integers decode as uint64 in fxamacker).
	v, err := extractCBORInt(uint64(42))
	if err != nil {
		t.Fatalf("extractCBORInt(uint64): %v", err)
	}
	if v != 42 {
		t.Errorf("extractCBORInt(uint64) = %d, want 42", v)
	}
}

func TestExtractCBORFloatFloat32(t *testing.T) {
	v, err := extractCBORFloat(float32(1.5))
	if err != nil {
		t.Fatalf("extractCBORFloat(float32): %v", err)
	}
	if v != float64(float32(1.5)) {
		t.Errorf("extractCBORFloat(float32) = %v, want %v", v, float64(float32(1.5)))
	}
}

func TestExtractCBORFloatUint64(t *testing.T) {
	v, err := extractCBORFloat(uint64(10))
	if err != nil {
		t.Fatalf("extractCBORFloat(uint64): %v", err)
	}
	if v != 10.0 {
		t.Errorf("extractCBORFloat(uint64) = %v, want 10.0", v)
	}
}

func TestExtractCBORDictStringKeyed(t *testing.T) {
	// Directly pass a map[string]any to test that branch.
	input := map[string]any{"key": "value"}
	m, err := extractCBORDict(input)
	if err != nil {
		t.Fatalf("extractCBORDict(map[string]any): %v", err)
	}
	if len(m) != 1 {
		t.Errorf("extractCBORDict length = %d, want 1", len(m))
	}
}

func TestExtractCBORIntNegative(t *testing.T) {
	// Negative CBOR integers decode as int64 in fxamacker/cbor.
	// Pass int64 directly to ensure the case int64: branch is exercised.
	v, err := extractCBORInt(int64(-42))
	if err != nil {
		t.Fatalf("extractCBORInt(int64 negative): %v", err)
	}
	if v != -42 {
		t.Errorf("extractCBORInt(int64 negative) = %d, want -42", v)
	}
}

func TestExtractCBORFloatNegativeInt(t *testing.T) {
	// Negative integers come as int64 from CBOR decode.
	// Pass int64 directly to exercise the case int64: branch in extractCBORFloat.
	v, err := extractCBORFloat(int64(-3))
	if err != nil {
		t.Fatalf("extractCBORFloat(int64 negative): %v", err)
	}
	if v != -3.0 {
		t.Errorf("extractCBORFloat(int64 negative) = %v, want -3.0", v)
	}
}

func TestEncodeCBORError(t *testing.T) {
	// Functions cannot be CBOR-encoded; this exercises the error return path.
	_, err := encodeCBOR(func() {})
	if err == nil {
		t.Fatal("encodeCBOR(func): expected error")
	}
}
