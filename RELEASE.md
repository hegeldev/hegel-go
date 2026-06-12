RELEASE_TYPE: minor

This release replaces the Python-server backend with a direct FFI binding to libhegel, the native Rust engine shipped by [hegel-rust](https://github.com/hegeldev/hegel-rust).

The public API is unchanged: `hegel.Test`, `hegel.Run`, `hegel.MustRun`, `hegel.Workload`, every generator, and every option work exactly as before.

What changes is the installation requirement. Instead of `uv` and `hegel-core`, hegel-go now needs `libhegel.so` (Linux) or `libhegel.dylib` (macOS) at runtime. It is resolved in this order:

1. `$HEGEL_LIBHEGEL_PATH` if set.
2. `../hegel-rust/target/release/libhegel.<ext>` (and the `debug` build) relative to your project root, for local development against a sibling hegel-rust checkout.
3. The platform-appropriate binary fetched on first use from the hegel-rust GitHub release matching the embedded version, cached under `~/.cache/hegel-go/libhegel/<version>/`.

The auto-download is verified against a SHA-256 digest baked into the library (not one fetched from the release, which would offer no protection against a tampered release); set `HEGEL_LIBHEGEL_NO_DOWNLOAD` to opt out.
