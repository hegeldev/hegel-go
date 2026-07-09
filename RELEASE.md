RELEASE_TYPE: minor

Failing-test reports now flow through the engine's document printer, in
line with the other Hegel frontends: drawn values, notes, and
`ht.Log`/`ht.Error`/`ht.Fatal` messages collect in an engine-hosted
document, rendered together when the test case completes.

Draw lines now read `file:line: name = value`, where `name` is the
binding receiving the draw (`xs := hegel.Draw(...)` reports as `xs = …`;
draws without a single binding name are numbered `draw_1`, `draw_2`, …),
with the file:line written by hegel-go rather than `testing.T`'s log
decoration. Values print compositionally: lists and maps print element by
element through their element generators, `Optional` prints `nil` or
`&value` (previously the pointer's address), map entries print in the
order they were drawn (previously `%#v`'s sorted-key order), and a note
made inside a composite's body lands after the draw's line instead of
mid-line.

The engine's explain phase is on by default (disable with
`hegel.WithPhases` minus `hegel.PhaseExplain`): after shrinking, parts of
the minimal counterexample whose value is irrelevant to the failure are
annotated on the report's draw lines as `// or any other generated value`.
Compositional printing gives these annotations element-level granularity —
a single irrelevant list element is annotated on its own line. When
several draws vary freely, a leading note reports whether varying them
together still always failed.

This release also fixes a rare crash: a wrapper's GC cleanup could free
its native libhegel handle while a call on that same handle was still
executing. Every native call now pins its handle's wrapper with
`runtime.KeepAlive`, and a test enforces the rule for all future
bindings.

Requires the next libhegel release (the `hegel_printer_*` document API,
`hegel_note`, `hegel_failure_comment*`, and
`hegel_test_case_choice_count`).
