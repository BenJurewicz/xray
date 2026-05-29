package view

import (
	"fmt"
	"regexp"
	"strings"

	"xray/internal/xmltree"
)

// Row is one rendered tree line.
type Row struct {
	NodeID   int
	AttrIdx  int // >= 0 for attr rows, -1 otherwise
	Text     string
	Match    bool
	LongText bool
	AttrLong bool
	Foldable bool
	Expanded bool
}

// Options controls tree flattening.
type Options struct {
	Expanded        map[int]bool
	Matches         map[int]bool
	InlineTextLimit int
	AttrExpanded    map[int]map[int]bool
	TextExpanded    map[int]bool
	WrapWidth       int // terminal width in columns, 0 = default 120
}

// Flatten renders the XML document to navigable rows.
func Flatten(doc *xmltree.Document, opts Options) []Row {
	limit := opts.InlineTextLimit
	if limit <= 0 {
		limit = xmltree.DefaultInlineTextLimit
	}
	wrapWidth := opts.WrapWidth
	if wrapWidth <= 0 {
		wrapWidth = 120
	}
	var rows []Row
	for _, w := range doc.Warnings {
		rows = append(rows, Row{AttrIdx: -1, Text: "⚠ " + w})
	}
	for i, root := range doc.Roots {
		last := i == len(doc.Roots)-1
		flattenNode(&rows, root, "", last, opts, limit, wrapWidth)
	}
	if len(rows) == 0 {
		rows = append(rows, Row{AttrIdx: -1, Text: "No XML elements found."})
	}
	return rows
}

func flattenNode(rows *[]Row, n *xmltree.Node, prefix string, last bool, opts Options, limit int, wrapWidth int) bool {
	if opts.Matches != nil && !subtreeVisible(n, opts.Matches) {
		return false
	}

	connector := "└─ "
	childPrefix := prefix + "   "
	if !last {
		connector = "├─ "
		childPrefix = prefix + "│  "
	}

	nodeHasLongAttr := hasLongAttr(n, limit)
	expanded := opts.Expanded == nil || opts.Expanded[n.ID]
	hasKids := len(n.Children) > 0 || len(n.Text) > limit || nodeHasLongAttr || n.Warning != ""
	glyph := "  "
	if hasKids {
		if expanded {
			glyph = "▾ "
		} else {
			glyph = "▸ "
		}
	}
	line := prefix + connector + glyph + formatNode(n, limit, nodeHasLongAttr)
	*rows = append(*rows, Row{NodeID: n.ID, AttrIdx: -1, Text: line, Match: opts.Matches != nil && opts.Matches[n.ID]})

	if !expanded {
		return true
	}

	childRows := childItems(n, limit, opts)
	for i, item := range childRows {
		isLast := i == len(childRows)-1
		conn := "├─ "
		if isLast {
			conn = "└─ "
		}
		// Compute wrapping width for this child item.
		childDepth := len([]rune(childPrefix)) // rune count of child prefix = columns
		availWidth := wrapWidth - childDepth - 3 - 2 // -3 for conn, -2 for 2-rune glyph
		if availWidth < 20 {
			availWidth = 20
		}
		switch item.kind {
		case itemAttr:
			// Always use a 2-char glyph so all attr rows start at the same column.
			glyph := "  "
			if item.attrLong {
				if item.attrExpanded {
					glyph = "▾ "
				} else {
					glyph = "▸ "
				}
			}
			// Recompute availWidth using actual glyph
			actualPrefix := childDepth + 3 + len([]rune(glyph))
			if actualPrefix < wrapWidth {
				availWidth = wrapWidth - actualPrefix
			} else {
				availWidth = 20
			}
			textLines := textRows(item.text, availWidth)
			if len(textLines) == 0 {
				break
			}
			*rows = append(*rows, Row{NodeID: n.ID, AttrIdx: item.attrIdx, Text: childPrefix + conn + glyph + textLines[0], Foldable: item.attrLong, Expanded: item.attrExpanded, AttrLong: true})
			for _, line := range textLines[1:] {
				continuationPrefix := childPrefix + "   "
				if !isLast {
					continuationPrefix = childPrefix + "│  "
				}
				*rows = append(*rows, Row{NodeID: n.ID, AttrIdx: item.attrIdx, Text: continuationPrefix + line, AttrLong: true})
			}
		case itemText:
			if !item.textExpanded {
				foldedText := truncateWithEllipsis(item.text)
				line := childPrefix + conn + "▸ \"" + foldedText + "\""
				*rows = append(*rows, Row{NodeID: n.ID, AttrIdx: -1, Text: line, Foldable: true, Expanded: false, LongText: true})
				break
			}

			textLines := textRows(item.text, availWidth)
			if len(textLines) == 0 {
				break
			}
			*rows = append(*rows, Row{NodeID: n.ID, AttrIdx: -1, Text: childPrefix + conn + "▾ \"" + textLines[0], Foldable: true, Expanded: true, LongText: true})
			for _, line := range textLines[1:] {
				continuationPrefix := childPrefix + "   "
				if !isLast {
					continuationPrefix = childPrefix + "│  "
				}
				*rows = append(*rows, Row{NodeID: n.ID, AttrIdx: -1, Text: continuationPrefix + line, LongText: true})
			}
			lastIdx := len(*rows) - 1
			(*rows)[lastIdx].Text += "\""
		case itemWarning:
			*rows = append(*rows, Row{AttrIdx: -1, Text: childPrefix + conn + "⚠ " + item.text})
		case itemNode:
			flattenNode(rows, item.node, childPrefix, isLast, opts, limit, wrapWidth)
		}
	}
	return true
}

