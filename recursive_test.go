package hegel

import (
	"errors"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

type recursiveNode struct {
	value       int
	left, right *recursiveNode
}

func recursiveTree() RecursiveGenerator[*recursiveNode] {
	leaf := Map(Integers(0, 100), func(value int) *recursiveNode {
		return &recursiveNode{value: value}
	})
	return Recursive(leaf, func(subtree Generator[*recursiveNode]) Generator[*recursiveNode] {
		return Composite(func(tc TestCase) *recursiveNode {
			return &recursiveNode{
				left:  Draw(tc, subtree),
				right: Draw(tc, subtree),
			}
		})
	})
}

func treeDimensions(n *recursiveNode) (depth, leaves int) {
	if n.left == nil && n.right == nil {
		return 0, 1
	}
	leftDepth, leftLeaves := treeDimensions(n.left)
	rightDepth, rightLeaves := treeDimensions(n.right)
	return max(leftDepth, rightDepth) + 1, leftLeaves + rightLeaves
}

func TestRecursiveGeneratesBoundedTrees(t *testing.T) {
	t.Parallel()
	const (
		maxDepth  = 4
		maxLeaves = 8
	)
	gen := recursiveTree().MaxDepth(maxDepth).MaxLeaves(maxLeaves)
	var sawBranch bool

	Test(t, func(ht *T) {
		tree := Draw(ht, gen)
		depth, leaves := treeDimensions(tree)
		if depth > maxDepth {
			ht.Fatalf("tree depth %d exceeds maximum %d", depth, maxDepth)
		}
		if leaves > maxLeaves {
			ht.Fatalf("tree has %d leaves, exceeds maximum %d", leaves, maxLeaves)
		}
		if depth > 0 {
			sawBranch = true
		}
	}, WithTestCases(100))
	if !sawBranch {
		t.Fatal("recursive generator never generated a branch")
	}
}

func TestRecursiveMaxDepthZeroOnlyGeneratesLeaves(t *testing.T) {
	t.Parallel()
	branchCalls := 0
	gen := Recursive(Just(42), func(Generator[int]) Generator[int] {
		branchCalls++
		return Just(0)
	}).MaxDepth(0)

	Test(t, func(ht *T) {
		if got := Draw(ht, gen); got != 42 {
			ht.Fatalf("got %d, want leaf value 42", got)
		}
	}, WithTestCases(20))
	if branchCalls != 0 {
		t.Fatalf("branch called %d times at maximum depth zero", branchCalls)
	}
}

func TestRecursiveBuilderMethodsAreImmutable(t *testing.T) {
	t.Parallel()
	base := Recursive(Just(0), func(Generator[int]) Generator[int] { return Just(1) })
	configured := base.MaxDepth(7).MaxLeaves(11)

	if base.maxDepth != defaultRecursiveMaxDepth || base.maxLeaves != defaultRecursiveMaxLeaves {
		t.Fatalf("configuration mutated base: depth=%d leaves=%d", base.maxDepth, base.maxLeaves)
	}
	if configured.maxDepth != 7 || configured.maxLeaves != 11 {
		t.Fatalf("configuration not applied: depth=%d leaves=%d", configured.maxDepth, configured.maxLeaves)
	}
}

func TestRecursiveRejectsNegativeLimits(t *testing.T) {
	t.Parallel()
	base := Recursive(Just(0), func(Generator[int]) Generator[int] { return Just(1) })
	for _, test := range []struct {
		name string
		gen  RecursiveGenerator[int]
		want string
	}{
		{name: "depth", gen: base.MaxDepth(-1), want: "max_depth=-1 must be non-negative"},
		{name: "leaves", gen: base.MaxLeaves(-1), want: "max_leaves=-1 must be non-negative"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.gen.draw(&testCase{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("draw error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRecursiveRetriesLeafBudgetOverflow(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(9), libhegel.OK, // new_recursion
		libhegel.OK,        // first generator span
		libhegel.OK,        // first recursive span
		false, libhegel.OK, // first branch
		libhegel.E_RETRY, "leaf budget", // first leaf
		libhegel.OK,        // retry
		libhegel.OK,        // second generator span
		libhegel.OK,        // second recursive span
		false, libhegel.OK, // second branch
		libhegel.OK, // second leaf
		libhegel.OK, // leaf generator span
		libhegel.OK, // leaf generator stop
		libhegel.OK, // finish
		libhegel.OK, // recursive span stop
		libhegel.OK, // generator span stop
	)

	got, err := Recursive(Just(42), func(Generator[int]) Generator[int] { return Just(0) }).draw(tc)
	if err != nil || got != 42 {
		t.Fatalf("draw = %d, %v; want 42, nil", got, err)
	}
}

func TestRecursiveRestartsRepricedAttemptWithoutRetry(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(9), libhegel.OK, // new_recursion
		libhegel.OK,        // first generator span
		libhegel.OK,        // first recursive span
		false, libhegel.OK, // first branch
		libhegel.OK,                  // first leaf
		libhegel.OK,                  // first leaf generator span
		libhegel.OK,                  // first leaf generator stop
		libhegel.E_RETRY, "repriced", // first finish
		libhegel.OK,        // second generator span
		libhegel.OK,        // second recursive span
		false, libhegel.OK, // second branch
		libhegel.OK, // second leaf
		libhegel.OK, // second leaf generator span
		libhegel.OK, // second leaf generator stop
		libhegel.OK, // second finish
		libhegel.OK, // recursive span stop
		libhegel.OK, // generator span stop
	)

	got, err := Recursive(Just(42), func(Generator[int]) Generator[int] { return Just(0) }).draw(tc)
	if err != nil || got != 42 {
		t.Fatalf("draw = %d, %v; want 42, nil", got, err)
	}
}

func TestRecursivePropagatesRetryExhaustion(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(9), libhegel.OK, // new_recursion
		libhegel.OK,        // generator span
		libhegel.OK,        // recursive span
		false, libhegel.OK, // branch
		libhegel.E_RETRY, "leaf budget", // leaf
		libhegel.E_ASSUME, "attempts exhausted", // retry
	)

	_, err := Recursive(Just(42), func(Generator[int]) Generator[int] { return Just(0) }).draw(tc)
	if !errors.Is(err, libhegel.E_ASSUME) {
		t.Fatalf("draw error = %v, want E_ASSUME", err)
	}
}

func TestRecursivePropagatesStartSpanError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(9), libhegel.OK, // new_recursion
		libhegel.OK,                       // generator span
		libhegel.E_BACKEND, "span failed", // recursive span
	)

	_, err := Recursive(Just(42), func(Generator[int]) Generator[int] { return Just(0) }).draw(tc)
	if !errors.Is(err, libhegel.E_BACKEND) {
		t.Fatalf("draw error = %v, want E_BACKEND", err)
	}
}

func TestRecursivePropagatesLeafError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(9), libhegel.OK, // new_recursion
		libhegel.OK,        // generator span
		libhegel.OK,        // recursive span
		false, libhegel.OK, // branch
		libhegel.E_BACKEND, "leaf failed", // leaf
	)

	_, err := Recursive(Just(42), func(Generator[int]) Generator[int] { return Just(0) }).draw(tc)
	if !errors.Is(err, libhegel.E_BACKEND) {
		t.Fatalf("draw error = %v, want E_BACKEND", err)
	}
}

