RELEASE_TYPE: patch

This patch improves the output of failing tests. When a test fails, Hegel now emits a line for every `hegel.Draw` call made during the final replay, echoing the call site, the source statement, and the drawn value:

```
    example_test.go:46: slice1 := hegel.Draw(...) = []int{0, 0}
```

If the source file isn't available, a synthesized statement is used instead:

```
    example_test.go:46: hegel.Draw[[]int](...) = []int{0, 0}
```

`hegel.T`'s methods, `hegel.Draw`, and other internal frames are also marked as test helpers, so `file:line` decoration from `t.Log`, `t.Fatal`, and friends now points at the user's test body rather than into Hegel's internals.
