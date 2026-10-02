package hegel

import (
	"fmt"
	"math"
	"reflect"
)

// Default constructs a generator from the shape of T. Supported types are
// booleans, integers, floats, strings, arrays, slices, maps, pointers, and
// structs with exported fields. Unsupported types panic when Default is called.
// The element of a zero-length array is not inspected because it is never drawn.
func Default[T any]() Generator[T] {
	t := reflect.TypeFor[T]()
	b := defaultBuilder{nodes: make(map[reflect.Type]*defaultNode), active: make(map[reflect.Type]int)}
	root := b.build(t)
	b.compile(root, make(map[*defaultNode]bool))
	return &defaultGenerator[T]{root: root, recursive: b.recursive}
}

type defaultDraw func(TestCase, defaultState) (reflect.Value, error)

type defaultNode struct {
	shape defaultShape // Populated by build.
	draw  defaultDraw  // Populated by compile.
}

type defaultShape struct {
	typ    reflect.Type   // Type represented by this node.
	fields []*defaultNode // Struct fields in declaration order.
	elem   *defaultNode   // Element type, or map value type.
	key    *defaultNode   // Map key type.
	guard  bool           // Recursive edge cut at a leaf.
}

type defaultBuilder struct {
	nodes     map[reflect.Type]*defaultNode
	active    map[reflect.Type]int
	stack     []*defaultNode
	recursive bool
}

func (b *defaultBuilder) build(t reflect.Type) *defaultNode {
	if n := b.nodes[t]; n != nil {
		if from, ok := b.active[t]; ok {
			// Every Go type cycle must pass through an indirection. Cut the
			// innermost pointer or collection on this cycle at a leaf.
			for i := len(b.stack) - 1; i >= from; i-- {
				candidate := b.stack[i]
				switch candidate.shape.typ.Kind() {
				case reflect.Pointer, reflect.Slice, reflect.Map:
					candidate.shape.guard = true
					b.recursive = true
					return n
				}
			}
			panic(fmt.Sprintf("Default[%s]: recursive type has no terminating edge", t))
		}
		return n
	}

	n := &defaultNode{shape: defaultShape{typ: t}}
	b.nodes[t] = n
	b.active[t] = len(b.stack)
	b.stack = append(b.stack, n)
	defer func() {
		b.stack = b.stack[:len(b.stack)-1]
		delete(b.active, t)
	}()

	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String:
	case reflect.Pointer, reflect.Slice:
		n.shape.elem = b.build(t.Elem())
	case reflect.Array:
		if t.Len() > 0 {
			n.shape.elem = b.build(t.Elem())
		}
	case reflect.Map:
		n.shape.key = b.build(t.Key())
		n.shape.elem = b.build(t.Elem())
	case reflect.Struct:
		for field := range t.Fields() {
			if !field.IsExported() {
				panic(fmt.Sprintf("Default[%s]: field %s is unexported", t, field.Name))
			}
			n.shape.fields = append(n.shape.fields, b.build(field.Type))
		}
	default:
		panic(fmt.Sprintf("Default[%s]: unsupported kind %s", t, t.Kind()))
	}
	return n
}

func (b *defaultBuilder) compile(n *defaultNode, seen map[*defaultNode]bool) {
	if seen[n] {
		return
	}
	seen[n] = true
	n.draw = n.makeDraw()
	for _, child := range n.shape.fields {
		b.compile(child, seen)
	}
	if n.shape.elem != nil {
		b.compile(n.shape.elem, seen)
	}
	if n.shape.key != nil {
		b.compile(n.shape.key, seen)
	}
}

func (n *defaultNode) makeDraw() defaultDraw {
	t := n.shape.typ
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String:
		return defaultPrimitive(t)
	case reflect.Struct:
		return func(tc TestCase, state defaultState) (reflect.Value, error) {
			v := reflect.New(t).Elem()
			for i, child := range n.shape.fields {
				value, err := child.draw(tc, state)
				if err != nil {
					return reflect.Value{}, err
				}
				v.Field(i).Set(value)
			}
			return v, nil
		}
	case reflect.Array:
		return func(tc TestCase, state defaultState) (reflect.Value, error) {
			v := reflect.New(t).Elem()
			for i := 0; i < t.Len(); i++ {
				value, err := n.shape.elem.draw(tc, state)
				if err != nil {
					return reflect.Value{}, err
				}
				v.Index(i).Set(value)
			}
			return v, nil
		}
	case reflect.Pointer:
		guard := n.shape.guard
		return n.withGuard(func(tc TestCase, state defaultState) (reflect.Value, error) {
			if !guard {
				ctx, nativeTC := tc.engine()
				present, err := nativeTC.GenerateBoolean(ctx, 0.5, false, false)
				if err != nil {
					return reflect.Value{}, err
				}
				if !present {
					return reflect.Zero(t), nil
				}
			}
			value, err := n.shape.elem.draw(tc, state)
			if err != nil {
				return reflect.Value{}, err
			}
			v := reflect.New(t.Elem())
			v.Elem().Set(value)
			if v.Type() != t {
				v = v.Convert(t)
			}
			return v, nil
		})
	case reflect.Slice:
		minSize := 0
		if n.shape.guard {
			minSize = 1
		}
		return n.withGuard(func(tc TestCase, state defaultState) (reflect.Value, error) {
			coll, err := tc.newCollection(minSize, nil)
			if err != nil {
				return reflect.Value{}, err
			}
			v := reflect.MakeSlice(t, 0, 0)
			for coll.More() {
				value, err := n.shape.elem.draw(tc, state)
				if err != nil {
					return reflect.Value{}, err
				}
				v = reflect.Append(v, value)
			}
			return v, coll.Err()
		})
	case reflect.Map:
		minSize := 0
		if n.shape.guard {
			minSize = 1
		}
		return n.withGuard(func(tc TestCase, state defaultState) (reflect.Value, error) {
			coll, err := tc.newCollection(minSize, nil)
			if err != nil {
				return reflect.Value{}, err
			}
			v := reflect.MakeMap(t)
			for coll.More() {
				key, err := n.shape.key.draw(tc, state)
				if err != nil {
					return reflect.Value{}, err
				}
				if v.MapIndex(key).IsValid() {
					coll.Reject("duplicate key")
					continue
				}
				value, err := n.shape.elem.draw(tc, state)
				if err != nil {
					return reflect.Value{}, err
				}
				v.SetMapIndex(key, value)
			}
			return v, coll.Err()
		})
	}
	panic("unreachable")
}

