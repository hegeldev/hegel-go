package hegel

import (
	"fmt"
	"reflect"
)

// --- Lists generator ---

// ListGenerator configures and generates slices of values from an element generator.
// Use [Lists] to create one, then chain builder methods to configure bounds.
// Invalid configurations panic on the first [Draw] call.
type ListGenerator[T any] struct {
	elements Generator[T]
	minSize  int
	maxSize  int
	hasMax   bool
	unique   bool
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

// Unique requires every element of the generated list to be distinct.
// On the basic path, uniqueness is delegated to the server via the list schema.
// On the compositional path, duplicates are rejected via the collection protocol
// using [reflect.DeepEqual] for equality.
func (g ListGenerator[T]) Unique(unique bool) ListGenerator[T] {
	g.unique = unique
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
		return nil, false, fmt.Errorf("cannot have max_size=%d < min_size=%d", g.maxSize, g.minSize)
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
	if g.unique {
		rawSchema["unique"] = true
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
// falling back to the collection protocol otherwise. When unique is set on
// the compositional path, duplicates are rejected via the collection protocol
// so the server can retry until min_size distinct elements have been drawn.
func (g ListGenerator[T]) draw(s *TestCase) []T {
	bg, ok, err := g.asBasic()
	if err != nil {
		panic(err.Error())
	}
	if ok {
		return bg.draw(s)
	}
	var maxSize *int
	if g.hasMax {
		m := g.maxSize
		maxSize = &m
	}
	startSpan(s, labelList)
	result := []T{}
	coll := newCollection(s, g.minSize, maxSize)
	for coll.More(s) {
		element := g.elements.draw(s)
		if g.unique {
			duplicate := false
			for _, existing := range result {
				if reflect.DeepEqual(existing, element) {
					duplicate = true
					break
				}
			}
			if duplicate {
				coll.Reject(s)
				continue
			}
		}
		result = append(result, element)
	}
	stopSpan(s, false)
	return result
}

// --- Maps generator ---

// MapGenerator configures and generates map[K]V values.
// Use [Maps] to create one, then chain builder methods to configure bounds.
// Invalid configurations panic on the first [Draw] call.
type MapGenerator[K comparable, V any] struct {
	keys    Generator[K]
	values  Generator[V]
	minSize int
	maxSize int
	hasMax  bool
}

// Maps returns a Generator that produces map[K]V values.
func Maps[K comparable, V any](keys Generator[K], values Generator[V]) MapGenerator[K, V] {
	return MapGenerator[K, V]{keys: keys, values: values}
}

// MinSize sets the minimum number of key-value pairs. Default: 0.
func (g MapGenerator[K, V]) MinSize(n int) MapGenerator[K, V] {
	g.minSize = n
	return g
}

// MaxSize sets the maximum number of key-value pairs.
func (g MapGenerator[K, V]) MaxSize(n int) MapGenerator[K, V] {
	g.maxSize = n
	g.hasMax = true
	return g
}

// asBasic validates the configuration and returns a basic generator when both
// key and value generators are basic. Returns (nil, false, nil) when either
// is non-basic.
func (g MapGenerator[K, V]) asBasic() (*basicGenerator[map[K]V], bool, error) {
	if g.minSize < 0 {
		return nil, false, fmt.Errorf("min_size=%d must be non-negative", g.minSize)
	}
	if g.hasMax && g.maxSize < 0 {
		return nil, false, fmt.Errorf("max_size=%d must be non-negative", g.maxSize)
	}
	if g.hasMax && g.minSize > g.maxSize {
		return nil, false, fmt.Errorf("cannot have max_size=%d < min_size=%d", g.maxSize, g.minSize)
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
func (g MapGenerator[K, V]) draw(s *TestCase) map[K]V {
	bg, ok, err := g.asBasic()
	if err != nil {
		panic(err.Error())
	}
	if ok {
		return bg.draw(s)
	}
	var maxSize *int
	if g.hasMax {
		m := g.maxSize
		maxSize = &m
	}
	startSpan(s, labelMap)
	result := map[K]V{}
	coll := newCollection(s, g.minSize, maxSize)
	for coll.More(s) {
		startSpan(s, labelMapEntry)
		k := g.keys.draw(s)
		if _, exists := result[k]; exists {
			stopSpan(s, false)
			coll.Reject(s)
			continue
		}
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
