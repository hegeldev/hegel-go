package hegel

import (
	"fmt"
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
		if err != nil {
			panic(fmt.Sprintf("hegel: unreachable: OneOf generateFromSchema: %v", err))
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
	if len(generators) < 2 {
		panic("hegel: OneOf requires at least 2 generators")
	}

	// Check if all generators are basic.
	allBasic := true
	for _, g := range generators {
		if _, ok := g.(*basicGenerator[T]); !ok {
			allBasic = false
			break
		}
	}

	if !allBasic {
		gens := make([]Generator[T], len(generators))
		copy(gens, generators)
		return &compositeOneOfGenerator[T]{generators: gens}
	}

	basics := make([]*basicGenerator[T], len(generators))
	for i, g := range generators {
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
		if err != nil {
			panic(fmt.Sprintf("hegel: unreachable: Optional generateFromSchema: %v", err))
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

// IPAddressVersion specifies which IP version to generate.
type IPAddressVersion int

const (
	// IPVersion4 generates IPv4 addresses.
	IPVersion4 IPAddressVersion = 4
	// IPVersion6 generates IPv6 addresses.
	IPVersion6 IPAddressVersion = 6
)

// IPAddressOptions holds options for the IPAddresses generator.
type IPAddressOptions struct {
	// Version selects the IP version (4 or 6). Zero means generate both.
	Version IPAddressVersion
}

// IPAddresses returns a Generator that produces IP address strings.
func IPAddresses(opts IPAddressOptions) Generator[string] {
	switch opts.Version {
	case IPVersion4:
		return &basicGenerator[string]{schema: map[string]any{"type": "ipv4"}}
	case IPVersion6:
		return &basicGenerator[string]{schema: map[string]any{"type": "ipv6"}}
	default:
		return OneOf[string](
			&basicGenerator[string]{schema: map[string]any{"type": "ipv4"}},
			&basicGenerator[string]{schema: map[string]any{"type": "ipv6"}},
		)
	}
}