func (n *defaultNode) withGuard(body defaultDraw) defaultDraw {
	if !n.shape.guard {
		return body
	}
	return func(tc TestCase, state defaultState) (reflect.Value, error) {
		if state.leaf {
			return reflect.Zero(n.shape.typ), nil
		}
		branch, err := state.choose(state.depth + 1)
		if err != nil {
			return reflect.Value{}, err
		}
		if !branch {
			return reflect.Zero(n.shape.typ), nil
		}
		state.depth++
		return body(tc, state)
	}
}

type defaultState struct {
	choose recursionChoice
	depth  uint64
	leaf   bool
}

type defaultGenerator[T any] struct {
	root      *defaultNode
	recursive bool
}

func (g *defaultGenerator[T]) draw(tc TestCase) (T, error) {
	var zero T
	if !g.recursive {
		value, err := g.root.draw(tc, defaultState{})
		if err != nil {
			return zero, err
		}
		return value.Interface().(T), nil
	}
	value, err := runRecursion(tc, defaultRecursiveMaxDepth, defaultRecursiveMaxLeaves, nil,
		func(attempt TestCase, choose recursionChoice) (reflect.Value, error) {
			branch, err := choose(0)
			if err != nil {
				return reflect.Value{}, err
			}
			return g.root.draw(attempt, defaultState{choose: choose, leaf: !branch})
		})
	if err != nil {
		return zero, err
	}
	return value.Interface().(T), nil
}
func defaultPrimitive(t reflect.Type) defaultDraw {
	convert := func(v any) reflect.Value { return reflect.ValueOf(v).Convert(t) }
	switch t.Kind() {
	case reflect.Bool:
		g := Booleans()
		return func(tc TestCase, _ defaultState) (reflect.Value, error) { v, e := draw(tc, g); return convert(v), e }
	case reflect.String:
		g := Text()
		return func(tc TestCase, _ defaultState) (reflect.Value, error) { v, e := draw(tc, g); return convert(v), e }
	case reflect.Float32:
		g := Floats[float32]()
		return func(tc TestCase, _ defaultState) (reflect.Value, error) { v, e := draw(tc, g); return convert(v), e }
	case reflect.Float64:
		g := Floats[float64]()
		return func(tc TestCase, _ defaultState) (reflect.Value, error) { v, e := draw(tc, g); return convert(v), e }
	case reflect.Int:
		return defaultInteger(t, Integers(math.MinInt, math.MaxInt))
	case reflect.Int8:
		return defaultInteger(t, Integers[int8](math.MinInt8, math.MaxInt8))
	case reflect.Int16:
		return defaultInteger(t, Integers[int16](math.MinInt16, math.MaxInt16))
	case reflect.Int32:
		return defaultInteger(t, Integers[int32](math.MinInt32, math.MaxInt32))
	case reflect.Int64:
		return defaultInteger(t, Integers[int64](math.MinInt64, math.MaxInt64))
	case reflect.Uint:
		return defaultInteger(t, Integers[uint](0, math.MaxUint))
	case reflect.Uint8:
		return defaultInteger(t, Integers[uint8](0, math.MaxUint8))
	case reflect.Uint16:
		return defaultInteger(t, Integers[uint16](0, math.MaxUint16))
	case reflect.Uint32:
		return defaultInteger(t, Integers[uint32](0, math.MaxUint32))
	case reflect.Uint64:
		return defaultInteger(t, Integers[uint64](0, math.MaxUint64))
	case reflect.Uintptr:
		return defaultInteger(t, Integers[uintptr](0, math.MaxUint))
	}
	panic("unreachable")
}

func defaultInteger[T integer](t reflect.Type, g Generator[T]) defaultDraw {
	return func(tc TestCase, _ defaultState) (reflect.Value, error) {
		v, err := draw(tc, g)
		return reflect.ValueOf(v).Convert(t), err
	}
}
