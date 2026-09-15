package hegel

import (
	"hash/maphash"

	"hegel.dev/go/hegel/internal/libhegel"
)

type label string

var labelSeed = maphash.MakeSeed()

func (l label) hash() libhegel.Label {
	// Frontend span labels are process-local identities owned by this package.
	// Shrinking compares them only within one process, so local hashing does not
	// require libhegel's LabelFromName or LabelCombine APIs.
	return libhegel.Label(maphash.String(labelSeed, string(l)))
}
