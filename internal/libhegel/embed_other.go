//go:build !((linux && !android && (amd64 || arm64)) || (darwin && !ios && arm64) || (windows && (amd64 || arm64)))

package libhegel

// embeddedLib is empty on platforms with no vendored libhegel binary (e.g.
// darwin/amd64). The loader skips the embedded candidate and reports a clear
// error if local discovery also fails.
var embeddedLib []byte
