package hegel

import "fmt"

// --- Lists generator ---

// ListGenerator configures and generates slices of values from an element generator.
// Use [Lists] to create one, then chain builder methods to configure bounds.
// Invalid configurations panic on the first [Draw] call.
type ListGenerator[T any] struct {
	elements Generator[T]
	minSize  int
	maxSize  int
	hasMax   bool
}

// Lists returns a Generator that produces slices of values from the elements generator.
func Lists[T any](elements Generator[T]) ListGenerator[T] {
	return ListGenerator[T]{elements: elements}
}

// MinSize sets the minimum number of elements (inclusive). Default: 0.
func (g ListGenerator[T]) MinSize(n int) ListGenerator[T] {
	g.minSize = n
	return g
}

// MaxSize sets the maximum number of elements (inclusive).
func (g ListGenerator[T]) MaxSize(n int) ListGenerator[T] {
	g.maxSize = n
	g.hasMax = true
	return g
}

// asBasic validates the configuration and returns a basic generator when the
// element generator is itself basic. Returns (nil, false, nil) when the
// element generator is non-basic.
func (g ListGenerator[T]) asBasic() (*basicGenerator[[]T], bool, error) {
	if g.minSize < 0 {
		return nil, false, fmt.Errorf("min_size=%d must be non-negative", g.minSize)
	}
	if g.hasMax && g.maxSize < 0 {
		return nil, false, fmt.Errorf("max_size=%d must be non-negative", g.maxSize)
	}
	if g.hasMax && g.minSize > g.maxSize {
		return nil, false, fmt.Errorf("Cannot have max_size=%d < min_size=%d", g.maxSize, g.minSize)
	}

	bg, ok, err := g.elements.asBasic()
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	rawSchema := map[string]any{
		"type":     "list",
		"elements": bg.schema,
		"min_size": int64(g.minSize),
	}
	if g.hasMax {
		rawSchema["max_size"] = int64(g.maxSize)
	}
	elemParse := bg.parse
	return &basicGenerator[[]T]{
		schema: rawSchema,
		parse: func(raw any) []T {
			rawSlice, ok := raw.([]any)
			if !ok {
				return nil
			}
			result := make([]T, len(rawSlice))
			for i, x := range rawSlice {
				result[i] = elemParse(x)
			}
			return result
		},
	}, true, nil
}

// draw produces a list by dispatching to the basic schema when possible,
// falling back to the collection protocol otherwise.
func (g ListGenerator[T]) draw(s *TestCase) []T {
	bg, ok, err := g.asBasic()
	if err != nil {
		panic(err.Error())
	}
	if ok {
		return bg.draw(s)
	}
	maxSize := -1
	if g.hasMax {
		maxSize = g.maxSize
	}
	startSpan(s, labelList)
	var result []T
	coll := newCollection(s, g.minSize, maxSize)
	for coll.More(s) {
		result = append(result, g.elements.draw(s))
	}
	stopSpan(s, false)
	return result
}

// --- Dicts generator ---

// DictGenerator configures and generates map[K]V values.
// Use [Dicts] to create one, then chain builder methods to configure bounds.
// Invalid configurations panic on the first [Draw] call.
type DictGenerator[K comparable, V any] struct {
	keys    Generator[K]
	values  Generator[V]
	minSize int
	maxSize int
	hasMax  bool
}

// Dicts returns a Generator that produces map[K]V values.
func Dicts[K comparable, V any](keys Generator[K], values Generator[V]) DictGenerator[K, V] {
	return DictGenerator[K, V]{keys: keys, values: values}
}

// MinSize sets the minimum number of key-value pairs. Default: 0.
func (g DictGenerator[K, V]) MinSize(n int) DictGenerator[K, V] {
	g.minSize = n
	return g
}

// MaxSize sets the maximum number of key-value pairs.
func (g DictGenerator[K, V]) MaxSize(n int) DictGenerator[K, V] {
	g.maxSize = n
	g.hasMax = true
	return g
}

// asBasic validates the configuration and returns a basic generator when both
// key and value generators are basic. Returns (nil, false, nil) when either
// is non-basic.
func (g DictGenerator[K, V]) asBasic() (*basicGenerator[map[K]V], bool, error) {
	if g.minSize < 0 {
		return nil, false, fmt.Errorf("min_size=%d must be non-negative", g.minSize)
	}
	if g.hasMax && g.maxSize < 0 {
		return nil, false, fmt.Errorf("max_size=%d must be non-negative", g.maxSize)
	}
	if g.hasMax && g.minSize > g.maxSize {
		return nil, false, fmt.Errorf("Cannot have max_size=%d < min_size=%d", g.maxSize, g.minSize)
	}

	keyBasic, keyOk, err := g.keys.asBasic()
	if err != nil {
		return nil, false, err
	}
	valBasic, valOk, err := g.values.asBasic()
	if err != nil {
		return nil, false, err
	}
	if !keyOk || !valOk {
		return nil, false, nil
	}

	rawSchema := map[string]any{
		"type":     "dict",
		"keys":     keyBasic.schema,
		"values":   valBasic.schema,
		"min_size": int64(g.minSize),
	}
	if g.hasMax {
		rawSchema["max_size"] = int64(g.maxSize)
	}
	keyParse := keyBasic.parse
	valParse := valBasic.parse
	return &basicGenerator[map[K]V]{
		schema: rawSchema,
		parse: func(v any) map[K]V {
			return pairsToMap[K, V](v, keyParse, valParse)
		},
	}, true, nil
}

// draw produces a map by dispatching to the basic schema when possible,
// falling back to the collection protocol otherwise.
func (g DictGenerator[K, V]) draw(s *TestCase) map[K]V {
	bg, ok, err := g.asBasic()
	if err != nil {
		panic(err.Error())
	}
	if ok {
		return bg.draw(s)
	}
	maxSize := g.maxSize
	if !g.hasMax {
		maxSize = g.minSize + 10
	}
	startSpan(s, labelMap)
	result := map[K]V{}
	coll := newCollection(s, g.minSize, maxSize)
	for coll.More(s) {
		startSpan(s, labelMapEntry)
		k := g.keys.draw(s)
		v := g.values.draw(s)
		result[k] = v
		stopSpan(s, false)
	}
	stopSpan(s, false)
	return result
}

// pairsToMap converts a CBOR-decoded pair list [[k,v], ...] to a map[K]V.
func pairsToMap[K comparable, V any](v any, keyParse func(any) K, valParse func(any) V) map[K]V {
	result := map[K]V{}
	pairs, ok := v.([]any)
	if !ok {
		return result
	}
	for _, pair := range pairs {
		kv, ok := pair.([]any)
		if !ok || len(kv) < 2 {
			continue
		}
		result[keyParse(kv[0])] = valParse(kv[1])
	}
	return result
}
