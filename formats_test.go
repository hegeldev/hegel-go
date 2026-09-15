package hegel

// formats_test.go tests email, url, domain, date, and datetime generators.

import (
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

func TestDatesInclusiveBounds(t *testing.T) {
	t.Parallel()

	minVal := time.Date(1999, time.December, 31, 12, 30, 0, 0, time.FixedZone("ignored", 3600))
	maxVal := time.Date(2000, time.January, 2, 1, 2, 3, 0, time.UTC)
	Test(t, func(ht *T) {
		v := Draw(ht, Dates().Min(minVal).Max(maxVal))
		if v.Before(time.Date(1999, time.December, 31, 0, 0, 0, 0, time.UTC)) ||
			v.After(time.Date(2000, time.January, 2, 0, 0, 0, 0, time.UTC)) {
			panic("date outside configured bounds")
		}
	}, WithTestCases(30))
}

func TestDatesEqualBounds(t *testing.T) {
	t.Parallel()

	bound := time.Date(2024, time.February, 29, 18, 30, 0, 0, time.UTC)
	Test(t, func(ht *T) {
		want := time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC)
		if got := Draw(ht, Dates().Min(bound).Max(bound)); !got.Equal(want) {
			panic("date with equal bounds did not produce the bound")
		}
	}, WithTestCases(1))
}

func TestDatesOneSidedBounds(t *testing.T) {
	t.Parallel()

	minVal := time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC)
	maxVal := time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	Test(t, func(ht *T) {
		if got := Draw(ht, Dates().Min(minVal)); !got.Equal(minVal) {
			panic("date did not honor one-sided minimum")
		}
		if got := Draw(ht, Dates().Max(maxVal)); !got.Equal(maxVal) {
			panic("date did not honor one-sided maximum")
		}
	}, WithTestCases(1))
}

func TestDatesSupportExtendedYears(t *testing.T) {
	t.Parallel()

	bound := time.Date(-10000, time.January, 1, 0, 0, 0, 0, time.UTC)
	Test(t, func(ht *T) {
		if got := Draw(ht, Dates().Min(bound).Max(bound)); !got.Equal(bound) {
			panic("date did not preserve an engine-supported extended year")
		}
	}, WithTestCases(1))
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

func TestDatetimesInclusiveBounds(t *testing.T) {
	t.Parallel()

	minVal := time.Date(1999, time.December, 31, 23, 59, 59, 123, time.FixedZone("ignored", 3600))
	maxVal := time.Date(2000, time.January, 1, 0, 0, 1, 456, time.UTC)
	wantMin := time.Date(1999, time.December, 31, 23, 59, 59, 123, time.UTC)
	Test(t, func(ht *T) {
		v := Draw(ht, Datetimes().Min(minVal).Max(maxVal))
		if v.Before(wantMin) || v.After(maxVal) {
			panic("datetime outside configured bounds")
		}
	}, WithTestCases(30))
}

func TestDatetimesEqualBounds(t *testing.T) {
	t.Parallel()

	bound := time.Date(2024, time.February, 29, 12, 34, 56, 789, time.UTC)
	Test(t, func(ht *T) {
		if got := Draw(ht, Datetimes().Min(bound).Max(bound)); !got.Equal(bound) {
			panic("datetime with equal bounds did not produce the bound")
		}
	}, WithTestCases(1))
}

func TestDatetimesOneSidedBounds(t *testing.T) {
	t.Parallel()

	minVal := time.Date(9999, time.December, 31, 23, 59, 59, 999999999, time.UTC)
	maxVal := time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	Test(t, func(ht *T) {
		if got := Draw(ht, Datetimes().Min(minVal)); !got.Equal(minVal) {
			panic("datetime did not honor one-sided minimum")
		}
		if got := Draw(ht, Datetimes().Max(maxVal)); !got.Equal(maxVal) {
			panic("datetime did not honor one-sided maximum")
		}
	}, WithTestCases(1))
}

func TestDatetimesSupportExtendedYears(t *testing.T) {
	t.Parallel()

	bound := time.Date(10000, time.December, 31, 23, 59, 59, 999999999, time.UTC)
	Test(t, func(ht *T) {
		if got := Draw(ht, Datetimes().Min(bound).Max(bound)); !got.Equal(bound) {
			panic("datetime did not preserve an engine-supported extended year")
		}
	}, WithTestCases(1))
}
