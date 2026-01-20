package hegel

// EmailGenerator generates email addresses.
type EmailGenerator struct{}

// Emails returns a generator for email addresses.
func Emails() *EmailGenerator {
	return &EmailGenerator{}
}

// Generate produces an email address.
func (g *EmailGenerator) Generate() string {
	return generateFromSchema[string](g.Schema())
}

// Schema returns the JSON schema for email addresses.
func (g *EmailGenerator) Schema() map[string]any {
	return map[string]any{
		"type": "email",
	}
}

// URLGenerator generates URLs.
type URLGenerator struct{}

// URLs returns a generator for URLs.
func URLs() *URLGenerator {
	return &URLGenerator{}
}

// Generate produces a URL.
func (g *URLGenerator) Generate() string {
	return generateFromSchema[string](g.Schema())
}

// Schema returns the JSON schema for URLs.
func (g *URLGenerator) Schema() map[string]any {
	return map[string]any{
		"type": "url",
	}
}

// DomainGenerator generates domain names.
type DomainGenerator struct {
	maxLength int
}

// Domains returns a generator for domain names.
// Default maximum length is 255 characters.
func Domains() *DomainGenerator {
	return &DomainGenerator{maxLength: 255}
}

// MaxLength sets the maximum domain length.
func (g *DomainGenerator) MaxLength(n int) *DomainGenerator {
	g.maxLength = n
	return g
}

// Generate produces a domain name.
func (g *DomainGenerator) Generate() string {
	return generateFromSchema[string](g.Schema())
}

// Schema returns the JSON schema for domain names.
func (g *DomainGenerator) Schema() map[string]any {
	return map[string]any{
		"type":       "domain",
		"max_length": g.maxLength,
	}
}

// IPVersion specifies which IP address version to generate.
type IPVersion int

const (
	// IPVersionAny generates either IPv4 or IPv6 addresses.
	IPVersionAny IPVersion = iota
	// IPVersionV4 generates only IPv4 addresses.
	IPVersionV4
	// IPVersionV6 generates only IPv6 addresses.
	IPVersionV6
)

// IPAddressGenerator generates IP addresses.
type IPAddressGenerator struct {
	version IPVersion
}

// IPAddresses returns a generator for IP addresses.
// By default, generates either IPv4 or IPv6.
func IPAddresses() *IPAddressGenerator {
	return &IPAddressGenerator{version: IPVersionAny}
}

// V4 restricts to IPv4 addresses only.
func (g *IPAddressGenerator) V4() *IPAddressGenerator {
	g.version = IPVersionV4
	return g
}

// V6 restricts to IPv6 addresses only.
func (g *IPAddressGenerator) V6() *IPAddressGenerator {
	g.version = IPVersionV6
	return g
}

// Generate produces an IP address.
func (g *IPAddressGenerator) Generate() string {
	return generateFromSchema[string](g.Schema())
}

// Schema returns the JSON schema for IP addresses.
func (g *IPAddressGenerator) Schema() map[string]any {
	switch g.version {
	case IPVersionV4:
		return map[string]any{
			"type": "ipv4",
		}
	case IPVersionV6:
		return map[string]any{
			"type": "ipv6",
		}
	default:
		return map[string]any{
			"one_of": []map[string]any{
				{"type": "ipv4"},
				{"type": "ipv6"},
			},
		}
	}
}

// DateGenerator generates ISO 8601 dates (YYYY-MM-DD).
type DateGenerator struct{}

// Dates returns a generator for ISO 8601 dates.
func Dates() *DateGenerator {
	return &DateGenerator{}
}

// Generate produces a date string.
func (g *DateGenerator) Generate() string {
	return generateFromSchema[string](g.Schema())
}

// Schema returns the JSON schema for dates.
func (g *DateGenerator) Schema() map[string]any {
	return map[string]any{
		"type": "date",
	}
}

// TimeGenerator generates ISO 8601 times (HH:MM:SS).
type TimeGenerator struct{}

// Times returns a generator for ISO 8601 times.
func Times() *TimeGenerator {
	return &TimeGenerator{}
}

// Generate produces a time string.
func (g *TimeGenerator) Generate() string {
	return generateFromSchema[string](g.Schema())
}

// Schema returns the JSON schema for times.
func (g *TimeGenerator) Schema() map[string]any {
	return map[string]any{
		"type": "time",
	}
}

// DateTimeGenerator generates ISO 8601 datetimes.
type DateTimeGenerator struct{}

// DateTimes returns a generator for ISO 8601 datetimes.
func DateTimes() *DateTimeGenerator {
	return &DateTimeGenerator{}
}

// Generate produces a datetime string.
func (g *DateTimeGenerator) Generate() string {
	return generateFromSchema[string](g.Schema())
}

// Schema returns the JSON schema for datetimes.
func (g *DateTimeGenerator) Schema() map[string]any {
	return map[string]any{
		"type": "datetime",
	}
}
