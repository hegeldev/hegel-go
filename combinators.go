package hegel

import "net/netip"

// --- OneOf generator ---

// oneOfGenerator generates a value from one of the given generators.
type oneOfGenerator[T any] struct {
	generators []Generator[T]
}

func (g *oneOfGenerator[T]) draw(tc TestCase) (T, error) {
	var zero T
	return withSpan(tc, "one_of", func() (T, error) {
		ctx, ltc := tc.engine()
		idx, err := ltc.GenerateInteger(ctx, 0, int64(len(g.generators)-1))
		if err != nil {
			return zero, err
		}
		return g.generators[idx].draw(tc)
	})
}

// OneOf returns a Generator that produces values from one of the given generators.
//
// Requires at least 1 generator.
func OneOf[T any](generators ...Generator[T]) Generator[T] {
	if len(generators) == 0 {
		panic("OneOf requires at least one generator")
	}
	gens := make([]Generator[T], len(generators))
	copy(gens, generators)
	return &oneOfGenerator[T]{generators: gens}
}

// Optional returns a Generator that produces either nil (as *T) or a value from element.
func Optional[T any](element Generator[T]) Generator[*T] {
	return &optionalGenerator[T]{inner: element}
}

// optionalGenerator generates either nil or a value from inner.
type optionalGenerator[T any] struct {
	inner Generator[T]
}

func (g *optionalGenerator[T]) draw(tc TestCase) (*T, error) {
	return withSpan(tc, "optional", func() (*T, error) {
		ctx, ltc := tc.engine()
		idx, err := ltc.GenerateInteger(ctx, 0, 1)
		if err != nil {
			return nil, err
		}
		if idx == 0 {
			return nil, nil
		}
		v, err := g.inner.draw(tc)
		if err != nil {
			return nil, err
		}
		return &v, nil
	})
}

// --- IPAddresses generator ---

// IPAddressGenerator configures and generates IP addresses.
// Use [IPAddresses] to create one, then chain builder methods to configure it.
type IPAddressGenerator struct {
	// version is 0 (unset; both v4 and v6), 4, or 6.
	version int
}

var _ Generator[netip.Addr] = IPAddressGenerator{}

// IPAddresses returns a Generator that produces IP addresses.
func IPAddresses() IPAddressGenerator {
	return IPAddressGenerator{}
}

// IPv4 restricts the generator to IPv4 addresses only.
func (g IPAddressGenerator) IPv4() IPAddressGenerator {
	g.version = 4
	return g
}

// IPv6 restricts the generator to IPv6 addresses only.
func (g IPAddressGenerator) IPv6() IPAddressGenerator {
	g.version = 6
	return g
}

// drawV4 draws an IPv4 address.
func drawV4(tc TestCase) (netip.Addr, error) {
	ctx, ltc := tc.engine()
	b, err := ltc.GenerateIPv4(ctx)
	if err != nil {
		return netip.Addr{}, err
	}
	return netip.AddrFrom4(b), nil
}

// drawV6 draws an IPv6 address.
func drawV6(tc TestCase) (netip.Addr, error) {
	ctx, ltc := tc.engine()
	b, err := ltc.GenerateIPv6(ctx)
	if err != nil {
		return netip.Addr{}, err
	}
	return netip.AddrFrom16(b), nil
}

func (g IPAddressGenerator) draw(tc TestCase) (netip.Addr, error) {
	switch g.version {
	case 4:
		return drawV4(tc)
	case 6:
		return drawV6(tc)
	default:
		return withSpan(tc, "one_of", func() (netip.Addr, error) {
			ctx, ltc := tc.engine()
			idx, err := ltc.GenerateInteger(ctx, 0, 1)
			if err != nil {
				return netip.Addr{}, err
			}
			if idx == 0 {
				return drawV4(tc)
			}
			return drawV6(tc)
		})
	}
}
