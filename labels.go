package hegel

import (
	"hash/maphash"
	"math"
	"reflect"
	"time"

	"hegel.dev/go/hegel/internal/libhegel"
)

var labelSeed = maphash.MakeSeed()

type labelGenerator interface {
	hashFields(*maphash.Hash) bool
}

func hashGenerator(h *maphash.Hash, g labelGenerator) bool {
	value := reflect.ValueOf(g)
	if !value.IsValid() || (value.Kind() == reflect.Pointer && value.IsNil()) {
		return false
	}
	maphash.WriteComparable(h, reflect.TypeOf(g))
	return g.hashFields(h)
}

func labelFor(g labelGenerator) (libhegel.Label, bool) {
	var h maphash.Hash
	h.SetSeed(labelSeed)
	if !hashGenerator(&h, g) {
		return 0, false
	}
	return libhegel.Label(h.Sum64()), true
}

func staticLabel(name string) libhegel.Label {
	return libhegel.Label(maphash.String(labelSeed, name))
}

func hashComparable(h *maphash.Hash, value any) bool {
	rv := reflect.ValueOf(value)
	if rv.IsValid() && (!rv.Comparable() || containsNaN(rv)) {
		return false
	}
	maphash.WriteComparable(h, value)
	return true
}

func containsNaN(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Float32, reflect.Float64:
		return math.IsNaN(value.Float())
	case reflect.Complex64, reflect.Complex128:
		v := value.Complex()
		return math.IsNaN(real(v)) || math.IsNaN(imag(v))
	case reflect.Interface:
		return !value.IsNil() && containsNaN(value.Elem())
	case reflect.Array:
		for i := range value.Len() {
			if containsNaN(value.Index(i)) {
				return true
			}
		}
	case reflect.Struct:
		for _, field := range value.Fields() {
			if containsNaN(field) {
				return true
			}
		}
	}
	return false
}

func hashFunction(h *maphash.Hash, fn any) bool {
	rv := reflect.ValueOf(fn)
	if !rv.IsValid() || rv.Kind() != reflect.Func {
		return false
	}
	maphash.WriteComparable(h, rv.Pointer())
	return true
}

func hashValues(h *maphash.Hash, values ...any) bool {
	for _, value := range values {
		if !hashComparable(h, value) {
			return false
		}
	}
	return true
}

func hashOptional[T any](h *maphash.Hash, value *T) bool {
	_ = hashComparable(h, value != nil)
	if value == nil {
		return true
	}
	return hashComparable(h, *value)
}

func hashOptionalFloat(h *maphash.Hash, value *float64) bool {
	maphash.WriteComparable(h, value != nil)
	if value == nil {
		return true
	}
	return hashComparable(h, math.Float64bits(*value))
}

func hashSlice[T any](h *maphash.Hash, values []T) bool {
	_ = hashComparable(h, len(values))
	for _, value := range values {
		if !hashComparable(h, value) {
			return false
		}
	}
	return true
}

func hashOptionalTime(h *maphash.Hash, value *time.Time, includeTime bool) bool {
	maphash.WriteComparable(h, value != nil)
	if value == nil {
		return true
	}
	maphash.WriteComparable(h, value.Year())
	maphash.WriteComparable(h, value.Month())
	maphash.WriteComparable(h, value.Day())
	return !includeTime || hashValues(h, value.Hour(), value.Minute(), value.Second(), value.Nanosecond())
}
