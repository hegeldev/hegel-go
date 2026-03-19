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

func (g ListGenerator[T]) buildGenerator() Generator[[]T] {
	if g.minSize < 0 {
		panic(fmt.Sprintf("hegel: min_size=%d must be non-negative", g.minSize))
	}
	if g.hasMax && g.maxSize < 0 {
		panic(fmt.Sprintf("hegel: max_size=%d must be non-negative", g.maxSize))
	}
	if g.hasMax && g.minSize > g.maxSize {
		panic(fmt.Sprintf("hegel: Cannot have max_size=%d < min_size=%d", g.maxSize, g.minSize))
	}

	elements := unwrapGenerator(g.elements)
	if bg, ok := elements.(*basicGenerator[T]); ok {
		rawSchema := map[string]any{
			"type":     "list",
			"elements": bg.schema,
			"min_size": int64(g.minSize),
		}
		if g.hasMax {
			rawSchema["max_size"] = int64(g.maxSize)
		}
		if bg.transform != nil {
			t := bg.transform
			return &basicGenerator[[]T]{
				schema: rawSchema,
				transform: func(raw any) []T {
					rawSlice, ok := raw.([]any)
					if !ok {
						return nil
					}
					result := make([]T, len(rawSlice))
					for i, x := range rawSlice {
						result[i] = t(x)
					}
					return result
				},
			}
		}
		return &basicGenerator[[]T]{
			schema: rawSchema,
			transform: func(raw any) []T {
				rawSlice, ok := raw.([]any)
				if !ok {
					return nil
				}
				result := make([]T, len(rawSlice))
				for i, x := range rawSlice {
					result[i] = x.(T)
				}
				return result
			},
		}
	}

	maxSize := -1
	if g.hasMax {
		maxSize = g.maxSize
	}
	return &compositeListGenerator[T]{
		elements: elements,
		minSize:  g.minSize,
		maxSize:  maxSize,
	}
}

// draw produces a list by delegating to the effective generator.
func (g ListGenerator[T]) draw(s *TestCase) []T {
	return g.buildGenerator().draw(s)
}

// compositeListGenerator generates a list using the collection protocol.
type compositeListGenerator[T any] struct {
	elements Generator[T]
	minSize  int
	maxSize  int
}

// draw produces a list by using the collection protocol inside a labelList span.
func (g *compositeListGenerator[T]) draw(s *TestCase) []T {
	var result []T
	startSpan(s, labelList)
	panicked := true
	defer func() {
		stopSpan(s, panicked)
	}()
	coll := newCollection(s, g.minSize, g.maxSize)
	for coll.More(s) {
		result = append(result, g.elements.draw(s))
	}
	panicked = false
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

func (g DictGenerator[K, V]) buildGenerator() Generator[map[K]V] {
	if g.minSize < 0 {
		panic(fmt.Sprintf("hegel: min_size=%d must be non-negative", g.minSize))
	}
	if g.hasMax && g.maxSize < 0 {
		panic(fmt.Sprintf("hegel: max_size=%d must be non-negative", g.maxSize))
	}
	if g.hasMax && g.minSize > g.maxSize {
		panic(fmt.Sprintf("hegel: Cannot have max_size=%d < min_size=%d", g.maxSize, g.minSize))
	}

	keys := unwrapGenerator(g.keys)
	values := unwrapGenerator(g.values)
	keyBasic, keyIsBasic := keys.(*basicGenerator[K])
	valBasic, valIsBasic := values.(*basicGenerator[V])
	if keyIsBasic && valIsBasic {
		rawSchema := map[string]any{
			"type":     "dict",
			"keys":     keyBasic.schema,
			"values":   valBasic.schema,
			"min_size": int64(g.minSize),
		}
		if g.hasMax {
			rawSchema["max_size"] = int64(g.maxSize)
		}
		keyTransform := keyBasic.transform
		valTransform := valBasic.transform
		return &basicGenerator[map[K]V]{
			schema: rawSchema,
			transform: func(v any) map[K]V {
				return pairsToMap[K, V](v, keyTransform, valTransform)
			},
		}
	}
	return &compositeDictGenerator[K, V]{
		keys:    keys,
		values:  values,
		minSize: g.minSize,
		maxSize: g.maxSize,
		hasMax:  g.hasMax,
	}
}

// draw produces a map by delegating to the effective generator.
func (g DictGenerator[K, V]) draw(s *TestCase) map[K]V {
	return g.buildGenerator().draw(s)
}

// pairsToMap converts a CBOR-decoded pair list [[k,v], ...] to a map[K]V.
func pairsToMap[K comparable, V any](v any, keyTransform func(any) K, valTransform func(any) V) map[K]V {
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
		var k K
		if keyTransform != nil {
			k = keyTransform(kv[0])
		} else {
			k = kv[0].(K)
		}
		var val V
		if valTransform != nil {
			val = valTransform(kv[1])
		} else {
			val = kv[1].(V)
		}
		result[k] = val
	}
	return result
}

// compositeDictGenerator generates maps using the collection protocol.
type compositeDictGenerator[K comparable, V any] struct {
	keys    Generator[K]
	values  Generator[V]
	minSize int
	maxSize int
	hasMax  bool
}

// draw implements Generator by using the MAP span and collection protocol.
func (g *compositeDictGenerator[K, V]) draw(s *TestCase) map[K]V {
	var result map[K]V
	discardableGroup(s, labelMap, func() {
		maxSz := g.maxSize
		if !g.hasMax {
			maxSz = g.minSize + 10
		}
		coll := newCollection(s, g.minSize, maxSz)
		m := map[K]V{}
		for coll.More(s) {
			group(s, labelMapEntry, func() {
				k := g.keys.draw(s)
				v := g.values.draw(s)
				m[k] = v
			})
		}
		result = m
	})
	return result
}