func TestRecursivePropagatesFinishError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(9), libhegel.OK, // new_recursion
		libhegel.OK,        // generator span
		libhegel.OK,        // recursive span
		false, libhegel.OK, // branch
		libhegel.OK,                         // leaf
		libhegel.OK,                         // leaf generator span
		libhegel.OK,                         // leaf generator stop
		libhegel.E_BACKEND, "finish failed", // finish
	)

	_, err := Recursive(Just(42), func(Generator[int]) Generator[int] { return Just(0) }).draw(tc)
	if !errors.Is(err, libhegel.E_BACKEND) {
		t.Fatalf("draw error = %v, want E_BACKEND", err)
	}
}

func TestRecursivePropagatesStopSpanError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(9), libhegel.OK, // new_recursion
		libhegel.OK,        // generator span
		libhegel.OK,        // recursive span
		false, libhegel.OK, // branch
		libhegel.OK,                            // leaf
		libhegel.OK,                            // leaf generator span
		libhegel.OK,                            // leaf generator stop
		libhegel.OK,                            // finish
		libhegel.E_BACKEND, "stop span failed", // recursive span stop
	)

	_, err := Recursive(Just(42), func(Generator[int]) Generator[int] { return Just(0) }).draw(tc)
	if !errors.Is(err, libhegel.E_BACKEND) {
		t.Fatalf("draw error = %v, want E_BACKEND", err)
	}
}

func TestRecursiveShrinksToMinimalBranch(t *testing.T) {
	t.Parallel()
	gen := recursiveTree().MaxDepth(4)
	var minimal *recursiveNode

	err := run(1, func(tc TestCase) {
		tree := Draw(tc, gen)
		depth, _ := treeDimensions(tree)
		if depth > 0 {
			minimal = tree
			tc.FailNow()
		}
	}, WithTestCases(200))
	if err == nil {
		t.Fatal("never generated a branch")
	}

	depth, leaves := treeDimensions(minimal)
	if depth != 1 || leaves != 2 {
		t.Fatalf("minimal branch has depth=%d leaves=%d, want depth=1 leaves=2", depth, leaves)
	}
	if minimal.left.value != 0 || minimal.right.value != 0 {
		t.Fatalf("minimal leaf values are %d and %d, want 0 and 0", minimal.left.value, minimal.right.value)
	}
}
