package yangregexp

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/dlclark/regexp2/v2/syntax"
)

// Pattern is one XML Schema regex and its YANG 1.1 matching polarity.
// Expression is unquoted; the caller must escape it for the output language.
type Pattern struct {
	Expression  string
	InvertMatch bool
}

// ErrUnsupported means a valid parsed expression cannot be translated safely.
var ErrUnsupported = errors.New("unsupported regex conversion")

// ErrLimit means conversion exceeded a resource bound.
var ErrLimit = errors.New("regex conversion limit exceeded")

const (
	maxInput    = 16 << 10
	maxOutput   = 1 << 20
	maxBranches = 256
	maxDepth    = 128
	anyRune     = `[\s\S]`
	noRune      = `[^\s\S]`
)

// Convert returns conjunctive YANG patterns with the same MatchString behavior
// as regexp2's ECMAScript mode over XML 1.0 strings. Invalid source syntax,
// unsupported constructs, and resource limits return no partial result.
func Convert(source string) ([]Pattern, error) {
	if len(source) > maxInput {
		return nil, ErrLimit
	}
	if !utf8.ValidString(source) {
		return nil, fmt.Errorf("invalid UTF-8 regex")
	}
	tree, err := syntax.Parse(source, syntax.ParseOptions{RegexOptions: syntax.ECMAScript})
	if err != nil {
		return nil, fmt.Errorf("parse regex: %w", err)
	}
	c := converter{}
	branches, err := c.lower(tree.Root, nil, 0)
	if err != nil {
		return nil, err
	}
	return c.patterns(branches, false, 0)
}

type part struct {
	kind syntax.NodeType
	text string
	look *syntax.RegexNode
}
type branch []part
type converter struct{ size int }

func unsupported(reason string) error { return fmt.Errorf("%w: %s", ErrUnsupported, reason) }

// lower removes captures and lowers regular subtrees to XSD. Assertions remain
// structured parts so concatenation and alternation retain their precedence.
func (c *converter) lower(n *syntax.RegexNode, tail []*syntax.RegexNode, depth int) ([]branch, error) {
	if depth > maxDepth {
		return nil, ErrLimit
	}
	if n.Options & ^syntax.ECMAScript != 0 {
		return nil, unsupported("regex flags or lookbehind")
	}
	atom := func(text string) ([]branch, error) {
		c.size += len(text)
		if c.size > maxOutput {
			return nil, ErrLimit
		}
		return []branch{{{kind: syntax.NtOne, text: text}}}, nil
	}
	switch n.T {
	case syntax.NtCapture, syntax.NtGroup:
		if n.T == syntax.NtCapture && n.N != -1 {
			return nil, unsupported("balancing capture")
		}
		return c.lower(n.Children[0], tail, depth+1)
	case syntax.NtAtomic:
		// regexp2 also inserts atomic groups during optimization. Removing one is
		// safe at the end of a match, or when its consumed length is fixed.
		if len(tail) != 0 && !fixedLength(n.Children[0]) {
			return nil, unsupported("atomic group before a continuation")
		}
		return c.lower(n.Children[0], tail, depth+1)
	case syntax.NtEmpty, syntax.NtUpdateBumpalong:
		return atom("")
	case syntax.NtResetCapture:
		// Repeated ECMAScript groups clear captures before each iteration.
		// This consumes nothing and cannot affect matching in our supported
		// subset: backreferences and capture-dependent conditions are rejected.
		return atom("")
	case syntax.NtNothing:
		return atom(noRune)
	case syntax.NtBeginning, syntax.NtEnd:
		return []branch{{{kind: n.T}}}, nil
	case syntax.NtPosLook, syntax.NtNegLook:
		return []branch{{{kind: n.T, look: n.Children[0]}}}, nil
	case syntax.NtConcatenate:
		result := []branch{{}}
		for i, child := range n.Children {
			next := append(append([]*syntax.RegexNode{}, n.Children[i+1:]...), tail...)
			bs, err := c.lower(child, next, depth+1)
			if err != nil {
				return nil, err
			}
			if len(result)*len(bs) > maxBranches {
				return nil, ErrLimit
			}
			var joined []branch
			for _, left := range result {
				for _, right := range bs {
					b := append(append(branch{}, left...), right...)
					joined = append(joined, b)
				}
			}
			result = joined
		}
		return result, nil
	case syntax.NtAlternate:
		var result []branch
		for _, child := range n.Children {
			bs, err := c.lower(child, tail, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, bs...)
			if len(result) > maxBranches {
				return nil, ErrLimit
			}
		}
		// Keep ordinary alternatives grouped; distribute only assertion-bearing
		// branches. This avoids exponential expansion for regular expressions.
		if text, ok := regular(result); ok {
			return atom(text)
		}
		return result, nil
	case syntax.NtLoop, syntax.NtLazyloop:
		next := append([]*syntax.RegexNode{n}, tail...)
		bs, err := c.lower(n.Children[0], next, depth+1)
		if err != nil {
			return nil, err
		}
		text, ok := regular(bs)
		if !ok {
			return nil, unsupported("repeated assertion")
		}
		return atom(repeat(text, n.M, n.N))
	case syntax.NtMulti:
		var b strings.Builder
		for _, ch := range n.Str {
			b.WriteString(literal(ch))
		}
		return atom(b.String())
	default:
		if n.IsOneFamily() || n.IsNotoneFamily() || n.IsSetFamily() {
			if n.IsAtomicloopFamily() && n.M != n.N && !disjointContinuation(n, tail) {
				return nil, unsupported("atomic repetition overlapping its continuation")
			}
			text := character(n)
			switch n.T {
			case syntax.NtOne, syntax.NtNotone, syntax.NtSet:
				return atom(text)
			default:
				return atom(repeat(text, n.M, n.N))
			}
		}
		return nil, unsupported(fmt.Sprintf("AST node %v", n.T))
	}
}

