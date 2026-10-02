package hegel

import (
	"strings"
	"testing"
	"unsafe"
)

type defaultNamedInt int16
type defaultNamedString string
type defaultNamedPointer *defaultTree

type defaultRecord struct {
	Flag   bool
	Count  defaultNamedInt
	Name   defaultNamedString
	Values []uint8
	Lookup map[string]int
	Pair   [2]float32
}

type defaultTree struct {
	Value int
	Next  defaultNamedPointer
}

type defaultA struct{ B *defaultB }
type defaultB struct{ A *defaultA }
type defaultList struct{ Children []defaultList }
type defaultMap map[string]defaultMap
type defaultBinary struct{ Left, Right *defaultBinary }
type defaultKey struct{ Next *defaultKey }
type defaultKeyMap map[defaultKey]int
type defaultLeaf struct{ Value int }
type defaultShared struct{ Left, Right *defaultLeaf }

func TestDefaultCachesSharedAcyclicShape(t *testing.T) {
	g := Default[defaultShared]().(*defaultGenerator[defaultShared])
	if g.recursive {
		t.Fatal("shared field type was mistaken for a cycle")
	}
	if g.root.shape.fields[0].shape.elem != g.root.shape.fields[1].shape.elem {
		t.Fatal("shared field type has separate plans")
	}
}

func TestDefaultCompositeTypes(t *testing.T) {
	gen := Default[defaultRecord]()
	sawValues, sawLookup, sawCount := false, false, false
	Test(t, func(tc *T) {
		v := Draw(tc, gen)
		if v.Values == nil || v.Lookup == nil {
			tc.Fatal("collections were not constructed")
		}
		sawValues = sawValues || len(v.Values) > 0
		sawLookup = sawLookup || len(v.Lookup) > 0
		sawCount = sawCount || v.Count != 0
	}, WithTestCases(50))
	if !sawValues || !sawLookup || !sawCount {
		t.Fatalf("generation lacked variety: values=%v lookup=%v count=%v", sawValues, sawLookup, sawCount)
	}
}

func TestDefaultRecursiveTypes(t *testing.T) {
	treeGen := Default[defaultTree]()
	mutualGen := Default[defaultA]()
	listGen := Default[defaultList]()
	mapGen := Default[defaultMap]()
	tests := []struct {
		name  string
		check func(*T) bool
	}{
		{"named pointer", func(tc *T) bool {
			v := Draw(tc, treeGen)
			depth := 0
			for v.Next != nil {
				depth++
				v = *v.Next
			}
			if depth > defaultRecursiveMaxDepth {
				tc.Fatalf("pointer depth %d exceeds limit", depth)
			}
			return depth > 0
		}},
		{"mutual", func(tc *T) bool {
			v := Draw(tc, mutualGen)
			depth := 0
			for v.B != nil {
				depth++
				if v.B.A == nil {
					break
				}
				v = *v.B.A
			}
			if depth > defaultRecursiveMaxDepth {
				tc.Fatalf("mutual depth %d exceeds limit", depth)
			}
			return depth > 0
		}},
		{"slice", func(tc *T) bool {
			v := Draw(tc, listGen)
			depth := 0
			for len(v.Children) > 0 {
				depth++
				v = v.Children[0]
			}
			if depth > defaultRecursiveMaxDepth {
				tc.Fatalf("slice depth %d exceeds limit", depth)
			}
			return depth > 0
		}},
		{"map", func(tc *T) bool {
			v := Draw(tc, mapGen)
			depth := 0
			for len(v) > 0 {
				depth++
				for _, child := range v {
					v = child
					break
				}
			}
			if depth > defaultRecursiveMaxDepth {
				tc.Fatalf("map depth %d exceeds limit", depth)
			}
			return depth > 0
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sawBranch := false
			Test(t, func(tc *T) {
				branch := test.check(tc)
				sawBranch = sawBranch || branch
			}, WithTestCases(50))
			if !sawBranch {
				t.Fatal("no recursive branch was generated")
			}
		})
	}
}

func TestDefaultShrinksRecursivePointer(t *testing.T) {
	gen := Default[defaultTree]()
	var minimal defaultTree
	err := run(1, func(tc TestCase) {
		v := Draw(tc, gen)
		if v.Next != nil {
			minimal = v
			tc.FailNow()
		}
	}, WithTestCases(200))
	if err == nil {
		t.Fatal("no recursive branch was generated")
	}
	if minimal.Next == nil || minimal.Next.Next != nil {
		t.Fatalf("minimal tree = %#v, want one recursive edge", minimal)
	}
}

func TestDefaultSiblingRecursionSharesBudget(t *testing.T) {
	gen := Default[defaultBinary]()
	var dimensions func(*defaultBinary) (int, int)
	dimensions = func(n *defaultBinary) (int, int) {
		if n == nil {
			return 0, 1
		}
		leftDepth, leftLeaves := dimensions(n.Left)
		rightDepth, rightLeaves := dimensions(n.Right)
		return max(leftDepth, rightDepth) + 1, leftLeaves + rightLeaves
	}
	Test(t, func(tc *T) {
		value := Draw(tc, gen)
		depth, leaves := dimensions(&value)
		if depth > defaultRecursiveMaxDepth+1 || leaves > defaultRecursiveMaxLeaves {
			tc.Fatalf("depth=%d leaves=%d exceed recursion limits", depth, leaves)
		}
	}, WithTestCases(100))
}

func TestDefaultRecursiveMapKey(t *testing.T) {
	gen := Default[defaultKeyMap]()
	sawEntry := false
	Test(t, func(tc *T) {
		value := Draw(tc, gen)
		for key := range value {
			sawEntry = true
			depth := 0
			for key.Next != nil {
				depth++
				key = *key.Next
			}
			if depth > defaultRecursiveMaxDepth {
				tc.Fatalf("map key depth %d exceeds limit", depth)
			}
		}
	}, WithTestCases(50))
	if !sawEntry {
		t.Fatal("no map entry was generated")
	}
}

func TestDefaultRejectsUnsupportedFields(t *testing.T) {
	for _, test := range []struct {
		name string
		make func()
		want string
	}{
		{"interface", func() { Default[struct{ Value any }]() }, "unsupported kind interface"},
		{"private", func() { Default[struct{ value int }]() }, "field value is unexported"},
		{"channel", func() { Default[chan int]() }, "unsupported kind chan"},
		{"function", func() { Default[func()]() }, "unsupported kind func"},
		{"complex", func() { Default[complex128]() }, "unsupported kind complex128"},
		{"unsafe pointer", func() { Default[unsafe.Pointer]() }, "unsupported kind unsafe.Pointer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				p := recover()
				if p == nil || !strings.Contains(p.(string), test.want) {
					t.Fatalf("panic = %v, want %q", p, test.want)
				}
			}()
			test.make()
		})
	}
}
