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

func TestExtractInt(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, int64(42)))
	v, err := extractInt(decoded)
	if err != nil {
		t.Fatalf("extractInt: %v", err)
	}
	if v != 42 {
		t.Errorf("extractInt = %d, want 42", v)
	}
}

func TestExtractIntWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, "not an int"))
	_, err := extractInt(decoded)
	if err == nil {
		t.Fatal("extractInt with string: expected error")
	}
}

func TestExtractFloat(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, 3.14))
	v, err := extractFloat(decoded)
	if err != nil {
		t.Fatalf("extractFloat: %v", err)
	}
	if v != 3.14 {
		t.Errorf("extractFloat = %v, want 3.14", v)
	}
}

func TestExtractFloatFromInt(t *testing.T) {
	// Integers should also be extractable as floats (common protocol pattern)
	decoded := cborDecodeAny(t, cborEncode(t, int64(7)))
	v, err := extractFloat(decoded)
	if err != nil {
		t.Fatalf("extractFloat from int: %v", err)
	}
	if v != 7.0 {
		t.Errorf("extractFloat from int = %v, want 7.0", v)
	}
}

func TestExtractFloatWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, "not a float"))
	_, err := extractFloat(decoded)
	if err == nil {
		t.Fatal("extractFloat with string: expected error")
	}
}

func TestExtractString(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, "hello"))
	v, err := extractString(decoded)
	if err != nil {
		t.Fatalf("extractString: %v", err)
	}
	if v != "hello" {
		t.Errorf("extractString = %q, want \"hello\"", v)
	}
}

func TestExtractStringWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, int64(42)))
	_, err := extractString(decoded)
	if err == nil {
		t.Fatal("extractString with int: expected error")
	}
}

func TestExtractBool(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, true))
	v, err := extractBool(decoded)
	if err != nil {
		t.Fatalf("extractBool: %v", err)
	}
	if !v {
		t.Errorf("extractBool = %v, want true", v)
	}
}

func TestExtractBoolWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, int64(1)))
	_, err := extractBool(decoded)
	if err == nil {
		t.Fatal("extractBool with int: expected error")
	}
}

func TestExtractBytes(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, []byte{0xDE, 0xAD}))
	v, err := extractBytes(decoded)
	if err != nil {
		t.Fatalf("extractBytes: %v", err)
	}
	if len(v) != 2 || v[0] != 0xDE || v[1] != 0xAD {
		t.Errorf("extractBytes = %v, want [0xDE 0xAD]", v)
	}
}

func TestExtractBytesWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, "not bytes"))
	_, err := extractBytes(decoded)
	if err == nil {
		t.Fatal("extractBytes with string: expected error")
	}
}

func TestExtractList(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, []any{int64(1), int64(2)}))
	v, err := extractList(decoded)
	if err != nil {
		t.Fatalf("extractList: %v", err)
	}
	if len(v) != 2 {
		t.Errorf("extractList length = %d, want 2", len(v))
	}
}

func TestExtractListWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, "not a list"))
	_, err := extractList(decoded)
	if err == nil {
		t.Fatal("extractList with string: expected error")
	}
}

func TestExtractDict(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, map[string]any{"k": "v"}))
	v, err := extractDict(decoded)
	if err != nil {
		t.Fatalf("extractDict: %v", err)
	}
	if len(v) != 1 {
		t.Errorf("extractDict length = %d, want 1", len(v))
	}
}

func TestExtractDictWrongType(t *testing.T) {
	decoded := cborDecodeAny(t, cborEncode(t, int64(42)))
	_, err := extractDict(decoded)
	if err == nil {
		t.Fatal("extractDict with int: expected error")
	}
}

