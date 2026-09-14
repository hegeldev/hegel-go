RELEASE_TYPE: patch

This patch adds support for generating datetimes in time zones other than UTC ([#101](https://github.com/hegeldev/hegel-go/issues/101)).

The new `Timezones` generator produces `*time.Location` values for the zones of the IANA time zone database, shrinking toward UTC. `Datetimes` now returns a `DatetimeGenerator` builder whose `Timezones` method pairs each generated wall-clock datetime with a location drawn from the given generator:

```go
// A datetime in a random IANA zone.
hegel.Draw(ht, hegel.Datetimes().Timezones(hegel.Timezones()))

// A datetime in New York.
ny, _ := time.LoadLocation("America/New_York")
hegel.Draw(ht, hegel.Datetimes().Timezones(hegel.Just(ny)))
```

`Datetimes()` without a timezone generator still produces UTC values from the same choice sequence as before, so existing tests and saved examples are unaffected. Hegel now imports `time/tzdata`, so every zone name resolves even on hosts without a system time zone database, and draw reports print a `*time.Location` as `time.Location("Europe/London")` rather than dumping its transition table.
