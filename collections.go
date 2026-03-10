package hegel

import "fmt"

// --- Lists generator ---

// ListOption configures optional behavior for the [Lists] generator.
type ListOption func(*listConfig)

type listConfig struct {
	minSize int
	maxSize int
}

// ListMinSize sets the minimum number of elements (inclusive). Defaults to 0.
func ListMinSize(n int) ListOption {
	return func(cfg *listConfig) { cfg.minSize = n }
}

// ListMaxSize sets the maximum number of elements (inclusive). Negative means unbounded.
func ListMaxSize(n int) ListOption {
	return func(cfg *listConfig) { cfg.maxSize = n }
}

// Lists returns a Generator that produces slices of values from the elements generator.
func Lists[T any](elements Generator[T], opts ...ListOption) Generator[[]T] {
	cfg := listConfig{maxSize: -1}
	for _, o := range opts {
		o(&cfg)
	}

	minSize := max(cfg.minSize, 0)
	if cfg.maxSize >= 0 && minSize > cfg.maxSize {
		panic(fmt.Sprintf("hegel: Cannot have max_size=%d < min_size=%d", cfg.maxSize, minSize))
	}

	if bg, ok := elements.(*basicGenerator[T]); ok {
		rawSchema := map[string]any{
			"type":     "list",
			"elements": bg.schema,
			"min_size": int64(minSize),
		}
		if cfg.maxSize >= 0 {
			rawSchema["max_size"] = int64(cfg.maxSize)
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

	return &compositeListGenerator[T]{
		elements: elements,
		minSize:  minSize,
		maxSize:  cfg.maxSize,
	}
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

// DictOption configures optional behavior for the [Dicts] generator.
type DictOption func(*dictConfig)

type dictConfig struct {
	minSize int
	maxSize int
	hasMax  bool
}

// DictMinSize sets the minimum number of key-value pairs. Defaults to 0.
func DictMinSize(n int) DictOption {
	return func(cfg *dictConfig) { cfg.minSize = n }
}

// DictMaxSize sets the maximum number of key-value pairs.
func DictMaxSize(n int) DictOption {
	return func(cfg *dictConfig) { cfg.maxSize = n; cfg.hasMax = true }
}

// Dicts returns a Generator that produces map[K]V values.
func Dicts[K comparable, V any](keys Generator[K], values Generator[V], opts ...DictOption) Generator[map[K]V] {
	var cfg dictConfig
	for _, o := range opts {
		o(&cfg)
	}
	keyBasic, keyIsBasic := keys.(*basicGenerator[K])
	valBasic, valIsBasic := values.(*basicGenerator[V])
	if keyIsBasic && valIsBasic {
		rawSchema := map[string]any{
			"type":     "dict",
			"keys":     keyBasic.schema,
			"values":   valBasic.schema,
			"min_size": int64(cfg.minSize),
		}
		if cfg.hasMax {
			rawSchema["max_size"] = int64(cfg.maxSize)
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
		minSize: cfg.minSize,
		maxSize: cfg.maxSize,
		hasMax:  cfg.hasMax,
	}
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
