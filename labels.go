package hegel

import (
	"hash/maphash"
	"reflect"

	"hegel.dev/go/hegel/internal/libhegel"
)

var labelSeed = maphash.MakeSeed()

func hashValue(seed maphash.Seed, v any) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	value := reflect.ValueOf(v)
	if value.IsValid() && value.Comparable() {
		maphash.WriteComparable(&h, v)
	} else {
		maphash.WriteComparable(&h, reflect.TypeOf(v))
	}
	return h.Sum64()
}

func labelFor(v any) libhegel.Label {
	return libhegel.Label(hashValue(labelSeed, v))
}
