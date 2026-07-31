RELEASE_TYPE: patch

This patch makes the libhegel cache best-effort. Previously, hegel-go failed to load if it could not write the embedded libhegel to the per-user cache directory (`~/.cache/hegel-go` on Linux, `~/Library/Caches/hegel-go` on macOS), so sandboxed environments that deny writes outside the workspace — such as Codex's macOS sandbox — could not run hegel tests at all ([#115](https://github.com/hegeldev/hegel-go/issues/115)).

The cache is now purely a performance optimization: when it cannot be read or written, the library is extracted to a fresh directory under the system temp dir instead, and loading fails only when that also fails.
