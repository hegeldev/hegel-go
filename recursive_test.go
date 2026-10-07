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

type mixedStmt struct {
	condition *mixedExpr
	value     int
}

type mixedExpr struct{ nested *mixedStmt }

func mixedTree() RecursiveFuncGenerator[*mixedStmt] {
	stmtLeaf := Just(&mixedStmt{})
	exprLeaf := Just(&mixedExpr{})
	var stmtBranch func(TestCase, Recursor) *mixedStmt
	var exprBranch func(TestCase, Recursor) *mixedExpr
	stmtBranch = func(tc TestCase, r Recursor) *mixedStmt {
		return &mixedStmt{
			condition: Recurse(tc, r, exprLeaf, exprBranch),
			value:     Recurse(tc, r, Just(1), func(TestCase, Recursor) int { return 2 }),
		}
	}
	exprBranch = func(tc TestCase, r Recursor) *mixedExpr {
		return &mixedExpr{nested: Recurse(tc, r, stmtLeaf, stmtBranch)}
	}
	return RecursiveFunc(stmtLeaf, stmtBranch)
}

func mixedStmtDimensions(n *mixedStmt) (depth, leaves int) {
	if n.condition == nil {
		return 0, 1
	}
	childDepth, childLeaves := mixedExprDimensions(n.condition)
	return childDepth + 1, childLeaves + 1
}

func mixedExprDimensions(n *mixedExpr) (depth, leaves int) {
	if n.nested == nil {
		return 0, 1
	}
	childDepth, childLeaves := mixedStmtDimensions(n.nested)
	return childDepth + 1, childLeaves
}

func TestRecursiveFuncMixedTypesShareLimits(t *testing.T) {
	t.Parallel()
	const maxDepth, maxLeaves = 4, 4
	var sawBranch bool
	Test(t, func(ht *T) {
		tree := Draw(ht, mixedTree().MaxDepth(maxDepth).MaxLeaves(maxLeaves))
		depth, leaves := mixedStmtDimensions(tree)
		if depth > maxDepth || leaves > maxLeaves {
			ht.Fatalf("depth=%d leaves=%d exceeds limits", depth, leaves)
		}
		if tree.condition != nil {
			sawBranch = true
		}
	}, WithTestCases(100))
	if !sawBranch {
		t.Fatal("never generated a mixed branch")
	}
}

func TestRecursiveFuncDepthZeroSkipsBranch(t *testing.T) {
	t.Parallel()
	calls := 0
	gen := RecursiveFunc(Just(42), func(TestCase, Recursor) int {
		calls++
		return 0
	}).MaxDepth(0)
	Test(t, func(ht *T) {
		if got := Draw(ht, gen); got != 42 {
			ht.Fatalf("got %d, want 42", got)
		}
	}, WithTestCases(20))
	if calls != 0 {
		t.Fatalf("branch called %d times", calls)
	}
}

func TestRecursiveFuncBuildersAreImmutable(t *testing.T) {
	t.Parallel()
	base := mixedTree()
	configured := base.MaxDepth(3).MaxLeaves(5)
	if base.maxDepth != defaultRecursiveMaxDepth || base.maxLeaves != defaultRecursiveMaxLeaves {
		t.Fatal("builder mutated base")
	}
	if configured.maxDepth != 3 || configured.maxLeaves != 5 {
		t.Fatal("builder did not set limits")
	}
}

func TestRecursiveFuncShrinksMixedBranch(t *testing.T) {
	t.Parallel()
	var minimal *mixedStmt
	err := run(1, func(tc TestCase) {
		value := Draw(tc, mixedTree().MaxDepth(4).MaxLeaves(4))
		if value.condition != nil {
			minimal = value
			tc.FailNow()
		}
	}, WithTestCases(200))
	if err == nil {
		t.Fatal("never generated a branch")
	}
	depth, leaves := mixedStmtDimensions(minimal)
	if depth != 1 || leaves != 2 || minimal.condition.nested != nil || minimal.value != 1 {
		t.Fatalf("minimal mixed branch: depth=%d leaves=%d value=%d", depth, leaves, minimal.value)
	}
}

