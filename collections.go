package hegel

// --- Lists generator ---

// ListsOptions holds optional size constraints for the Lists generator.
type ListsOptions struct {
	// MinSize is the minimum number of elements (inclusive). Defaults to 0.
	MinSize int
	// MaxSize is the maximum number of elements (inclusive). Negative means unbounded.
	MaxSize int
}

// Lists returns a Generator that produces slices of values from the elements generator.
func Lists[T any](elements Generator[T], opts ListsOptions) Generator[[]T] {
	minSize := opts.MinSize
	if minSize < 0 {
		minSize = 0
	}

	if bg, ok := elements.(*basicGenerator[T]); ok {
		rawSchema := map[string]any{
			"type":     "list",
			"elements": bg.schema,
			"min_size": int64(minSize),
		}
		if opts.MaxSize >= 0 {
			rawSchema["max_size"] = int64(opts.MaxSize)
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
		maxSize:  opts.MaxSize,
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

// DictOptions holds optional parameters for the Dicts generator.
type DictOptions struct {
	// MinSize is the minimum number of key-value pairs. Defaults to 0.
	MinSize int
	// MaxSize is the maximum number of key-value pairs. Negative means unbounded.
	MaxSize int
	// HasMaxSize indicates whether MaxSize should be applied.
	HasMaxSize bool
}

// Dicts returns a Generator that produces map[K]V values.
func Dicts[K comparable, V any](keys Generator[K], values Generator[V], opts DictOptions) Generator[map[K]V] {
	keyBasic, keyIsBasic := keys.(*basicGenerator[K])
	valBasic, valIsBasic := values.(*basicGenerator[V])
	if keyIsBasic && valIsBasic {
		rawSchema := map[string]any{
			"type":     "dict",
			"keys":     keyBasic.schema,
			"values":   valBasic.schema,
			"min_size": int64(opts.MinSize),
		}
		if opts.HasMaxSize {
			rawSchema["max_size"] = int64(opts.MaxSize)
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
		minSize: opts.MinSize,
		maxSize: opts.MaxSize,
		hasMax:  opts.HasMaxSize,
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
