package yangregexp

import (
	"math"

	"github.com/dlclark/regexp2/v2/syntax"
)

// An atomic single-character repetition is safe to emit as a normal repetition
// when its continuation cannot consume that character. This includes an end
// anchor. Unlike a general atomic group, such a loop has no ordered branches.
func disjointContinuation(loop *syntax.RegexNode, tail []*syntax.RegexNode) bool {
	chars := nodeRanges(loop)
	for _, n := range tail {
		disjoint, nullable := disjointFirst(chars, n)
		if !disjoint {
			return false
		}
		if !nullable {
			return true
		}
	}
	return true
}

func disjointFirst(chars []interval, n *syntax.RegexNode) (bool, bool) {
	if n.IsOneFamily() || n.IsNotoneFamily() || n.IsSetFamily() {
		nullable := n.T != syntax.NtOne && n.T != syntax.NtNotone && n.T != syntax.NtSet && n.M == 0
		return len(intersect(chars, nodeRanges(n))) == 0, nullable
	}
	switch n.T {
	case syntax.NtMulti:
		return len(intersect(chars, []interval{{First: n.Str[0], Last: n.Str[0]}})) == 0, false
	case syntax.NtEmpty, syntax.NtBeginning, syntax.NtEnd, syntax.NtUpdateBumpalong, syntax.NtResetCapture:
		return true, true
	case syntax.NtNothing:
		return true, false
	case syntax.NtCapture, syntax.NtGroup, syntax.NtAtomic:
		return disjointFirst(chars, n.Children[0])
	case syntax.NtLoop, syntax.NtLazyloop:
		ok, nullable := disjointFirst(chars, n.Children[0])
		return ok, nullable || n.M == 0
	case syntax.NtConcatenate:
		for _, child := range n.Children {
			ok, nullable := disjointFirst(chars, child)
			if !ok || !nullable {
				return ok, nullable
			}
		}
		return true, true
	case syntax.NtAlternate:
		nullable := false
		for _, child := range n.Children {
			ok, empty := disjointFirst(chars, child)
			if !ok {
				return false, false
			}
			nullable = nullable || empty
		}
		return true, nullable
	default:
		// Assertions can constrain the repetition's length without consuming it.
		return false, false
	}
}

func fixedLength(n *syntax.RegexNode) bool { _, ok := exactLength(n); return ok }

func exactLength(n *syntax.RegexNode) (int, bool) {
	switch n.T {
	case syntax.NtOne, syntax.NtNotone, syntax.NtSet:
		return 1, true
	case syntax.NtMulti:
		return len(n.Str), true
	case syntax.NtEmpty, syntax.NtBeginning, syntax.NtEnd, syntax.NtUpdateBumpalong, syntax.NtResetCapture:
		return 0, true
	case syntax.NtCapture, syntax.NtGroup, syntax.NtAtomic:
		return exactLength(n.Children[0])
	case syntax.NtConcatenate:
		length := 0
		for _, child := range n.Children {
			size, ok := exactLength(child)
			if !ok || size > math.MaxInt-length {
				return 0, false
			}
			length += size
		}
		return length, true
	case syntax.NtAlternate:
		length, ok := exactLength(n.Children[0])
		for _, child := range n.Children[1:] {
			size, same := exactLength(child)
			ok = ok && same && size == length
		}
		return length, ok
	case syntax.NtLoop, syntax.NtLazyloop:
		size, ok := exactLength(n.Children[0])
		if !ok || n.M != n.N || size > math.MaxInt/max(1, n.M) {
			return 0, false
		}
		return size * n.M, true
	default:
		if n.IsOneFamily() || n.IsNotoneFamily() || n.IsSetFamily() {
			return n.M, n.M == n.N
		}
		return 0, false
	}
}
