package hegel

import (
	"fmt"

	cbor "github.com/fxamacker/cbor/v2"
)

func convertCBOR(v any) any {
	switch x := v.(type) {
	case cbor.Tag:
		if x.Number == 91 {
			b, ok := x.Content.([]byte)
			if !ok {
				panic(fmt.Sprintf("tag 91 content: expected []byte, got %T", x.Content))
			}
			return string(b)
		}
		return v
	case []any:
		for i, elem := range x {
			x[i] = convertCBOR(elem)
		}
		return x
	case map[any]any:
		for k, val := range x {
			x[k] = convertCBOR(val)
		}
		return x
	default:
		return v
	}
}

func decodeCBOR(data []byte) (any, error) {
	var v any
	if err := cbor.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("CBOR decode: %w", err)
	}
	return convertCBOR(v), nil
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
