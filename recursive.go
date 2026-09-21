package hegel

import (
	"errors"
	"fmt"
	"hash/maphash"

	"hegel.dev/go/hegel/internal/libhegel"
)

const (
	defaultRecursiveMaxDepth  = 32
	defaultRecursiveMaxLeaves = 100
)

// RecursiveGenerator generates recursively defined values within configurable
// depth and leaf limits. Invalid limits panic when the generator is drawn.
type RecursiveGenerator[T any] struct {
	leaf      Generator[T]
	branch    func(Generator[T]) Generator[T]
	maxDepth  int
	maxLeaves int
}

func (g RecursiveGenerator[T]) hashFields(h *maphash.Hash) bool {
	return hashGenerator(h, g.leaf) &&
		hashFunction(h, g.branch) &&
		hashValues(h, g.maxDepth, g.maxLeaves)
}

// Recursive returns a generator for recursively defined values.
//
// The leaf generator produces terminal values. The branch function receives a
// generator for child values and returns a generator that combines its children.
// The default maximum depth is 32 and the default leaf budget is 100.
func Recursive[T any](leaf Generator[T], branch func(Generator[T]) Generator[T]) RecursiveGenerator[T] {
	return RecursiveGenerator[T]{
		leaf:      leaf,
		branch:    branch,
		maxDepth:  defaultRecursiveMaxDepth,
		maxLeaves: defaultRecursiveMaxLeaves,
	}
}

// MaxDepth sets the maximum number of nested branches. Zero generates only
// leaves. The default is 32.
func (g RecursiveGenerator[T]) MaxDepth(n int) RecursiveGenerator[T] {
	g.maxDepth = n
	return g
}

// MaxLeaves sets the maximum number of leaves in one generated value. The
// default is 100.
func (g RecursiveGenerator[T]) MaxLeaves(n int) RecursiveGenerator[T] {
	g.maxLeaves = n
	return g
}

// leafBudgetRetry requires Retry to reset the recursion scope after unwinding.
type leafBudgetRetry struct{ error }

// finishRetry indicates that Finish already reset the recursion scope.
type finishRetry struct{ error }

func (g RecursiveGenerator[T]) draw(tc TestCase) (T, error) {
	var zero T
	if g.maxDepth < 0 {
		return zero, fmt.Errorf("max_depth=%d must be non-negative", g.maxDepth)
	}
	if g.maxLeaves < 0 {
		return zero, fmt.Errorf("max_leaves=%d must be non-negative", g.maxLeaves)
	}

	ctx, nativeTC := tc.engine()
	recursion, err := nativeTC.NewRecursion(ctx, uint64(g.maxDepth), uint64(g.maxLeaves))
	if err != nil {
		return zero, err
	}

	root := &subtreeGenerator[T]{
		leaf:      g.leaf,
		branch:    g.branch,
		recursion: recursion,
	}
	for {
		var value T
		err := tc.invoke(func(attempt TestCase) {
			var drawErr error
			value, drawErr = draw(attempt, root)
			if drawErr != nil {
				attempt.abort(drawErr)
			}
		})
		if err == nil {
			return value, nil
		}

		// Both retry paths discard the attempt's native spans. The scoped
		// TestCase passed to invoke keeps their Go-side depth from leaking too.
		if _, ok := errors.AsType[*leafBudgetRetry](err); ok {
			if err := recursion.Retry(ctx, nativeTC); err != nil {
				return zero, err
			}
			continue
		}
		if _, ok := errors.AsType[*finishRetry](err); ok {
			continue
		}
		return zero, err
	}
}

// subtreeGenerator copies track depth independently and share a recursion scope.
type subtreeGenerator[T any] struct {
	leaf      Generator[T]
	branch    func(Generator[T]) Generator[T]
	recursion *libhegel.Recursion
	depth     uint64
}

func (g *subtreeGenerator[T]) hashFields(h *maphash.Hash) bool {
	return hashGenerator(h, g.leaf) && hashFunction(h, g.branch) && hashComparable(h, g.depth)
}

func (g *subtreeGenerator[T]) draw(tc TestCase) (T, error) {
	var zero T
	if err := tc.startSpan(staticLabel("recursive")); err != nil {
		return zero, err
	}

	ctx, nativeTC := tc.engine()
	isBranch, err := g.recursion.Branch(ctx, nativeTC, g.depth)
	if err != nil {
		return zero, err
	}

	var value T
	if isBranch {
		child := *g
		child.depth++
		value, err = draw(tc, g.branch(&child))
	} else {
		if err := g.recursion.Leaf(ctx, nativeTC); err != nil {
			if errors.Is(err, libhegel.E_RETRY) {
				return zero, &leafBudgetRetry{err}
			}
			return zero, err
		}
		value, err = draw(tc, g.leaf)
	}
	if err != nil {
		return zero, err
	}

	if g.depth == 0 {
		err := g.recursion.Finish(ctx, nativeTC)
		if errors.Is(err, libhegel.E_RETRY) {
			return zero, &finishRetry{err}
		}
		if err != nil {
			return zero, err
		}
	}
	if err := tc.stopSpan(false); err != nil {
		return zero, err
	}
	return value, nil
}
