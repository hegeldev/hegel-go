RELEASE_TYPE: patch

This release adds support for stateful property testing via `hegel.RunStateful`.

This release also makes `*hegel.TestCase` compatible with the `TestingT` interfaces used by popular assertion libraries (testify, gotest.tools, gomega). Assertions from those libraries can now be used directly inside `Composite` callbacks, `Run` bodies, and stateful rules, where only a `*TestCase` is available.
