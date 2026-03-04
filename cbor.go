package hegel

import (
	"fmt"

	cbor "github.com/fxamacker/cbor/v2"
)

// decMode is a package-level CBOR decode mode used for all decoding.
// It is safe for concurrent use.
var decMode = mustDecMode()

// mustDecMode creates a CBOR decode mode from default options.
// DecOptions{} is always valid, so this never panics in practice.
func mustDecMode() cbor.DecMode {
	m, err := cbor.DecOptions{}.DecMode()
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: failed to create CBOR decode mode: %v", err))
	}
	return m
}

// decodeCBOR decodes CBOR-encoded bytes into a generic Go value (any).
// Maps decode to map[interface{}]interface{} by default.
func decodeCBOR(data []byte) (any, error) {
	var v any
	if err := decMode.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("CBOR decode: %w", err)
	}
	return v, nil
}

// encodeCBOR encodes a Go value to CBOR bytes.
func encodeCBOR(v any) ([]byte, error) {
	b, err := cbor.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("CBOR encode: %w", err)
	}
	return b, nil
}

// extractCBORInt extracts an integer value from a CBOR-decoded value.
// The fxamacker/cbor library decodes CBOR integers as uint64 (positive) or
// int64 (negative), so both are handled.
func extractCBORInt(v any) (int64, error) {
	switch x := v.(type) {
	case int64:
		return x, nil
	case uint64:
		return int64(x), nil
	default:
		return 0, fmt.Errorf("expected int, got %T", v)
	}
}

// extractCBORFloat extracts a float64 from a CBOR-decoded value.
func extractCBORFloat(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case uint64:
		return float64(x), nil
	default:
		return 0, fmt.Errorf("expected float, got %T", v)
	}
}

// extractCBORString extracts a string from a CBOR-decoded value.
func extractCBORString(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("expected string, got %T", v)
	}
	return s, nil
}

// extractCBORBool extracts a bool from a CBOR-decoded value.
func extractCBORBool(v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("expected bool, got %T", v)
	}
	return b, nil
}

// extractCBORBytes extracts a []byte from a CBOR-decoded value.
func extractCBORBytes(v any) ([]byte, error) {
	b, ok := v.([]byte)
	if !ok {
		return nil, fmt.Errorf("expected bytes, got %T", v)
	}
	return b, nil
}

// extractCBORList extracts a []any slice from a CBOR-decoded value.
func extractCBORList(v any) ([]any, error) {
	l, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("expected list, got %T", v)
	}
	return l, nil
}

// extractCBORDict extracts a map[any]any from a CBOR-decoded value.
func extractCBORDict(v any) (map[any]any, error) {
	switch m := v.(type) {
	case map[any]any:
		return m, nil
	case map[string]any:
		out := make(map[any]any, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected dict, got %T", v)
	}
}
