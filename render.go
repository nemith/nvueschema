package main

import (
	"strings"

	"github.com/jwalton/gchalk"
)

// renderTypeSegs formats type segments with coloring into a strings.Builder.
// Wraps in brackets: [type | "literal"]
func renderTypeSegs(b *strings.Builder, segs []typeSegment) {
	if len(segs) == 0 {
		return
	}
	b.WriteString(" [")
	for _, seg := range segs {
		if seg.literal {
			b.WriteString(gchalk.Magenta(seg.text))
		} else {
			b.WriteString(gchalk.Yellow(seg.text))
		}
	}
	b.WriteString("]")
}

// renderDefault formats a default value with coloring into a strings.Builder.
func renderDefault(b *strings.Builder, defaultVal string) {
	if defaultVal == "" {
		return
	}
	b.WriteString(" " + gchalk.Cyan("(default: "+defaultVal+")"))
}

// renderDesc formats a description with coloring into a strings.Builder.
func renderDesc(b *strings.Builder, desc string) {
	if desc == "" {
		return
	}
	b.WriteString("  " + gchalk.Dim(desc))
}

// renderNodeDetail appends type, default, and description to a line.
func renderNodeDetail(b *strings.Builder, segs []typeSegment, defaultVal, desc string) {
	renderTypeSegs(b, segs)
	renderDefault(b, defaultVal)
	renderDesc(b, desc)
}
