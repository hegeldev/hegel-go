RELEASE_TYPE: patch

Embedded `libhegel` binaries now fall back to a secure per-user temporary cache when the normal user cache is unavailable, allowing tests to run in sandboxes that prohibit writes outside the workspace.
