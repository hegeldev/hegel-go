package hegel

import (
	"fmt"
	"math"
	"net/netip"
	"reflect"
	"sync"
	"time"
)

// Exact types use their domain generator before structural reflection.
var defaultKnownTypes = map[reflect.Type]defaultDraw{
	reflect.TypeFor[time.Time]():  defaultFromGenerator[time.Time](reflect.TypeFor[time.Time](), Datetimes()),
	reflect.TypeFor[netip.Addr](): defaultFromGenerator[netip.Addr](reflect.TypeFor[netip.Addr](), IPAddresses()),
}

var defaultGenerators = struct {
	sync.RWMutex
	byType map[reflect.Type]any // Values are Generator[T] for the key type T.
}{byType: make(map[reflect.Type]any)}

// DefaultOption configures a generator constructed by [Default].
//
// Use [WithGenerator] to construct an option; the zero value is invalid.
type DefaultOption struct {
	typ  reflect.Type
	draw defaultDraw
}

// WithGenerator overrides the default generator for T.
//
// The generator applies to every occurrence of the exact type T, including
// nested fields and collection elements. It takes precedence over built-in
// generators and allows types that Default otherwise cannot construct.
//
// WithGenerator returns an option for [Default].
func WithGenerator[T any](g Generator[T]) DefaultOption {
	t := reflect.TypeFor[T]()
	return DefaultOption{typ: t, draw: defaultFromGenerator(t, g)}
}

// Default constructs a generator for T.
//
// Supported types are booleans, integers, floats, strings, arrays, slices, maps,
// pointers, and structs with exported fields, including named types. The element
// type of a zero-length array is not inspected. Integers use the full range of
// their type. Slices and maps may be empty, and pointers may be nil.
// Well-known types use their specialized generators: time.Time uses Datetimes,
// netip.Addr uses IPAddresses, and uuid.UUID uses UUIDs (Go 1.27 and later).
//
// Options override generators for exact types; the last option for a type wins.
//
// Default returns the same cached generator for each type when no options are
// supplied. Calls with options construct a fresh generator. It panics at
// construction for invalid options or recursive or unsupported types that are
// not handled by an override.
func Default[T any](options ...DefaultOption) Generator[T] {
	t := reflect.TypeFor[T]()
	if len(options) > 0 {
		overrides := make(map[reflect.Type]defaultDraw, len(options))
		for _, option := range options {
			if option.typ == nil {
				panic(fmt.Sprintf("Default[%s]: invalid zero-value option", t))
			}
			overrides[option.typ] = option.draw
		}
		gen, err := newDefaultGenerator[T](overrides)
		if err != nil {
			panic(fmt.Sprintf("Default[%s]: %v", t, err))
		}
		return gen
	}
	defaultGenerators.RLock()
	cached := defaultGenerators.byType[t]
	defaultGenerators.RUnlock()
	if cached != nil {
		return cached.(Generator[T])
	}
	defaultGenerators.Lock()
	defer defaultGenerators.Unlock()
	cached = defaultGenerators.byType[t]
	if cached == nil {
		gen, err := newDefaultGenerator[T](nil)
		if err != nil {
			panic(fmt.Sprintf("Default[%s]: %v", t, err))
		}
		cached = gen
		defaultGenerators.byType[t] = cached
	}
	return cached.(Generator[T])
}

func newDefaultGenerator[T any](overrides map[reflect.Type]defaultDraw) (Generator[T], error) {
	shape, err := buildDefault(reflect.TypeFor[T](), make(map[reflect.Type]*defaultShape), make(map[reflect.Type]bool), overrides)
	if err != nil {
		return nil, err
	}
	drawValue := compileDefault(shape, make(map[*defaultShape]defaultDraw))
	return &defaultGenerator[T]{drawValue: drawValue}, nil
}

type defaultDraw func(TestCase) (reflect.Value, error)

type defaultShape struct {
	draw   defaultDraw
	typ    reflect.Type
	fields []*defaultShape // Struct fields in declaration order.
	elem   *defaultShape   // Element type, or map value type.
	key    *defaultShape
}

func buildDefault(t reflect.Type, nodes map[reflect.Type]*defaultShape, active map[reflect.Type]bool, overrides map[reflect.Type]defaultDraw) (*defaultShape, error) {
	if active[t] {
		return nil, fmt.Errorf("recursive types are unsupported: %s", t)
	}
	if n := nodes[t]; n != nil {
		return n, nil
	}

	n := &defaultShape{typ: t, draw: overrides[t]}
	if n.draw != nil {
		nodes[t] = n
		return n, nil
	}
	if defaultKnownTypes[t] != nil {
		nodes[t] = n
		return n, nil
	}
	active[t] = true
	defer delete(active, t)

	var err error

	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String:
	case reflect.Pointer, reflect.Slice:
		n.elem, err = buildDefault(t.Elem(), nodes, active, overrides)
	case reflect.Array:
		if t.Len() > 0 {
			n.elem, err = buildDefault(t.Elem(), nodes, active, overrides)
		}
	case reflect.Map:
		n.key, err = buildDefault(t.Key(), nodes, active, overrides)
		if err == nil {
			n.elem, err = buildDefault(t.Elem(), nodes, active, overrides)
		}
	case reflect.Struct:
		for field := range t.Fields() {
			if !field.IsExported() {
				return nil, fmt.Errorf("field %s is unexported in %s", field.Name, t)
			}
			child, err := buildDefault(field.Type, nodes, active, overrides)
			if err != nil {
				return nil, err
			}
			n.fields = append(n.fields, child)
		}
	default:
		return nil, fmt.Errorf("unsupported kind %s", t.Kind())
	}
	if err != nil {
		return nil, err
	}
	nodes[t] = n
	return n, nil
}

