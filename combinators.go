package hegel

import (
	"net/netip"

	"hegel.dev/go/hegel/internal/libhegel"
)

// --- OneOf generator ---

// oneOfGenerator generates a value from one of the given generators.
type oneOfGenerator[T any] struct {
	generators []Generator[T]
}

// asBasic returns a basic generator with a one_of schema when every branch
// is basic. Returns (nil, false, nil) when any branch is not.
//
// The wire shape for one_of responses is [index, value]: the server tells us
// which branch produced the value, so we dispatch to the matching parse fn
// without baking a tag into the schema.
func (g *oneOfGenerator[T]) asBasic() (*basicGenerator[T], bool, error) {
	basics := make([]*basicGenerator[T], 0, len(g.generators))
	for _, branch := range g.generators {
		bg, ok, err := branch.asBasic()
		if err != nil {
			return nil, false, err
		}
		if !ok {
			return nil, false, nil
		}
		basics = append(basics, bg)
	}

	schemas := make([]any, len(basics))
	parseFns := make([]func(any) T, len(basics))
	for i, bg := range basics {
		schemas[i] = bg.schema
		parseFns[i] = bg.parse
	}

	return &basicGenerator[T]{
		schema: map[string]any{"type": "one_of", "generators": schemas},
		parse: func(raw any) T {
			elems := raw.([]any)
			idx := extractInt(elems[0])
			return parseFns[idx](elems[1])
		},
	}, true, nil
}

// draw produces a value by dispatching to the basic schema when possible,
// falling back to a server-side index draw otherwise.
func (g *oneOfGenerator[T]) draw(tc TestCase) (T, error) {
	var zero T
	bg, ok, err := g.asBasic()
	if err != nil {
		return zero, err
	}
	if ok {
		return bg.draw(tc)
	}
	return withSpan(tc, libhegel.LABEL_ONE_OF, func() (T, error) {
		n := len(g.generators)
		idx, err := tc.generate(map[string]any{
			"type":      "integer",
			"min_value": int64(0),
			"max_value": int64(n - 1),
		})
		if err != nil { // coverage-ignore
			return zero, err
		}
		return g.generators[extractInt(idx)].draw(tc)
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

// asBasic returns a basic generator with a one_of schema (a null branch and
// an inner-value branch) when inner is basic. Returns (nil, false, nil) when
// inner is not.
//
// The wire shape for one_of responses is [index, value]: branch 0 is null,
// branch 1 is the inner value, so we dispatch on the server-supplied index
// without baking a tag into the schema.
func (g *optionalGenerator[T]) asBasic() (*basicGenerator[*T], bool, error) {
	innerBasic, ok, err := g.inner.asBasic()
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}

	innerParse := innerBasic.parse
	schema := map[string]any{
		"type": "one_of",
		"generators": []any{
			map[string]any{"type": "constant", "value": nil},
			innerBasic.schema,
		},
	}
	return &basicGenerator[*T]{
		schema: schema,
		parse: func(raw any) *T {
			elems := raw.([]any)
			idx := extractInt(elems[0])
			if idx == 0 {
				return nil
			}
			v := innerParse(elems[1])
			return &v
		},
	}, true, nil
}

// draw generates either nil or a value by dispatching to the basic schema
// when inner is basic, falling back to a server-side index draw otherwise.
func (g *optionalGenerator[T]) draw(tc TestCase) (*T, error) {
	bg, ok, err := g.asBasic()
	if err != nil {
		return nil, err
	}
	if ok {
		return bg.draw(tc)
	}
	return withSpan(tc, libhegel.LABEL_ONE_OF, func() (*T, error) {
		idx, err := tc.generate(map[string]any{
			"type":      "integer",
			"min_value": int64(0),
			"max_value": int64(1),
		})
		if err != nil { // coverage-ignore
			return nil, err
		}
		if extractInt(idx) == 0 {
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
	version int64
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

// asBasic always returns a basic generator: when version is set it produces a
// single-type schema; otherwise it delegates to OneOf of the two basic
// branches, which itself yields a basic.
func (g IPAddressGenerator) asBasic() (*basicGenerator[netip.Addr], bool, error) {
	addrTransform := func(a any) netip.Addr {
		return netip.MustParseAddr(a.(string))
	}
	if g.version != 0 {
		return &basicGenerator[netip.Addr]{
			schema: map[string]any{"type": "ip_address", "version": g.version},
			parse:  addrTransform,
		}, true, nil
	}
	return OneOf(
		&basicGenerator[netip.Addr]{
			schema: map[string]any{"type": "ip_address", "version": int64(4)},
			parse:  addrTransform,
		},
		&basicGenerator[netip.Addr]{
			schema: map[string]any{"type": "ip_address", "version": int64(6)},
			parse:  addrTransform,
		},
	).asBasic()
}

// draw produces an IP address by dispatching to the basic schema returned by asBasic.
func (g IPAddressGenerator) draw(tc TestCase) (netip.Addr, error) {
	bg, _, err := g.asBasic()
	if err != nil { // coverage-ignore
		return netip.Addr{}, err
	}
	return bg.draw(tc)
}
