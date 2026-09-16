RELEASE_TYPE: patch

Property runs now report their source location to Antithesis and can include
the counterexample's reproduction blob in failure output. Use
`WithReproductionBlob(false)` to suppress that line when the active profile
would otherwise enable it.
