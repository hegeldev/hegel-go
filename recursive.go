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

// Recursor identifies a recursion scope during one draw.
//
// A branch callback receives a child Recursor and may pass it to Recurse for
// any result type. A zero or expired Recursor is invalid.
type Recursor struct {
	state   *recursionState
	depth   uint64
	attempt uint64
}

// RecursiveFuncGenerator generates recursive values that may have children of different Go types.
//
// Invalid limits panic when the generator is drawn.
type RecursiveFuncGenerator[T any] struct {
	leaf      Generator[T]
	branch    func(TestCase, Recursor) T
	maxDepth  int
	maxLeaves int
}

// RecursiveFunc returns a generator for recursive values that may have different child types.
//
// Supply a valid leaf generator for the root and each Recurse call. The
// default maximum depth is 32 and the shared leaf budget is 100.
//
// The returned generator supports MaxDepth and MaxLeaves.
func RecursiveFunc[T any](leaf Generator[T], branch func(TestCase, Recursor) T) RecursiveFuncGenerator[T] {
	return RecursiveFuncGenerator[T]{
		leaf:      leaf,
		branch:    branch,
		maxDepth:  defaultRecursiveMaxDepth,
		maxLeaves: defaultRecursiveMaxLeaves,
	}
}

// MaxDepth sets the maximum number of nested branches. Zero generates only
// leaves. The default is 32.
func (g RecursiveFuncGenerator[T]) MaxDepth(n int) RecursiveFuncGenerator[T] {
	g.maxDepth = n
	return g
}

// MaxLeaves sets the shared maximum number of leaves in one generated value.
// The default is 100.
func (g RecursiveFuncGenerator[T]) MaxLeaves(n int) RecursiveFuncGenerator[T] {
	g.maxLeaves = n
	return g
}

// Recurse draws a child node of any type within a branch's recursion scope.
//
// Pass the current TestCase and Recursor from the branch callback, together
// with a valid leaf and a branch function for the child's type. All node types
// spend from the same leaf budget. A zero, expired, or cross-test Recursor
// aborts the test case.
//
// It returns the generated child value, or aborts the test case on error.
func Recurse[T any](tc TestCase, r Recursor, leaf Generator[T], branch func(TestCase, Recursor) T) T {
	var zero T
	if err := r.validate(tc); err != nil {
		tc.abort(err)
		return zero
	}
	value, err := recursionNode(tc, r, leaf, func(tc TestCase, child Recursor) (T, error) {
		return branch(tc, child), nil
	})
	if err != nil {
		tc.abort(err)
	}
	return value
}

// leafBudgetRetry requires Retry to reset the recursion scope after unwinding.
type leafBudgetRetry struct{ error }

// finishRetry indicates that Finish already reset the recursion scope.
type finishRetry struct{ error }

type recursionState struct {
	native  *libhegel.Recursion
	tc      *libhegel.TestCase
	active  bool
	attempt uint64
}

func (r Recursor) validate(tc TestCase) error {
	if r.state == nil || !r.state.active || r.attempt != r.state.attempt {
		return fmt.Errorf("invalid or expired recursion scope")
	}
	_, nativeTC := tc.engine()
	if nativeTC != r.state.tc {
		return fmt.Errorf("recursion scope belongs to a different test case")
	}
	return nil
}

func (g RecursiveGenerator[T]) draw(tc TestCase) (T, error) {
	return drawRecursion(tc, g.maxDepth, g.maxLeaves, g.leaf,
		func(tc TestCase, child Recursor) (T, error) {
			subtree := subtreeGenerator[T]{leaf: g.leaf, branch: g.branch, recursor: child}
			return draw(tc, g.branch(&subtree))
		})
}

func (g RecursiveFuncGenerator[T]) draw(tc TestCase) (T, error) {
	return drawRecursion(tc, g.maxDepth, g.maxLeaves, g.leaf,
		func(tc TestCase, child Recursor) (T, error) {
			return g.branch(tc, child), nil
		})
}

