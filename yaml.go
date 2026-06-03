package manifestrenderer

import (
	"strconv"
	"strings"
)

// node is a minimal, deterministic YAML document model. It exists so the
// rendered bytes are owned by this package (not an upstream marshaller whose
// formatting may change between versions), which is what makes the three
// dialects byte-stable and diffable in CI. Only the subset needed for K8s
// manifests is modeled: scalars, ordered maps, and sequences.
type node struct {
	kind  nodeKind
	str   string   // scalar / rawScalar
	keys  []string // map key order (insertion order is authored, deterministic)
	vals  []*node  // map values, parallel to keys
	items []*node  // sequence items
}

type nodeKind int

const (
	// scalarNode is a string scalar: safe-quoted so a number- or bool-looking
	// string stays a string.
	scalarNode nodeKind = iota
	// rawScalarNode is a non-string scalar (int, bool) rendered verbatim — never
	// quoted, so YAML reads it as the intended type.
	rawScalarNode
	mapNode
	seqNode
)

// scalar builds a string scalar node (safe-quoted as needed).
func scalar(s string) *node { return &node{kind: scalarNode, str: s} }

// rawScalar builds a verbatim scalar node — used for values that must NOT be
// quoted (ints, bools) so YAML parses them as the intended type.
func rawScalar(s string) *node { return &node{kind: rawScalarNode, str: s} }

// intScalar builds an integer scalar node, rendered verbatim (unquoted).
func intScalar(n int) *node { return rawScalar(strconv.Itoa(n)) }

// boolScalar builds a boolean scalar node, rendered verbatim (unquoted).
func boolScalar(b bool) *node {
	if b {
		return rawScalar("true")
	}
	return rawScalar("false")
}

// mapping builds an ordered map node. Authoring the keys explicitly (never
// iterating a Go map) is what guarantees deterministic output.
func mapping() *node { return &node{kind: mapNode} }

// set appends a key/value to a mapping in authored order.
func (n *node) set(key string, val *node) *node {
	n.keys = append(n.keys, key)
	n.vals = append(n.vals, val)
	return n
}

// setStr is set with a scalar string value, skipped when the value is empty so
// optional fields do not render as empty keys.
func (n *node) setStr(key, val string) *node {
	if val == "" {
		return n
	}
	return n.set(key, scalar(val))
}

// sequence builds a sequence node.
func sequence(items ...*node) *node { return &node{kind: seqNode, items: items} }

// render serializes the node to deterministic YAML at the given indent depth.
func (n *node) render(b *strings.Builder, indent int) {
	switch n.kind {
	case scalarNode:
		b.WriteString(quoteScalar(n.str))
		b.WriteByte('\n')
	case rawScalarNode:
		b.WriteString(n.str)
		b.WriteByte('\n')
	case mapNode:
		for i, k := range n.keys {
			v := n.vals[i]
			writeIndent(b, indent)
			b.WriteString(k)
			b.WriteByte(':')
			switch v.kind {
			case scalarNode, rawScalarNode:
				b.WriteByte(' ')
				v.render(b, 0)
			case mapNode:
				if len(v.keys) == 0 {
					b.WriteString(" {}\n")
				} else {
					b.WriteByte('\n')
					v.render(b, indent+1)
				}
			case seqNode:
				if len(v.items) == 0 {
					b.WriteString(" []\n")
				} else {
					b.WriteByte('\n')
					v.render(b, indent)
				}
			}
		}
	case seqNode:
		for _, it := range n.items {
			writeIndent(b, indent)
			b.WriteString("- ")
			switch it.kind {
			case scalarNode, rawScalarNode:
				it.render(b, 0)
			case mapNode:
				// Inline the first map key on the "- " line, the rest indented.
				renderSeqMap(b, it, indent)
			case seqNode:
				it.render(b, indent+1)
			}
		}
	}
}

// renderSeqMap renders a map that is a sequence item: the first key sits on the
// "- " dash line, subsequent keys align under it.
func renderSeqMap(b *strings.Builder, m *node, indent int) {
	for i, k := range m.keys {
		v := m.vals[i]
		if i > 0 {
			writeIndent(b, indent+1)
		}
		b.WriteString(k)
		b.WriteByte(':')
		switch v.kind {
		case scalarNode, rawScalarNode:
			b.WriteByte(' ')
			v.render(b, 0)
		case mapNode:
			if len(v.keys) == 0 {
				b.WriteString(" {}\n")
			} else {
				b.WriteByte('\n')
				v.render(b, indent+2)
			}
		case seqNode:
			if len(v.items) == 0 {
				b.WriteString(" []\n")
			} else {
				b.WriteByte('\n')
				v.render(b, indent+1)
			}
		}
	}
}

func writeIndent(b *strings.Builder, indent int) {
	for i := 0; i < indent; i++ {
		b.WriteString("  ")
	}
}

// quoteScalar quotes a scalar only when needed for unambiguous YAML, so common
// values stay bare for readable, diff-friendly output. The rule is conservative
// and deterministic.
func quoteScalar(s string) string {
	if s == "" {
		return `""`
	}
	if needsQuote(s) {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

func needsQuote(s string) bool {
	switch s {
	case "true", "false", "null", "yes", "no", "on", "off", "~":
		return true
	}
	// Leading/trailing space, or a character that would change YAML meaning.
	if s != strings.TrimSpace(s) {
		return true
	}
	// A colon is only ambiguous when followed by a space (flow-mapping syntax);
	// "ghcr.io/acme/api:1.4.0" is a valid bare scalar, "a: b" is not.
	if strings.Contains(s, ": ") || strings.HasSuffix(s, ":") {
		return true
	}
	for _, r := range s {
		switch r {
		case '#', '{', '}', '[', ']', ',', '&', '*', '!', '|', '>', '\'', '"', '%', '@', '`', '\n', '\t':
			return true
		}
	}
	// A leading character that would start a YAML indicator.
	switch s[0] {
	case '-', '?', ' ':
		return true
	}
	// A bare number-looking string (int or float) is quoted so it stays a string.
	if _, err := strconv.Atoi(s); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	return false
}

// toYAML renders a document node to a complete YAML document with a trailing
// newline.
func toYAML(doc *node) []byte {
	var b strings.Builder
	doc.render(&b, 0)
	return []byte(b.String())
}
