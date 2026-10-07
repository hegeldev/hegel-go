RELEASE_TYPE: patch

Add `Default[T]()` to construct generators for supported primitive and composite Go types, including named types. Recursive types and unsupported fields are rejected when the generator is constructed.
