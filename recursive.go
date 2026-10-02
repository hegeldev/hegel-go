package hegel

import (
	"errors"
	"fmt"

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

func (g RecursiveGenerator[T]) draw(tc TestCase) (T, error) {
	rootLabel := labelFor(&subtreeGenerator[T]{})
	return runRecursion(tc, g.maxDepth, g.maxLeaves, &rootLabel, func(attempt TestCase, choose recursionChoice) (T, error) {
		root := &subtreeGenerator[T]{leaf: g.leaf, branch: g.branch, choose: choose}
		return root.draw(attempt)
	})
}

// subtreeGenerator copies track depth independently and share a choice function.
type subtreeGenerator[T any] struct {
	leaf   Generator[T]
	branch func(Generator[T]) Generator[T]
	choose recursionChoice
	depth  uint64
}

func (g *subtreeGenerator[T]) draw(tc TestCase) (T, error) {
	drawValue := func() (T, error) {
		var zero T
		isBranch, err := g.choose(g.depth)
		if err != nil {
			return zero, err
		}
		if isBranch {
			child := *g
			child.depth++
			return draw(tc, g.branch(&child))
		}
		return draw(tc, g.leaf)
	}
	if g.depth == 0 {
		return drawValue()
	}
	return withSpan(tc, labelFromName("recursive"), drawValue)
}

// leafBudgetRetry requires Retry after the current attempt unwinds.
type leafBudgetRetry struct{ error }

// finishRetry means Finish has already reset the recursion scope.
type finishRetry struct{ error }

type recursionChoice func(depth uint64) (bool, error)

func runRecursion[T any](
	tc TestCase,
	maxDepth, maxLeaves int,
	outerSpan *libhegel.Label,
	attempt func(TestCase, recursionChoice) (T, error),
) (T, error) {
	var zero T
	if maxDepth < 0 {
		return zero, fmt.Errorf("max_depth=%d must be non-negative", maxDepth)
	}
	if maxLeaves < 0 {
		return zero, fmt.Errorf("max_leaves=%d must be non-negative", maxLeaves)
	}
	ctx, nativeTC := tc.engine()
	recursion, err := nativeTC.NewRecursion(ctx, uint64(maxDepth), uint64(maxLeaves))
	if err != nil {
		return zero, err
	}
	for {
		var value T
		err := tc.invoke(func(scoped TestCase) {
			if outerSpan != nil {
				if err := scoped.startSpan(*outerSpan); err != nil {
					scoped.abort(err)
					return
				}
			}
			if err := scoped.startSpan(labelFromName("recursive")); err != nil {
				scoped.abort(err)
				return
			}
			choose := func(depth uint64) (bool, error) {
				ctx, nativeTC := scoped.engine()
				branch, err := recursion.Branch(ctx, nativeTC, depth)
				if err != nil || branch {
					return branch, err
				}
				if err := recursion.Leaf(ctx, nativeTC); err != nil {
					if errors.Is(err, libhegel.E_RETRY) {
						return false, &leafBudgetRetry{err}
					}
					return false, err
				}
				return false, nil
			}
			var drawErr error
			value, drawErr = attempt(scoped, choose)
			if drawErr != nil {
				scoped.abort(drawErr)
				return
			}
			if err := recursion.Finish(ctx, nativeTC); err != nil {
				if errors.Is(err, libhegel.E_RETRY) {
					scoped.abort(&finishRetry{err})
				} else {
					scoped.abort(err)
				}
				return
			}
			if err := scoped.stopSpan(false); err != nil {
				scoped.abort(err)
				return
			}
			if outerSpan != nil {
				if err := scoped.stopSpan(false); err != nil {
					scoped.abort(err)
				}
			}
		})
		if err == nil {
			return value, nil
		}

		// invoke discards native spans and isolates the Go span depth on error.
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
