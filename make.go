package hegel

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// StructGenerator generates struct values with customizable field generators.
type StructGenerator[T any] struct {
	fieldGens map[string]any // field name -> generator (type erased)
	schema    map[string]any // cached schema
	typeInfo  reflect.Type   // struct type info
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
		return g.generateFromTupleSchema(schema)
	}

	// Compositional fallback using reflection
	return g.generateCompositional()
}

func (g *StructGenerator[T]) generateFromTupleSchema(schema map[string]any) T {
	needConnection := !isConnected()
	if needConnection {
		openConnection()
	}

	resultBytes := sendRequest("generate", schema)

	if needConnection {
		closeConnection()
	}

	var values []any
	err := json.Unmarshal(resultBytes, &values)
	if err != nil {
		panic(fmt.Sprintf("hegel: failed to deserialize struct values: %v\nValue: %s", err, resultBytes))
	}

	result := reflect.New(g.typeInfo).Elem()
	valueIdx := 0
	for i := 0; i < g.typeInfo.NumField(); i++ {
		field := g.typeInfo.Field(i)
		if !field.IsExported() {
			continue
		}

		rawValue := values[valueIdx]
		valueIdx++

		fieldValue := convertValue(rawValue, field.Type)
		result.Field(i).Set(fieldValue)
	}

	return result.Interface().(T)
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

	elements := make([]map[string]any, 0)

	for i := 0; i < g.typeInfo.NumField(); i++ {
		field := g.typeInfo.Field(i)
		if !field.IsExported() {
			continue
		}

		gen := g.fieldGens[field.Name]

		fieldSchema := callSchema(gen)
		if fieldSchema == nil {
			return nil // Can't compose schema
		}

		elements = append(elements, fieldSchema)
	}

	g.schema = map[string]any{
		"type":     "tuple",
		"elements": elements,
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
				"type":     "list",
				"elements": elemSchema,
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
				"type":   "dict",
				"values": valueSchema,
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
				"one_of": []map[string]any{
					{"type": "null"},
					innerSchema,
				},
			}
		}(),
	}
}

// convertValue converts a JSON-decoded value to a reflect.Value of the target type.
func convertValue(raw any, targetType reflect.Type) reflect.Value {
	if raw == nil {
		return reflect.Zero(targetType)
	}

	switch targetType.Kind() {
	case reflect.Bool:
		return reflect.ValueOf(raw.(bool))
	case reflect.Int:
		return reflect.ValueOf(int(raw.(float64)))
	case reflect.Int8:
		return reflect.ValueOf(int8(raw.(float64)))
	case reflect.Int16:
		return reflect.ValueOf(int16(raw.(float64)))
	case reflect.Int32:
		return reflect.ValueOf(int32(raw.(float64)))
	case reflect.Int64:
		return reflect.ValueOf(int64(raw.(float64)))
	case reflect.Uint:
		return reflect.ValueOf(uint(raw.(float64)))
	case reflect.Uint8:
		return reflect.ValueOf(uint8(raw.(float64)))
	case reflect.Uint16:
		return reflect.ValueOf(uint16(raw.(float64)))
	case reflect.Uint32:
		return reflect.ValueOf(uint32(raw.(float64)))
	case reflect.Uint64:
		return reflect.ValueOf(uint64(raw.(float64)))
	case reflect.Float32:
		return reflect.ValueOf(float32(raw.(float64)))
	case reflect.Float64:
		return reflect.ValueOf(raw.(float64))
	case reflect.String:
		return reflect.ValueOf(raw.(string))
	case reflect.Slice:
		rawSlice := raw.([]any)
		slice := reflect.MakeSlice(targetType, len(rawSlice), len(rawSlice))
		for i, elem := range rawSlice {
			slice.Index(i).Set(convertValue(elem, targetType.Elem()))
		}
		return slice
	case reflect.Map:
		rawMap := raw.(map[string]any)
		result := reflect.MakeMap(targetType)
		for k, v := range rawMap {
			result.SetMapIndex(reflect.ValueOf(k), convertValue(v, targetType.Elem()))
		}
		return result
	case reflect.Ptr:
		ptr := reflect.New(targetType.Elem())
		ptr.Elem().Set(convertValue(raw, targetType.Elem()))
		return ptr
	default:
		panic(fmt.Sprintf("convertValue: unsupported type %v", targetType))
	}
}