func TestRecurseRejectsInvalidScopes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		makeScope func(*testCase) Recursor
		want      string
	}{
		{"zero", func(*testCase) Recursor { return Recursor{} }, "invalid or expired recursion scope"},
		{"expired", func(tc *testCase) Recursor {
			_, native := tc.engine()
			return Recursor{state: &recursionState{tc: native, attempt: 1}, attempt: 1}
		}, "invalid or expired recursion scope"},
		{"previous attempt", func(tc *testCase) Recursor {
			_, native := tc.engine()
			return Recursor{state: &recursionState{tc: native, active: true, attempt: 2}, attempt: 1}
		}, "invalid or expired recursion scope"},
		{"cross-test", func(*testCase) Recursor {
			return Recursor{state: &recursionState{active: true, attempt: 1}, attempt: 1}
		}, "recursion scope belongs to a different test case"},
	} {
		t.Run(test.name, func(t *testing.T) {
			tc := newStubTestCase(t)
			err := tc.invoke(func(scoped TestCase) {
				Recurse(scoped, test.makeScope(tc), Just(0), func(TestCase, Recursor) int { return 1 })
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRecursiveFuncRejectsNilRootLeaf(t *testing.T) {
	t.Parallel()
	var leaf Generator[int]
	_, err := RecursiveFunc(leaf, func(TestCase, Recursor) int { return 0 }).draw(&testCase{})
	if err == nil || !strings.Contains(err.Error(), "recursive leaf generator is nil") {
		t.Fatalf("error = %v, want nil leaf error", err)
	}
}

func TestRecurseRejectsNilChildLeaf(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(9), libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		true, libhegel.OK,
	)
	var leaf Generator[int]
	_, err := RecursiveFunc(Just(1), func(tc TestCase, r Recursor) int {
		return Recurse(tc, r, leaf, func(TestCase, Recursor) int { return 2 })
	}).draw(tc)
	if err == nil || !strings.Contains(err.Error(), "recursive leaf generator is nil") {
		t.Fatalf("error = %v, want nil leaf error", err)
	}
}

func TestRecursiveRejectsExpiredChildGenerator(t *testing.T) {
	t.Parallel()
	child := subtreeGenerator[int]{leaf: Just(1)}
	_, err := child.draw(newStubTestCase(t))
	if err == nil || !strings.Contains(err.Error(), "invalid or expired recursion scope") {
		t.Fatalf("error = %v, want expired scope", err)
	}
}

func TestRecurseReturnsChildBranchValue(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(9), libhegel.OK, // recursion handle
		libhegel.OK,       // recursive span
		true, libhegel.OK, // branch
		libhegel.OK, // recursive span stop
	)
	ctx, nativeTC := tc.engine()
	native, err := nativeTC.NewRecursion(ctx, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	state := &recursionState{native: native, tc: nativeTC, active: true, attempt: 1}
	err = tc.invoke(func(scoped TestCase) {
		got := Recurse(scoped, Recursor{state: state, depth: 1, attempt: 1}, Just(1),
			func(TestCase, Recursor) int { return 2 })
		if got != 2 {
			t.Fatalf("child branch returned %d, want 2", got)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecurseRejectsScopeAfterDraw(t *testing.T) {
	t.Parallel()
	var retained Recursor
	err := run(1, func(tc TestCase) {
		_ = Draw(tc, RecursiveFunc(Just(1), func(_ TestCase, r Recursor) int {
			retained = r
			return 2
		}).MaxDepth(1))
	}, WithTestCases(100))
	if err != nil {
		t.Fatal(err)
	}
	if retained.state == nil {
		t.Fatal("never captured a branch scope")
	}
	tc := newStubTestCase(t)
	err = tc.invoke(func(scoped TestCase) {
		Recurse(scoped, retained, Just(1), func(TestCase, Recursor) int { return 2 })
	})
	if err == nil || !strings.Contains(err.Error(), "invalid or expired recursion scope") {
		t.Fatalf("error = %v, want expired scope", err)
	}
}

func TestRecursiveFuncAllowsZeroLeafLimit(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t,
		uintptr(9), libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		false, libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		libhegel.OK,
	)
	got, err := RecursiveFunc(Just(42), func(TestCase, Recursor) int { return 0 }).MaxLeaves(0).draw(tc)
	if err != nil || got != 42 {
		t.Fatalf("draw = %d, %v; want 42, nil", got, err)
	}
}
