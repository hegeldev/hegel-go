# Hegel Go SDK

Hegel Go SDK.

## Requirements

- Go 1.22 or later

The SDK automatically installs the Hegel CLI on first use. It will:
1. Download [uv](https://github.com/astral-sh/uv) (if not already on PATH)
2. Create a Python virtual environment in `.hegel/venv`
3. Install the hegel package into that environment

If you already have `hegel` on your PATH, the SDK will use that instead.

## Installation

```bash
go get github.com/antithesishq/hegel-go
```

## Quick Start

```go
package main

import (
    "testing"
    hegel "github.com/antithesishq/hegel-go"
)

func TestAdditionCommutative(t *testing.T) {
    hegel.Hegel(func() {
        x := hegel.Integers[int]().Generate()
        y := hegel.Integers[int]().Generate()
        if x+y != y+x {
            panic("commutativity violated")
        }
    }, hegel.HegelOptions{})
}
```

Run with `go test`.

## Configuration

Use `HegelOptions` for more control:

```go
func TestWithOptions(t *testing.T) {
    hegel.Hegel(func() {
        n := hegel.Integers[int]().Min(0).Max(100).Generate()
        if n < 0 || n > 100 {
            panic("out of bounds")
        }
    }, hegel.HegelOptions{
        TestCases: 500,
        Verbosity: "verbose",
    })
}
```

## API Reference

### Primitive Generators

```go
hegel.Hegel(func() {
    _ = hegel.Nulls().Generate()           // struct{}
    b := hegel.Booleans().Generate()       // bool
    n := hegel.Just(42).Generate()         // constant value
}, hegel.HegelOptions{})
```

### Numeric Generators

```go
hegel.Hegel(func() {
    // Integers with fluent configuration
    i := hegel.Integers[int]().Generate()                    // Full int range
    i8 := hegel.Integers[int8]().Generate()                  // -128 to 127
    u8 := hegel.Integers[uint8]().Generate()                 // 0 to 255
    bounded := hegel.Integers[int]().Min(0).Max(100).Generate()

    // Floats with fluent configuration
    f := hegel.Floats[float64]().Generate()
    bounded_f := hegel.Floats[float64]().Min(0.0).Max(1.0).Generate()
    exclusive := hegel.Floats[float64]().ExcludeMin().ExcludeMax().Generate()
}, hegel.HegelOptions{})
```

### String Generators

```go
hegel.Hegel(func() {
    s := hegel.Text().Generate()
    bounded := hegel.Text().MinSize(1).MaxSize(100).Generate()
    pattern := hegel.FromRegex("[a-z]{3}-[0-9]{3}").Generate()
}, hegel.HegelOptions{})
```

### Format String Generators

```go
hegel.Hegel(func() {
    email := hegel.Emails().Generate()
    url := hegel.URLs().Generate()
    domain := hegel.Domains().Generate()
    bounded_domain := hegel.Domains().MaxLength(50).Generate()
    ip := hegel.IPAddresses().Generate()       // IPv4 or IPv6
    ipv4 := hegel.IPAddresses().V4().Generate()
    ipv6 := hegel.IPAddresses().V6().Generate()
    date := hegel.Dates().Generate()           // YYYY-MM-DD
    time := hegel.Times().Generate()           // HH:MM:SS
    datetime := hegel.DateTimes().Generate()
}, hegel.HegelOptions{})
```

### Collection Generators

```go
hegel.Hegel(func() {
    // Slices
    slice := hegel.Slices(hegel.Integers[int]()).Generate()
    bounded := hegel.Slices(hegel.Text()).MinSize(1).MaxSize(10).Generate()
    unique := hegel.Slices(hegel.Integers[int]()).Unique().Generate()

    // Maps (keys are always strings)
    m := hegel.Maps(hegel.Integers[int]()).Generate()
    bounded_m := hegel.Maps(hegel.Text()).MinSize(1).MaxSize(5).Generate()
}, hegel.HegelOptions{})
```

### Combinators

```go
hegel.Hegel(func() {
    // Sample from fixed collection
    color := hegel.SampledFrom([]string{"apple", "banana", "cherry"}).Generate()

    // Choose from multiple generators (same type)
    n := hegel.OneOf(
        hegel.Integers[int]().Min(0).Max(10),
        hegel.Integers[int]().Min(100).Max(200),
    ).Generate()

    // Choose from generators of different types
    any := hegel.OneOfAny(
        hegel.AsAny(hegel.Text()),
        hegel.AsAny(hegel.Integers[int]()),
        hegel.AsAny(hegel.Booleans()),
    ).Generate()

    // Optional values (nil or value)
    opt := hegel.Optional(hegel.Integers[int]()).Generate()  // *int

    // Filter values
    even := hegel.Filter(
        hegel.Integers[int]().Min(0).Max(100),
        func(n int) bool { return n%2 == 0 },
    ).Generate()
}, hegel.HegelOptions{})
```

### Custom Generators

```go
hegel.Hegel(func() {
    // Simple custom generator
    even := hegel.Custom(func() int {
        return hegel.Integers[int]().Min(0).Max(50).Generate() * 2
    }).Generate()

    // Custom generator with schema (enables composition)
    positiveInt := hegel.CustomWithSchema(
        func() int { return hegel.Integers[int]().Min(1).Generate() },
        map[string]any{"type": "integer", "minimum": 1},
    ).Generate()
}, hegel.HegelOptions{})
```

### Struct Generation

```go
type Person struct {
    Name  string `json:"name"`
    Age   int    `json:"age"`
    Email string `json:"email"`
}

func TestPerson(t *testing.T) {
    hegel.Hegel(func() {
        // Generate with default generators for each field type
        person := hegel.Make[Person]().Generate()

        // Customize specific fields
        person = hegel.Make[Person]().
            With("Age", hegel.Integers[int]().Min(18).Max(65)).
            With("Email", hegel.Emails()).
            Generate()
    }, hegel.HegelOptions{})
}
```

## Schema Composition

When generators have JSON schemas, they can be composed for efficient single-socket round-trips:

```go
hegel.Hegel(func() {
    // This generates the entire structure in one request to Hegel
    people := hegel.Slices(
        hegel.Make[Person]().
            With("Age", hegel.Integers[int]().Min(0).Max(120)),
    ).MinSize(1).MaxSize(10).Generate()
}, hegel.HegelOptions{})
```

When schemas are unavailable (e.g., after using `Custom()` without a schema), the SDK falls back to compositional generation using multiple requests with proper span labeling.

## Assumptions

When generated data doesn't meet preconditions that can't be expressed in the schema, use `Assume()`:

```go
func TestAdults(t *testing.T) {
    hegel.Hegel(func() {
        person := hegel.Make[Person]().Generate()
        hegel.Assume(person.Age >= 18)

        // Test logic for adults only...
    }, hegel.HegelOptions{})
}
```
