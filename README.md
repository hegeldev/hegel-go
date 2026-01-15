# Hegel Go SDK

A Go SDK for Hegel property-based testing. This SDK allows Go test binaries to communicate with the Hegel server to generate random test data according to JSON schemas.

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

import hegel "github.com/antithesishq/hegel-go"

func main() {
    // Generate a random integer between 0 and 100
    n := hegel.Integers[int]().Min(0).Max(100).Generate()

    // Generate a slice of emails
    emails := hegel.Slices(hegel.Emails()).MinSize(1).MaxSize(5).Generate()

    // Generate a struct using reflection
    type Person struct {
        Name string `json:"name"`
        Age  int    `json:"age"`
    }

    person := hegel.Make[Person]().
        With("Age", hegel.Integers[int]().Min(18).Max(65)).
        Generate()
}
```

## Environment Variables

- `HEGEL_SOCKET`: Path to the Unix socket for generation requests (required)
- `HEGEL_REJECT_CODE`: Exit code to signal test case rejection (required)
- `HEGEL_DEBUG`: If set, prints request/response JSON to stderr

## API Reference

### Primitive Generators

```go
hegel.Nulls()           // Generates null (struct{})
hegel.Booleans()        // Generates true/false
hegel.Just(value)       // Always returns the same value
```

### Numeric Generators

```go
// Integers with fluent configuration
hegel.Integers[int]()                    // Full int range
hegel.Integers[int8]()                   // -128 to 127
hegel.Integers[uint8]()                  // 0 to 255
hegel.Integers[int]().Min(0).Max(100)    // Constrained range

// Floats with fluent configuration
hegel.Floats[float64]()                       // Any float
hegel.Floats[float64]().Min(0.0).Max(1.0)     // Constrained
hegel.Floats[float64]().ExcludeMin().ExcludeMax()  // Exclusive bounds
```

### String Generators

```go
hegel.Text()                           // Any string
hegel.Text().MinSize(1).MaxSize(100)   // Constrained length
hegel.FromRegex("[a-z]{3}-[0-9]{3}")   // Matches pattern
```

### Format String Generators

```go
hegel.Emails()                    // Email addresses
hegel.URLs()                      // URLs
hegel.Domains()                   // Domain names
hegel.Domains().MaxLength(50)     // Constrained domain length
hegel.IPAddresses()               // IPv4 or IPv6
hegel.IPAddresses().V4()          // IPv4 only
hegel.IPAddresses().V6()          // IPv6 only
hegel.Dates()                     // ISO 8601 dates (YYYY-MM-DD)
hegel.Times()                     // ISO 8601 times (HH:MM:SS)
hegel.DateTimes()                 // ISO 8601 datetimes
```

### Collection Generators

```go
// Slices
hegel.Slices(hegel.Integers[int]())                           // []int
hegel.Slices(hegel.Text()).MinSize(1).MaxSize(10)             // Constrained size
hegel.Slices(hegel.Integers[int]()).Unique()                  // Unique elements

// Maps (keys are always strings)
hegel.Maps(hegel.Integers[int]())                             // map[string]int
hegel.Maps(hegel.Text()).MinSize(1).MaxSize(5)                // Constrained size
```

### Combinators

```go
// Sample from fixed collection
hegel.SampledFrom([]string{"apple", "banana", "cherry"})

// Choose from multiple generators (same type)
hegel.OneOf(
    hegel.Integers[int]().Min(0).Max(10),
    hegel.Integers[int]().Min(100).Max(200),
)

// Choose from generators of different types
hegel.OneOfAny(
    hegel.AsAny(hegel.Text()),
    hegel.AsAny(hegel.Integers[int]()),
    hegel.AsAny(hegel.Booleans()),
)

// Optional values (nil or value)
hegel.Optional(hegel.Integers[int]())  // *int (nil or pointer to int)

// Filter values
hegel.Filter(
    hegel.Integers[int]().Min(0).Max(100),
    func(n int) bool { return n%2 == 0 },  // Even numbers only
    10,  // Max attempts
)
```

### Custom Generators

```go
// Simple custom generator
even := hegel.Custom(func() int {
    return hegel.Integers[int]().Min(0).Max(50).Generate() * 2
})

// Custom generator with schema (enables composition)
positiveInt := hegel.CustomWithSchema(
    func() int { return hegel.Integers[int]().Min(1).Generate() },
    map[string]any{"type": "integer", "minimum": 1},
)
```

### Struct Generation

```go
type Person struct {
    Name  string `json:"name"`
    Age   int    `json:"age"`
    Email string `json:"email"`
}

// Generate with default generators for each field type
person := hegel.Make[Person]().Generate()

// Customize specific fields
person := hegel.Make[Person]().
    With("Age", hegel.Integers[int]().Min(18).Max(65)).
    With("Email", hegel.Emails()).
    Generate()
```

## Schema Composition

When generators have JSON schemas, they can be composed for efficient single-socket round-trips. For example:

```go
// This generates the entire structure in one request to Hegel
people := hegel.Slices(
    hegel.Make[Person]().
        With("Age", hegel.Integers[int]().Min(0).Max(120)),
).MinSize(1).MaxSize(10).Generate()
```

When schemas are unavailable (e.g., after using `Custom()` without a schema), the SDK falls back to compositional generation using multiple requests with proper span labeling.

## Assumptions

When generated data doesn't meet preconditions that can't be expressed in the schema, use `Assume()`:

```go
func testProperty() {
    data := hegel.Make[Input]().Generate()

    hegel.Assume(isValidPrecondition(data))

    // Test logic here
}
```

## Exit Codes

- `0`: Test passed
- `HEGEL_REJECT_CODE`: Test case rejected (try different input)
- `1`: Test assertion failed
- `134`: Socket communication error