func drawRecursion[T any](tc TestCase, maxDepth, maxLeaves int, leaf Generator[T], branch func(TestCase, Recursor) (T, error)) (T, error) {
	var zero T
	if maxDepth < 0 {
		return zero, fmt.Errorf("max_depth=%d must be non-negative", maxDepth)
	}
	if maxLeaves < 0 {
		return zero, fmt.Errorf("max_leaves=%d must be non-negative", maxLeaves)
	}
	if leaf == nil {
		return zero, fmt.Errorf("recursive leaf generator is nil")
	}

	ctx, nativeTC := tc.engine()
	native, err := nativeTC.NewRecursion(ctx, uint64(maxDepth), uint64(maxLeaves))
	if err != nil {
		return zero, err
	}
	state := &recursionState{native: native, tc: nativeTC}
	for {
		value, err := drawRecursionAttempt(tc, state, leaf, branch)
		if err == nil {
			return value, nil
		}

		// Both retry paths discard the attempt's native spans. The scoped
		// TestCase passed to invoke keeps their Go-side depth from leaking too.
		if _, ok := errors.AsType[*leafBudgetRetry](err); ok {
			if err := native.Retry(ctx, nativeTC); err != nil {
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

func drawRecursionAttempt[T any](tc TestCase, state *recursionState, leaf Generator[T], branch func(TestCase, Recursor) (T, error)) (T, error) {
	state.attempt++
	state.active = true
	defer func() { state.active = false }()

	var value T
	err := tc.invoke(func(attempt TestCase) {
		var drawErr error
		value, drawErr = draw(attempt, &rootRecursionGenerator[T]{
			leaf: leaf, branch: branch, recursor: Recursor{state: state, attempt: state.attempt},
		})
		if drawErr != nil {
			attempt.abort(drawErr)
		}
	})
	return value, err
}

type rootRecursionGenerator[T any] struct {
	leaf     Generator[T]
	branch   func(TestCase, Recursor) (T, error)
	recursor Recursor
}

func (g *rootRecursionGenerator[T]) draw(tc TestCase) (T, error) {
	return recursionNode(tc, g.recursor, g.leaf, g.branch)
}

type subtreeGenerator[T any] struct {
	leaf     Generator[T]
	branch   func(Generator[T]) Generator[T]
	recursor Recursor
}

func (g *subtreeGenerator[T]) draw(tc TestCase) (T, error) {
	if err := g.recursor.validate(tc); err != nil {
		var zero T
		return zero, err
	}
	return recursionNode(tc, g.recursor, g.leaf, func(tc TestCase, child Recursor) (T, error) {
		subtree := *g
		subtree.recursor = child
		return draw(tc, g.branch(&subtree))
	})
}

func recursionNode[T any](tc TestCase, r Recursor, leaf Generator[T], branch func(TestCase, Recursor) (T, error)) (T, error) {
	var zero T
	if leaf == nil {
		return zero, fmt.Errorf("recursive leaf generator is nil")
	}
	if err := tc.startSpan(labelFromName("recursive")); err != nil {
		return zero, err
	}

	ctx, nativeTC := tc.engine()
	isBranch, err := r.state.native.Branch(ctx, nativeTC, r.depth)
	if err != nil {
		return zero, err
	}

	var value T
	if isBranch {
		child := r
		child.depth++
		value, err = branch(tc, child)
	} else {
		if err := r.state.native.Leaf(ctx, nativeTC); err != nil {
			if errors.Is(err, libhegel.E_RETRY) {
				return zero, &leafBudgetRetry{err}
			}
			return zero, err
		}
		value, err = draw(tc, leaf)
	}
	if err != nil {
		return zero, err
	}

	if r.depth == 0 {
		err := r.state.native.Finish(ctx, nativeTC)
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
