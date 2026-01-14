package hegel

import (
	"fmt"
	"reflect"
	"strings"
)

// StructGenerator generates struct values with customizable field generators.
type StructGenerator[T any] struct {
	fieldGens map[string]any    // field name -> generator (type erased)
	schema    map[string]any    // cached schema
	typeInfo  reflect.Type      // struct type info
}

// Make returns a generator for struct type T.
// T must be a struct with exported fields.
// Each field is generated using a default generator based on its type.
//
// Example:
//
//	type Person struct {
//	    Name string `json:"name"`
//	    Age  int    `json:"age"`
//	}
//
//	person := Make[Person]().
//	    With("Age", Integers[int]().Min(18).Max(65)).
//	    Generate()
func Make[T any]() *StructGenerator[T] {
	var zero T
	t := reflect.TypeOf(zero)

	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		panic(fmt.Sprintf("Make: T must be a struct type, got %v", t.Kind()))
	}

	g := &StructGenerator[T]{
		fieldGens: make(map[string]any),
		typeInfo:  t,
	}

	// Build default generators for each exported field
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		g.fieldGens[field.Name] = defaultGeneratorForType(field.Type)
	}

	return g
}

// With overrides the generator for a specific field.
// The fieldName must match an exported field name (not the json tag).
func (g *StructGenerator[T]) With(fieldName string, gen any) *StructGenerator[T] {
	if _, exists := g.fieldGens[fieldName]; !exists {
		panic(fmt.Sprintf("Make.With: unknown field %q", fieldName))
	}
	g.fieldGens[fieldName] = gen
	g.schema = nil // Invalidate cached schema
	return g
}

// Generate produces a struct value.
func (g *StructGenerator[T]) Generate() T {
	if schema := g.Schema(); schema != nil {
		return generateFromSchema[T](schema)
	}

	// Compositional fallback using reflection
	return g.generateCompositional()
}

func (g *StructGenerator[T]) generateCompositional() T {
	result := reflect.New(g.typeInfo).Elem()

	for i := 0; i < g.typeInfo.NumField(); i++ {
		field := g.typeInfo.Field(i)
		if !field.IsExported() {
			continue
		}

		gen := g.fieldGens[field.Name]
		value := callGenerate(gen)
		result.Field(i).Set(reflect.ValueOf(value))
	}

	return result.Interface().(T)
}

// Schema returns the JSON schema for this generator, or nil if unavailable.
func (g *StructGenerator[T]) Schema() map[string]any {
	if g.schema != nil {
		return g.schema
	}

	properties := make(map[string]any)
	required := make([]string, 0)

	for i := 0; i < g.typeInfo.NumField(); i++ {
		field := g.typeInfo.Field(i)
		if !field.IsExported() {
			continue
		}

		jsonName := getJSONFieldName(field)
		gen := g.fieldGens[field.Name]

		fieldSchema := callSchema(gen)
		if fieldSchema == nil {
			return nil // Can't compose schema
		}

		properties[jsonName] = fieldSchema
		required = append(required, jsonName)
	}

	g.schema = map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   required,
	}

	return g.schema
}

// getJSONFieldName returns the JSON field name for a struct field.
func getJSONFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" || tag == "-" {
		return field.Name
	}
	parts := strings.Split(tag, ",")
	if parts[0] == "" {
		return field.Name
	}
	return parts[0]
}

