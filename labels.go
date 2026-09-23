package hegel

import (
	"hash/maphash"
	"reflect"

	"hegel.dev/go/hegel/internal/libhegel"
)

var labelSeed = maphash.MakeSeed()

func labelFor[T any](g Generator[T]) libhegel.Label {
	var h maphash.Hash
	h.SetSeed(labelSeed)
	maphash.WriteComparable(&h, reflect.TypeOf(g))
	return libhegel.Label(h.Sum64())
}

func labelFromName(name string) libhegel.Label {
	return libhegel.Label(maphash.String(labelSeed, name))
}
