package main

import (
	"strings"

	"github.com/jwalton/gchalk"
	"nemith.io/nvueschema"
)

// renderTypeSegs formats type segments with coloring into a strings.Builder.
func renderTypeSegs(b *strings.Builder, segs []nvueschema.TypeSegment) {
	if len(segs) == 0 {
		return
	}
	b.WriteString(" [")
	for _, seg := range segs {
		if seg.Literal {
			b.WriteString(gchalk.Magenta(seg.Text))
		} else {
			b.WriteString(gchalk.Yellow(seg.Text))
		}
	}
	b.WriteString("]")
}

// renderDefault formats a default value with coloring.
func renderDefault(b *strings.Builder, defaultVal string) {
	if defaultVal == "" {
		return
	}
	b.WriteString(" " + gchalk.Cyan("(default: "+defaultVal+")"))
}

// renderDesc formats a description with coloring.
func renderDesc(b *strings.Builder, desc string) {
	if desc == "" {
		return
	}
	b.WriteString("  " + gchalk.Dim(desc))
}

// renderNodeDetail appends type, default, and description to a line.
func renderNodeDetail(b *strings.Builder, segs []nvueschema.TypeSegment, defaultVal, desc string) {
	renderTypeSegs(b, segs)
	renderDefault(b, defaultVal)
	renderDesc(b, desc)
}