func subtreeVisible(n *xmltree.Node, matches map[int]bool) bool {
	if matches[n.ID] {
		return true
	}
	for _, c := range n.Children {
		if subtreeVisible(c, matches) {
			return true
		}
	}
	return false
}

func formatNode(n *xmltree.Node, limit int, hasLongAttr bool) string {
	if hasLongAttr {
		return n.Name
	}
	parts := []string{n.Name}
	for _, a := range n.Attrs {
		parts = append(parts, fmt.Sprintf("@%s=%q", a.Name, a.Value))
	}
	line := strings.Join(parts, "  ")
	if n.Text != "" && len([]rune(n.Text)) <= limit {
		line += "  " + n.Text
	}
	return line
}

type childKind int

const (
	itemNode childKind = iota
	itemText
	itemWarning
	itemAttr
)

type childItem struct {
	kind         childKind
	node         *xmltree.Node
	text         string
	attrIdx      int
	attrExpanded bool
	attrLong     bool
	textExpanded bool
}

func childItems(n *xmltree.Node, limit int, opts Options) []childItem {
	items := []childItem{}
	nodeHasLongAttr := hasLongAttr(n, limit)
	if nodeHasLongAttr {
		for i, a := range n.Attrs {
			expanded := opts.AttrExpanded != nil && opts.AttrExpanded[n.ID] != nil && opts.AttrExpanded[n.ID][i]
			attrLong := len([]rune(a.Value)) > limit
			items = append(items, childItem{kind: itemAttr, text: formatAttr(a, expanded, limit), attrIdx: i, attrExpanded: expanded, attrLong: attrLong})
		}
	}
	if n.Text != "" && len([]rune(n.Text)) > limit {
		textExpanded := opts.TextExpanded != nil && opts.TextExpanded[n.ID]
		items = append(items, childItem{kind: itemText, text: n.Text, textExpanded: textExpanded})
	}
	if n.Warning != "" {
		items = append(items, childItem{kind: itemWarning, text: n.Warning})
	}
	for _, c := range n.Children {
		if opts.Matches == nil || subtreeVisible(c, opts.Matches) {
			items = append(items, childItem{kind: itemNode, node: c})
		}
	}
	return items
}

func formatAttr(a xmltree.Attr, expanded bool, limit int) string {
	val := a.Value
	if expanded || len([]rune(val)) <= limit {
		return fmt.Sprintf("@%s=%q", a.Name, val)
	}
	return fmt.Sprintf("@%s=%q", a.Name, truncateWithEllipsis(val))
}

func truncateWithEllipsis(s string) string {
	r := []rune(s)
	if len(r) <= 20 {
		return string(r) + ".." + string(r)
	}
	if len(r) <= 40 {
		return string(r[:20]) + ".." + string(r[len(r)-20:])
	}
	return string(r[:20]) + ".." + string(r[len(r)-20:])
}

func hasLongAttr(n *xmltree.Node, limit int) bool {
	for _, a := range n.Attrs {
		if len([]rune(a.Value)) > limit {
			return true
		}
	}
	return false
}

func textRows(s string, width int) []string {
	if width <= 0 {
		width = 120
	}

	var rows []string
	for _, segment := range strings.Split(s, "\n") {
		rows = append(rows, wrapTextSegment(segment, width)...)
	}
	return rows
}

func wrapTextSegment(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}

	var rows []string
	line := words[0]
	for _, word := range words[1:] {
		if len([]rune(line))+1+len([]rune(word)) > width {
			rows = append(rows, line)
			line = word
			continue
		}
		line += " " + word
	}
	rows = append(rows, line)
	return rows
}

