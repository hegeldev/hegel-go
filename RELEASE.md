RELEASE_TYPE: patch

This release adds `hegel.Composite`, for defining custom generators:

```go
type Person struct {
    Name           string
    Age            int
    DrivingLicense bool
}

personGen := hegel.Composite(func(tc *hegel.TestCase) Person {
    age := hegel.Draw(tc, hegel.Integers(0, 120))
    name := hegel.Draw(tc, hegel.Text())
    p := Person{Age: age, Name: name}
    if age >= 18 {
        p.DrivingLicense = hegel.Draw(tc, hegel.Booleans())
    }
    return p
})

hegel.Test(t, func(ht *hegel.T) {
    p := hegel.Draw(ht, personGen)
    // ...
})
```
