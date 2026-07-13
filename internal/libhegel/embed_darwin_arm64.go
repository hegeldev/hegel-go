//go:build darwin && !ios && arm64

package libhegel

import _ "embed"

//go:embed libs/libhegel-darwin-arm64.dylib
var embeddedLib []byte
