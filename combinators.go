package hegel

import (
	"fmt"
	"net/netip"
)

// --- OneOf generator ---

// compositeOneOfGenerator generates a value from one of the given generators
// using the Hegel server to pick the branch.
type compositeOneOfGenerator[T any] struct {
	generators []Generator[T]
}

// draw picks one generator and returns a value from it, wrapped in a ONE_OF span.
func (g *compositeOneOfGenerator[T]) draw(s *TestCase) T {
	var result T
	group(s, labelOneOf, func() {
		n := len(g.generators)
		idx, err := generateFromSchema(s, map[string]any{
			"type":      "integer",
			"min_value": int64(0),
			"max_value": int64(n - 1),
		})
		if err != nil { //nocov
			panic(fmt.Sprintf("hegel: OneOf generateFromSchema: %v", err)) //nocov
		}
		i := extractInt(idx)
		result = g.generators[i].draw(s)
	})
	return result
}

// OneOf returns a Generator that produces values from one of the given generators.
//
// Requires at least 2 generators.
func OneOf[T any](generators ...Generator[T]) Generator[T] {
	if len(generators) == 0 {
		panic("hegel: OneOf requires at least one generator")
	}

	resolved := make([]Generator[T], len(generators))
	for i, g := range generators {
		resolved[i] = unwrapGenerator(g)
	}

	// Check if all generators are basic.
	allBasic := true
	for _, g := range resolved {
		if _, ok := g.(*basicGenerator[T]); !ok {
			allBasic = false
			break
		}
	}

	if !allBasic {
		gens := make([]Generator[T], len(resolved))
		copy(gens, resolved)
		return &compositeOneOfGenerator[T]{generators: gens}
	}

	basics := make([]*basicGenerator[T], len(resolved))
	for i, g := range resolved {
		basics[i] = g.(*basicGenerator[T])
	}

	allIdentity := true
	for _, bg := range basics {
		if bg.transform != nil {
			allIdentity = false
			break
		}
	}

	if allIdentity {
		schemas := make([]any, len(basics))
		for i, bg := range basics {
			schemas[i] = bg.schema
		}
		return &basicGenerator[T]{
			schema: map[string]any{"one_of": schemas},
		}
	}

	// Path 2: tagged tuples
	taggedSchemas := make([]any, len(basics))
	for i, bg := range basics {
		taggedSchemas[i] = map[string]any{
			"type": "tuple",
			"elements": []any{
				map[string]any{"const": int64(i)},
				bg.schema,
			},
		}
	}

	transforms := make([]func(any) T, len(basics))
	for i, bg := range basics {
		transforms[i] = bg.transform
	}

	return &basicGenerator[T]{
		schema: map[string]any{"one_of": taggedSchemas},
		transform: func(tagged any) T {
			elems, _ := tagged.([]any)
			if len(elems) < 2 {
				return tagged.(T)
			}
			tag := extractInt(elems[0])
			value := elems[1]
			if t := transforms[tag]; t != nil {
				return t(value)
			}
			return value.(T)
		},
	}
}

// Optional returns a Generator that produces either nil (as *T) or a value from element.
func Optional[T any](element Generator[T]) Generator[*T] {
	return &optionalGenerator[T]{inner: element}
}

// optionalGenerator generates either nil or a value from inner.
type optionalGenerator[T any] struct {
	inner Generator[T]
}

// draw generates either nil or a value, wrapped in an OPTIONAL/ONE_OF span.
func (g *optionalGenerator[T]) draw(s *TestCase) *T {
	var result *T
	group(s, labelOneOf, func() {
		idx, err := generateFromSchema(s, map[string]any{
			"type":      "integer",
			"min_value": int64(0),
			"max_value": int64(1),
		})
		if err != nil { //nocov
			panic(fmt.Sprintf("hegel: Optional generateFromSchema: %v", err)) //nocov
		}
		i := extractInt(idx)
		if i == 0 {
			result = nil
		} else {
			v := g.inner.draw(s)
			result = &v
		}
	})
	return result
}

// --- IPAddresses generator ---

// IPAddressGenerator configures and generates IP addresses.
// Use [IPAddresses] to create one, then chain builder methods to configure it.
type IPAddressGenerator struct {
	version string
}

// IPAddresses returns a Generator that produces IP addresses.
func IPAddresses() IPAddressGenerator {
	return IPAddressGenerator{}
}

// IPv4 restricts the generator to IPv4 addresses only.
func (g IPAddressGenerator) IPv4() IPAddressGenerator {
	g.version = "ipv4"
	return g
}

// IPv6 restricts the generator to IPv6 addresses only.
func (g IPAddressGenerator) IPv6() IPAddressGenerator {
	g.version = "ipv6"
	return g
}

func (g IPAddressGenerator) buildGenerator() Generator[netip.Addr] {
	addrTransform := func(a any) netip.Addr {
		return netip.MustParseAddr(a.(string))
	}
	if g.version != "" {
		return &basicGenerator[netip.Addr]{
			schema:    map[string]any{"type": g.version},
			transform: addrTransform,
		}
	}
	return OneOf(
		&basicGenerator[netip.Addr]{
			schema:    map[string]any{"type": "ipv4"},
			transform: addrTransform,
		},
		&basicGenerator[netip.Addr]{
			schema:    map[string]any{"type": "ipv6"},
			transform: addrTransform,
		},
	)
}

func (g IPAddressGenerator) draw(s *TestCase) netip.Addr {
	return g.buildGenerator().draw(s)
}
