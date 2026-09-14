package hegel

// formats_test.go tests email, url, domain, date, datetime, and timezone generators.

import (
	"slices"
	"strings"
	"testing"
	"time"
	"unicode"

	"hegel.dev/go/hegel/internal/libhegel"
)

// =============================================================================
// Domain MaxLength validation (draw returns an error out of [4, 255])
// =============================================================================

// TestDomainsMaxLengthTooSmall covers the lower-bound rejection (a max_length
// too small to fit any TLD). The engine validates it during generator
// construction, so this drives the real build path.
func TestDomainsMaxLengthTooSmall(t *testing.T) {
	t.Parallel()
	_, err := Domains().MaxLength(3).build(libhegel.NewContext())
	assertErrorContains(t, "3", err)
}

// TestDomainsMaxLengthTooLarge covers the upper-bound rejection (> 255).
func TestDomainsMaxLengthTooLarge(t *testing.T) {
	t.Parallel()
	_, err := Domains().MaxLength(300).build(libhegel.NewContext())
	assertErrorContains(t, "300", err)
}

// =============================================================================
// E2E integration tests: property tests with the real hegel binary
// =============================================================================

// TestEmailsE2E verifies that generated emails contain "@".
func TestEmailsE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw(ht, Emails())
		if !strings.Contains(v, "@") {
			panic("email does not contain '@': " + v)
		}
	}, WithTestCases(30))
}

// TestURLsE2E verifies that generated URLs start with "http://" or "https://".
func TestURLsE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw(ht, URLs())
		if !strings.HasPrefix(v, "http://") && !strings.HasPrefix(v, "https://") {
			panic("url does not start with http:// or https://: " + v)
		}
	}, WithTestCases(30))
}

// isValidDomainChar returns true if r is a valid character in a domain label.
func isValidDomainChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '.'
}

// TestDomainsE2E verifies that generated domains contain only valid domain characters.
func TestDomainsE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw(ht, Domains())
		for _, r := range v {
			if !isValidDomainChar(r) {
				panic("domain contains invalid character '" + string(r) + "': " + v)
			}
		}
	}, WithTestCases(30))
}

// TestDomainsMaxLengthE2E verifies that generated domains respect the max_length constraint.
func TestDomainsMaxLengthE2E(t *testing.T) {
	t.Parallel()

	const maxLen = 20
	Test(t, func(ht *T) {
		v := Draw(ht, Domains().MaxLength(maxLen))
		if len(v) > maxLen {
			panic("domain exceeds max_length constraint: " + v)
		}
	}, WithTestCases(30))
}

// TestDatesE2E verifies that generated dates fall within the full Gregorian
// range. Note: the minimum date 0001-01-01T00:00:00Z equals Go's time.Time
// zero value, so IsZero() is not a validity check here — the year bound is.
func TestDatesE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw(ht, Dates())
		if v.Year() < 1 || v.Year() > 9999 {
			panic("date year out of range")
		}
		if v.Location() != time.UTC {
			panic("date is not in UTC")
		}
	}, WithTestCases(30))
}

// TestDatetimesE2E verifies that generated datetimes fall within the full
// Gregorian range with valid time components. As with dates, the minimum
// datetime equals time.Time's zero value, so IsZero() is not used.
func TestDatetimesE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw(ht, Datetimes())
		if v.Year() < 1 || v.Year() > 9999 {
			panic("datetime year out of range")
		}
		if v.Hour() > 23 || v.Minute() > 59 || v.Second() > 59 {
			panic("datetime time component out of range")
		}
		if v.Location() != time.UTC {
			panic("datetime is not in UTC")
		}
	}, WithTestCases(30))
}

// TestTimezonesE2E verifies that generated locations come from the candidate
// list and that the generator does not only produce UTC.
func TestTimezonesE2E(t *testing.T) {
	t.Parallel()

	sawNonUTC := false
	Test(t, func(ht *T) {
		loc := Draw(ht, Timezones())
		if !slices.Contains(timezoneNames, loc.String()) {
			panic("unexpected timezone: " + loc.String())
		}
		if loc != time.UTC {
			sawNonUTC = true
		}
	}, WithTestCases(50))
	if !sawNonUTC {
		t.Error("expected at least one non-UTC location in 50 test cases")
	}
}

// TestDatetimesTimezonesE2E verifies zone-aware datetimes carry a drawn
// location while keeping their wall-clock reading in range.
func TestDatetimesTimezonesE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw(ht, Datetimes().Timezones(Timezones()))
		if v.Year() < 1 || v.Year() > 9999 {
			panic("datetime year out of range")
		}
		if !slices.Contains(timezoneNames, v.Location().String()) {
			panic("unexpected timezone: " + v.Location().String())
		}
	}, WithTestCases(30))
}

// TestDatetimesPinnedTimezoneE2E pins the location with Just and checks every
// value is read in it.
func TestDatetimesPinnedTimezoneE2E(t *testing.T) {
	t.Parallel()

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	Test(t, func(ht *T) {
		v := Draw(ht, Datetimes().Timezones(Just(ny)))
		if v.Location() != ny {
			panic("datetime is not in America/New_York: " + v.String())
		}
	}, WithTestCases(30))
}
