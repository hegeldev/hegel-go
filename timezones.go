package hegel

import (
	"fmt"
	"sync"
	"time"

	// Timezones draws from a fixed list of IANA names. Embedding the tz
	// database guarantees every one of them resolves, even on hosts such as
	// Windows or minimal containers that ship no zoneinfo directory.
	// time.LoadLocation still prefers the system database when there is one.
	_ "time/tzdata"
)

//go:generate go run scripts/gen-timezones.go

// timezoneCache memoizes [time.LoadLocation] results, which otherwise parse
// the zone file on every draw.
var timezoneCache sync.Map // string -> *time.Location

// loadTimezone returns the location for an IANA zone name, caching successful
// lookups.
func loadTimezone(name string) (*time.Location, error) {
	if loc, ok := timezoneCache.Load(name); ok {
		return loc.(*time.Location), nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", name, err)
	}
	timezoneCache.Store(name, loc)
	return loc, nil
}

// Timezones returns a Generator that produces *time.Location values for the
// zones of the IANA time zone database, such as "Europe/London" or
// "America/New_York". UTC is the simplest value, so failing examples shrink
// toward it.
//
// The candidate names are a fixed list taken from the tz database bundled
// with Go (see timezones_data.go), so the same choice reproduces the same
// zone on every host, and the database is embedded via [time/tzdata] so every
// name resolves. Pair with [DatetimeGenerator.Timezones] to generate
// zone-aware datetimes.
//
// To generate fixed offsets instead, map an integer generator through
// [time.FixedZone]:
//
//	hegel.Map(hegel.Integers(-12*60*60, 14*60*60), func(secs int) *time.Location {
//		return time.FixedZone("", secs)
//	})
func Timezones() Generator[*time.Location] {
	return genFunc[*time.Location](func(tc TestCase) (*time.Location, error) {
		ctx, ltc := tc.engine()
		idx, err := ltc.GenerateInteger(ctx, 0, int64(len(timezoneNames)-1))
		if err != nil {
			return nil, err
		}
		return loadTimezone(timezoneNames[idx])
	})
}