// defaultGeneratorForType returns a default generator for a given reflect.Type.
func defaultGeneratorForType(t reflect.Type) any {
	switch t.Kind() {
	case reflect.Bool:
		return Booleans()
	case reflect.Int:
		return Integers[int]()
	case reflect.Int8:
		return Integers[int8]()
	case reflect.Int16:
		return Integers[int16]()
	case reflect.Int32:
		return Integers[int32]()
	case reflect.Int64:
		return Integers[int64]()
	case reflect.Uint:
		return Integers[uint]()
	case reflect.Uint8:
		return Integers[uint8]()
	case reflect.Uint16:
		return Integers[uint16]()
	case reflect.Uint32:
		return Integers[uint32]()
	case reflect.Uint64:
		return Integers[uint64]()
	case reflect.Float32:
		return Floats[float32]()
	case reflect.Float64:
		return Floats[float64]()
	case reflect.String:
		return Text()
	case reflect.Slice:
		elemGen := defaultGeneratorForType(t.Elem())
		return wrapSliceGenerator(elemGen, t.Elem())
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			panic(fmt.Sprintf("Make: map key must be string, got %v", t.Key().Kind()))
		}
		valueGen := defaultGeneratorForType(t.Elem())
		return wrapMapGenerator(valueGen, t.Elem())
	case reflect.Ptr:
		innerGen := defaultGeneratorForType(t.Elem())
		return wrapOptionalGenerator(innerGen, t.Elem())
	default:
		panic(fmt.Sprintf("Make: unsupported field type %v", t))
	}
}

// callGenerate calls Generate() on a type-erased generator.
func callGenerate(gen any) any {
	genVal := reflect.ValueOf(gen)
	method := genVal.MethodByName("Generate")
	if !method.IsValid() {
		panic("generator does not have Generate method")
	}
	results := method.Call(nil)
	return results[0].Interface()
}

// callSchema calls Schema() on a type-erased generator.
func callSchema(gen any) map[string]any {
	genVal := reflect.ValueOf(gen)
	method := genVal.MethodByName("Schema")
	if !method.IsValid() {
		return nil
	}
	results := method.Call(nil)
	if results[0].IsNil() {
		return nil
	}
	return results[0].Interface().(map[string]any)
}

// wrapSliceGenerator creates a slice generator for a given element type.
func wrapSliceGenerator(elemGen any, elemType reflect.Type) any {
	// Create a FuncGenerator that generates slices
	return &FuncGenerator[any]{
		genFn: func() any {
			length := Integers[int]().Min(0).Max(10).Generate()
			slice := reflect.MakeSlice(reflect.SliceOf(elemType), length, length)
			for i := 0; i < length; i++ {
				value := callGenerate(elemGen)
				slice.Index(i).Set(reflect.ValueOf(value))
			}
			return slice.Interface()
		},
		schema: func() map[string]any {
			elemSchema := callSchema(elemGen)
			if elemSchema == nil {
				return nil
			}
			return map[string]any{
				"type":  "array",
				"items": elemSchema,
			}
		}(),
	}
}

// wrapMapGenerator creates a map generator for a given value type.
func wrapMapGenerator(valueGen any, valueType reflect.Type) any {
	mapType := reflect.MapOf(reflect.TypeOf(""), valueType)

	return &FuncGenerator[any]{
		genFn: func() any {
			length := Integers[int]().Min(0).Max(5).Generate()
			result := reflect.MakeMap(mapType)

			keyGen := Text().MinSize(1).MaxSize(10)
			for i := 0; i < length; i++ {
				key := keyGen.Generate()
				value := callGenerate(valueGen)
				result.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(value))
			}
			return result.Interface()
		},
		schema: func() map[string]any {
			valueSchema := callSchema(valueGen)
			if valueSchema == nil {
				return nil
			}
			return map[string]any{
				"type":                 "object",
				"additionalProperties": valueSchema,
			}
		}(),
	}
}

// wrapOptionalGenerator creates an optional generator for a given inner type.
func wrapOptionalGenerator(innerGen any, innerType reflect.Type) any {
	ptrType := reflect.PointerTo(innerType)

	return &FuncGenerator[any]{
		genFn: func() any {
			isNil := Booleans().Generate()
			if isNil {
				return reflect.Zero(ptrType).Interface()
			}
			value := callGenerate(innerGen)
			ptr := reflect.New(innerType)
			ptr.Elem().Set(reflect.ValueOf(value))
			return ptr.Interface()
		},
		schema: func() map[string]any {
			innerSchema := callSchema(innerGen)
			if innerSchema == nil {
				return nil
			}
			return map[string]any{
				"anyOf": []map[string]any{
					{"type": "null"},
					innerSchema,
				},
			}
		}(),
	}
}