func TestExtractNullInput(t *testing.T) {
	// Each extractor with nil input should return an error
	if _, err := extractInt(nil); err == nil {
		t.Error("extractInt(nil): expected error")
	}
	if _, err := extractFloat(nil); err == nil {
		t.Error("extractFloat(nil): expected error")
	}
	if _, err := extractString(nil); err == nil {
		t.Error("extractString(nil): expected error")
	}
	if _, err := extractBool(nil); err == nil {
		t.Error("extractBool(nil): expected error")
	}
	if _, err := extractBytes(nil); err == nil {
		t.Error("extractBytes(nil): expected error")
	}
	if _, err := extractList(nil); err == nil {
		t.Error("extractList(nil): expected error")
	}
	if _, err := extractDict(nil); err == nil {
		t.Error("extractDict(nil): expected error")
	}
}

// --- DecodeCBOR / EncodeCBOR ---

func TestDecodeCBOR(t *testing.T) {
	b := cborEncode(t, int64(99))
	v, err := DecodeCBOR(b)
	if err != nil {
		t.Fatalf("DecodeCBOR: %v", err)
	}
	n, err := extractInt(v)
	if err != nil || n != 99 {
		t.Errorf("DecodeCBOR result: %v, %v", v, err)
	}
}

func TestDecodeCBORError(t *testing.T) {
	// 0xFF is not valid CBOR (break code without indefinite-length context)
	_, err := DecodeCBOR([]byte{0xFF})
	if err == nil {
		t.Fatal("DecodeCBOR(invalid): expected error")
	}
}

func TestEncodeCBOR(t *testing.T) {
	b, err := EncodeCBOR("hello")
	if err != nil {
		t.Fatalf("EncodeCBOR: %v", err)
	}
	if len(b) == 0 {
		t.Error("EncodeCBOR returned empty bytes")
	}
}

// --- Additional extractor branches ---

func TestExtractIntUint64(t *testing.T) {
	// Directly pass a uint64 (positive CBOR integers decode as uint64 in fxamacker).
	v, err := extractInt(uint64(42))
	if err != nil {
		t.Fatalf("extractInt(uint64): %v", err)
	}
	if v != 42 {
		t.Errorf("extractInt(uint64) = %d, want 42", v)
	}
}

func TestExtractFloatFloat32(t *testing.T) {
	v, err := extractFloat(float32(1.5))
	if err != nil {
		t.Fatalf("extractFloat(float32): %v", err)
	}
	if v != float64(float32(1.5)) {
		t.Errorf("extractFloat(float32) = %v, want %v", v, float64(float32(1.5)))
	}
}

func TestExtractFloatUint64(t *testing.T) {
	v, err := extractFloat(uint64(10))
	if err != nil {
		t.Fatalf("extractFloat(uint64): %v", err)
	}
	if v != 10.0 {
		t.Errorf("extractFloat(uint64) = %v, want 10.0", v)
	}
}

func TestExtractDictStringKeyed(t *testing.T) {
	// Directly pass a map[string]any to test that branch.
	input := map[string]any{"key": "value"}
	m, err := extractDict(input)
	if err != nil {
		t.Fatalf("extractDict(map[string]any): %v", err)
	}
	if len(m) != 1 {
		t.Errorf("extractDict length = %d, want 1", len(m))
	}
}

func TestExtractIntNegative(t *testing.T) {
	// Negative CBOR integers decode as int64 in fxamacker/cbor.
	// Pass int64 directly to ensure the case int64: branch is exercised.
	v, err := extractInt(int64(-42))
	if err != nil {
		t.Fatalf("extractInt(int64 negative): %v", err)
	}
	if v != -42 {
		t.Errorf("extractInt(int64 negative) = %d, want -42", v)
	}
}

func TestExtractFloatNegativeInt(t *testing.T) {
	// Negative integers come as int64 from CBOR decode.
	// Pass int64 directly to exercise the case int64: branch in extractFloat.
	v, err := extractFloat(int64(-3))
	if err != nil {
		t.Fatalf("extractFloat(int64 negative): %v", err)
	}
	if v != -3.0 {
		t.Errorf("extractFloat(int64 negative) = %v, want -3.0", v)
	}
}

func TestEncodeCBORError(t *testing.T) {
	// Functions cannot be CBOR-encoded; this exercises the error return path.
	_, err := EncodeCBOR(func() {})
	if err == nil {
		t.Fatal("EncodeCBOR(func): expected error")
	}
}