// SyntaxHighlight adds lightweight XML-aware styling to a rendered row while
// preserving the original text content for terminal selection and tests.
func SyntaxHighlight(line string) string {
	return SyntaxHighlightRow(Row{Text: line})
}

func SyntaxHighlightRow(row Row) string {
	line := row.Text
	if strings.TrimSpace(line) == "" || strings.Contains(line, "⚠") {
		return line
	}

	if row.AttrLong {
		// Attribute row: find @name=value and style properly.
		// First line: "   ├─   @name=\"value\"" or "   ├─ ▸ @name=\"val\""
		// Continuation: "   │  wrapped value text" (no @)
		if atIdx := strings.Index(line, "@"); atIdx >= 0 {
			return line[:atIdx] + highlightAttrFolded(line[atIdx:])
		}
		// Continuation line — content follows the tree prefix "│  ".
		marker := "│  "
		if idx := strings.LastIndex(line, marker); idx >= 0 {
			return line[:idx+len(marker)] + valueStyle.Render(line[idx+len(marker):])
		}
		return valueStyle.Render(line)
	}

	if row.LongText {
		return highlightLongText(line)
	}

	// Node line: tag name, optional inline attrs/text.
	idx := nodeStartIndex(line)
	if idx < 0 || idx >= len(line) {
		return line
	}
	return line[:idx] + highlightNodeText(line[idx:])
}

func highlightAttrFolded(s string) string {
	match := attrPattern.FindStringSubmatch(s)
	if match != nil {
		return attrStyle.Render(match[1]) + dimStyle.Render(match[2]) + valueStyle.Render(match[3])
	}
	if eq := strings.Index(s, "="); eq >= 0 {
		return attrStyle.Render(s[:eq]) + dimStyle.Render(s[eq:eq+1]) + valueStyle.Render(s[eq+1:])
	}
	return attrStyle.Render(s)
}

func highlightLongText(line string) string {
	idx := nodeStartIndex(line)
	if idx < 0 || idx >= len(line) {
		return textStyle.Render(line)
	}
	return line[:idx] + textStyle.Render(line[idx:])
}

func nodeStartIndex(line string) int {
	// Node line format: <prefix><connector><glyph><content>
	// Connector is "├─ " or "└─ " (3 runes).
	// Glyph is "▾ ", "▸ ", or "  " (always 2 runes on node lines).
	// Return the byte offset right after the glyph.
	runes := []rune(line)
	for _, conn := range []string{"└─ ", "├─ "} {
		cr := []rune(conn)
		// Search backwards through runes for the connector sequence
		for i := len(runes) - len(cr); i >= 0; i-- {
			match := true
			for j := range cr {
				if runes[i+j] != cr[j] {
					match = false
					break
				}
			}
			if match {
				after := i + len(cr) + 2 // skip connector (3) + glyph (2)
				if after > len(runes) {
					after = len(runes)
				}
				return len(string(runes[:after]))
			}
		}
	}
	return -1
}

func highlightNodeText(s string) string {
	fields := strings.SplitN(s, "  ", 2)
	head := tagStyle.Render(fields[0])
	if len(fields) == 1 {
		return head
	}
	return head + "  " + highlightRest(fields[1])
}

func highlightRest(s string) string {
	parts := strings.Split(s, "  ")
	for i, part := range parts {
		if strings.HasPrefix(part, "@") {
			parts[i] = highlightAttr(part)
			continue
		}
		parts[i] = textStyle.Render(part)
	}
	return strings.Join(parts, "  ")
}

var attrPattern = regexp.MustCompile(`^(@[^=]+)(=)(".*")$`)

func highlightAttr(s string) string {
	match := attrPattern.FindStringSubmatch(s)
	if match == nil {
		return attrStyle.Render(s)
	}
	return attrStyle.Render(match[1]) + dimStyle.Render(match[2]) + valueStyle.Render(match[3])
}

var (
	tagStyle   = ansiStyle("111")
	attrStyle  = ansiStyle("179")
	valueStyle = ansiStyle("150")
	textStyle  = ansiStyle("252")
	dimStyle   = ansiStyle("245")
)

type terminalStyle func(string) string

func (s terminalStyle) Render(v string) string { return s(v) }

func ansiStyle(color string) terminalStyle {
	return func(v string) string {
		if v == "" {
			return v
		}
		return "\x1b[38;5;" + color + "m" + v + "\x1b[0m"
	}
}
