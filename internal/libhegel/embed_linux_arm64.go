//go:build linux && !android && arm64

package libhegel

import _ "embed"

//go:embed libs/libhegel-linux-arm64.so
var embeddedLib []byte