func compileDefault(shape *defaultShape, compiled map[*defaultShape]defaultDraw) defaultDraw {
	if shape.draw != nil {
		return shape.draw
	}
	if drawValue := compiled[shape]; drawValue != nil {
		return drawValue
	}

	fields := make([]defaultDraw, len(shape.fields))
	for i, child := range shape.fields {
		fields[i] = compileDefault(child, compiled)
	}
	var elem, key defaultDraw
	if shape.elem != nil {
		elem = compileDefault(shape.elem, compiled)
	}
	if shape.key != nil {
		key = compileDefault(shape.key, compiled)
	}
	drawValue := makeDefaultDraw(shape.typ, fields, elem, key)
	compiled[shape] = drawValue
	return drawValue
}

func makeDefaultDraw(t reflect.Type, fields []defaultDraw, elem, key defaultDraw) defaultDraw {
	if drawValue := defaultKnownTypes[t]; drawValue != nil {
		return drawValue
	}
	switch t.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String:
		return defaultPrimitive(t)
	case reflect.Struct:
		return func(tc TestCase) (reflect.Value, error) {
			v := reflect.New(t).Elem()
			for i, child := range fields {
				value, err := child(tc)
				if err != nil {
					return reflect.Value{}, err
				}
				v.Field(i).Set(value)
			}
			return v, nil
		}
	case reflect.Array:
		return func(tc TestCase) (reflect.Value, error) {
			v := reflect.New(t).Elem()
			for i := 0; i < t.Len(); i++ {
				value, err := elem(tc)
				if err != nil {
					return reflect.Value{}, err
				}
				v.Index(i).Set(value)
			}
			return v, nil
		}
	case reflect.Pointer:
		return func(tc TestCase) (reflect.Value, error) {
			ctx, nativeTC := tc.engine()
			present, err := nativeTC.GenerateBoolean(ctx, 0.5, false, false)
			if err != nil {
				return reflect.Value{}, err
			}
			if !present {
				return reflect.Zero(t), nil
			}
			value, err := elem(tc)
			if err != nil {
				return reflect.Value{}, err
			}
			v := reflect.New(t.Elem())
			v.Elem().Set(value)
			if v.Type() != t {
				v = v.Convert(t)
			}
			return v, nil
		}
	case reflect.Slice:
		return func(tc TestCase) (reflect.Value, error) {
			coll, err := tc.newCollection(0, nil)
			if err != nil {
				return reflect.Value{}, err
			}
			v := reflect.MakeSlice(t, 0, 0)
			for coll.More() {
				value, err := elem(tc)
				if err != nil {
					return reflect.Value{}, err
				}
				v = reflect.Append(v, value)
			}
			return v, coll.Err()
		}
	case reflect.Map:
		return func(tc TestCase) (reflect.Value, error) {
			coll, err := tc.newCollection(0, nil)
			if err != nil {
				return reflect.Value{}, err
			}
			v := reflect.MakeMap(t)
			for coll.More() {
				mapKey, err := key(tc)
				if err != nil {
					return reflect.Value{}, err
				}
				if v.MapIndex(mapKey).IsValid() {
					coll.Reject("duplicate key")
					continue
				}
				value, err := elem(tc)
				if err != nil {
					return reflect.Value{}, err
				}
				v.SetMapIndex(mapKey, value)
			}
			return v, coll.Err()
		}
	}
	panic("unreachable")
}

type defaultGenerator[T any] struct {
	drawValue defaultDraw
}

func (g *defaultGenerator[T]) draw(tc TestCase) (T, error) {
	var zero T
	value, err := g.drawValue(tc)
	if err != nil {
		return zero, err
	}
	reflect.ValueOf(&zero).Elem().Set(value)
	return zero, nil
}

func defaultPrimitive(t reflect.Type) defaultDraw {
	switch t.Kind() {
	case reflect.Bool:
		return defaultFromGenerator(t, Booleans())
	case reflect.String:
		return defaultFromGenerator(t, Text())
	case reflect.Float32:
		return defaultFromGenerator(t, Floats[float32]())
	case reflect.Float64:
		return defaultFromGenerator(t, Floats[float64]())
	case reflect.Int:
		return defaultFromGenerator(t, Integers(math.MinInt, math.MaxInt))
	case reflect.Int8:
		return defaultFromGenerator(t, Integers[int8](math.MinInt8, math.MaxInt8))
	case reflect.Int16:
		return defaultFromGenerator(t, Integers[int16](math.MinInt16, math.MaxInt16))
	case reflect.Int32:
		return defaultFromGenerator(t, Integers[int32](math.MinInt32, math.MaxInt32))
	case reflect.Int64:
		return defaultFromGenerator(t, Integers[int64](math.MinInt64, math.MaxInt64))
	case reflect.Uint:
		return defaultFromGenerator(t, Integers[uint](0, math.MaxUint))
	case reflect.Uint8:
		return defaultFromGenerator(t, Integers[uint8](0, math.MaxUint8))
	case reflect.Uint16:
		return defaultFromGenerator(t, Integers[uint16](0, math.MaxUint16))
	case reflect.Uint32:
		return defaultFromGenerator(t, Integers[uint32](0, math.MaxUint32))
	case reflect.Uint64:
		return defaultFromGenerator(t, Integers[uint64](0, math.MaxUint64))
	case reflect.Uintptr:
		return defaultFromGenerator(t, Integers[uintptr](0, math.MaxUint))
	}
	panic("unreachable")
}

func defaultFromGenerator[T any](t reflect.Type, g Generator[T]) defaultDraw {
	return func(tc TestCase) (reflect.Value, error) {
		v, err := draw(tc, g)
		return reflect.ValueOf(&v).Elem().Convert(t), err
	}
}