func regular(bs []branch) (string, bool) {
	texts := make([]string, 0, len(bs))
	for _, b := range bs {
		var text strings.Builder
		for _, p := range b {
			if p.kind != syntax.NtOne {
				return "", false
			}
			text.WriteString(p.text)
		}
		texts = append(texts, text.String())
	}
	return alternate(texts), true
}

func alternate(texts []string) string {
	if len(texts) == 1 {
		return texts[0]
	}
	return "(" + strings.Join(texts, "|") + ")"
}

func repeat(text string, minimum, maximum int) string {
	suffix := ""
	switch {
	case minimum == 1 && maximum == 1:
		return text
	case minimum == 0 && maximum == 1:
		suffix = "?"
	case minimum == 0 && maximum == math.MaxInt32:
		suffix = "*"
	case minimum == 1 && maximum == math.MaxInt32:
		suffix = "+"
	case minimum == maximum:
		suffix = fmt.Sprintf("{%d}", minimum)
	case maximum == math.MaxInt32:
		suffix = fmt.Sprintf("{%d,}", minimum)
	default:
		suffix = fmt.Sprintf("{%d,%d}", minimum, maximum)
	}
	return "(" + text + ")" + suffix
}

func (c *converter) patterns(bs []branch, anchored bool, depth int) ([]Pattern, error) {
	if depth > maxDepth {
		return nil, ErrLimit
	}
	var alternatives []string
	var guards []Pattern
	for _, b := range bs {
		start, end := anchored, false
		var body strings.Builder
		for _, p := range b {
			switch p.kind {
			case syntax.NtOne:
				if end && p.text != "" {
					return nil, unsupported("consumption after end anchor")
				}
				body.WriteString(p.text)
			case syntax.NtBeginning:
				if body.Len() != 0 {
					return nil, unsupported("interior start anchor")
				}
				start = true
			case syntax.NtEnd:
				end = true
			case syntax.NtPosLook, syntax.NtNegLook:
				if !start || end || body.Len() != 0 {
					return nil, unsupported("lookahead outside the absolute start")
				}
				if len(bs) != 1 {
					return nil, unsupported("lookahead inside alternation")
				}
				inner, err := c.lower(p.look, nil, depth+1)
				if err != nil {
					return nil, err
				}
				ps, err := c.patterns(inner, true, depth+1)
				if err != nil {
					return nil, err
				}
				if p.kind == syntax.NtNegLook {
					if len(ps) != 1 {
						return nil, unsupported("negation of intersecting lookaheads")
					}
					ps[0].InvertMatch = !ps[0].InvertMatch
				}
				guards = append(guards, ps...)
			}
		}
		text := body.String()
		if !start {
			text = anyRune + "*" + text
		}
		if !end {
			text += anyRune + "*"
		}
		alternatives = append(alternatives, text)
	}
	result := append([]Pattern{{Expression: alternate(alternatives)}}, guards...)
	size := 0
	for _, p := range result {
		size += len(p.Expression)
	}
	if size > maxOutput {
		return nil, ErrLimit
	}
	return result, nil
}
