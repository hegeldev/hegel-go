package hegel

import (
	"hash/maphash"
	"math"
	"reflect"
	"time"

	"hegel.dev/go/hegel/internal/libhegel"
)

var labelSeed = maphash.MakeSeed()

func hashGenerator[T any](h *maphash.Hash, g Generator[T]) bool {
	maphash.WriteComparable(h, reflect.TypeOf(g))
	return g.hashFields(h)
}

func labelFor[T any](g Generator[T]) (libhegel.Label, bool) {
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

func hashValue(h *maphash.Hash, value any) bool {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		maphash.WriteComparable(h, value)
		return true
	}

	switch rv.Kind() {
	case reflect.Float32:
		maphash.WriteComparable(h, rv.Type())
		maphash.WriteComparable(h, math.Float32bits(float32(rv.Float())))
		return true
	case reflect.Float64:
		maphash.WriteComparable(h, rv.Type())
		maphash.WriteComparable(h, math.Float64bits(rv.Float()))
		return true
	case reflect.Complex64:
		v := complex64(rv.Complex())
		maphash.WriteComparable(h, rv.Type())
		maphash.WriteComparable(h, math.Float32bits(real(v)))
		maphash.WriteComparable(h, math.Float32bits(imag(v)))
		return true
	case reflect.Complex128:
		v := rv.Complex()
		maphash.WriteComparable(h, rv.Type())
		maphash.WriteComparable(h, math.Float64bits(real(v)))
		maphash.WriteComparable(h, math.Float64bits(imag(v)))
		return true
	case reflect.Func:
		maphash.WriteComparable(h, rv.Type())
		maphash.WriteComparable(h, rv.Pointer())
		return true
	}

	if !rv.Comparable() {
		return false
	}
	maphash.WriteComparable(h, value)
	return true
}

func hashValues(h *maphash.Hash, values ...any) bool {
	for _, value := range values {
		if !hashValue(h, value) {
			return false
		}
	}
	return true
}

func hashOptional[T any](h *maphash.Hash, value *T) bool {
	_ = hashValue(h, value != nil)
	if value == nil {
		return true
	}
	return hashValue(h, *value)
}

func hashSlice[T any](h *maphash.Hash, values []T) bool {
	_ = hashValue(h, len(values))
	for _, value := range values {
		if !hashValue(h, value) {
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
