//go:build linux && !android && amd64

package libhegel

import _ "embed"

// embeddedLib is the vendored libhegel binary for this GOOS/GOARCH, embedded
// from libs/ at build time. It is materialized to a file and dlopen'd as the
// last-resort loader candidate. See embed_other.go for the empty fallback on
// platforms without a vendored artifact.
//
//go:embed libs/libhegel-linux-amd64.so
var embeddedLib []byte
