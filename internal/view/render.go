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
	Text     string
	Match    bool
	LongText bool
	LongAttr bool
}

// Options controls tree flattening.
type Options struct {
	Expanded        map[int]bool
	Matches         map[int]bool
	InlineTextLimit int
}

// Flatten renders the XML document to navigable rows.
func Flatten(doc *xmltree.Document, opts Options) []Row {
	limit := opts.InlineTextLimit
	if limit <= 0 {
		limit = xmltree.DefaultInlineTextLimit
	}
	var rows []Row
	for _, w := range doc.Warnings {
		rows = append(rows, Row{Text: "⚠ " + w})
	}
	for i, root := range doc.Roots {
		last := i == len(doc.Roots)-1
		flattenNode(&rows, root, "", last, opts, limit)
	}
	if len(rows) == 0 {
		rows = append(rows, Row{Text: "No XML elements found."})
	}
	return rows
}

func flattenNode(rows *[]Row, n *xmltree.Node, prefix string, last bool, opts Options, limit int) bool {
	if opts.Matches != nil && !subtreeVisible(n, opts.Matches) {
		return false
	}

	connector := "└─ "
	childPrefix := prefix + "   "
	if !last {
		connector = "├─ "
		childPrefix = prefix + "│  "
	}

	expanded := opts.Expanded == nil || opts.Expanded[n.ID]
	hasKids := len(n.Children) > 0 || len(n.Text) > limit || hasLongAttr(n, limit) || n.Warning != ""
	glyph := "  "
	if hasKids {
		if expanded {
			glyph = "▾ "
		} else {
			glyph = "▸ "
		}
	}
	line := prefix + connector + glyph + formatNode(n, limit)
	*rows = append(*rows, Row{NodeID: n.ID, Text: line, Match: opts.Matches != nil && opts.Matches[n.ID]})

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
		switch item.kind {
		case itemAttrLong:
			attrLines := textRows(item.text, 120)
			if len(attrLines) == 0 {
				break
			}
			*rows = append(*rows, Row{Text: childPrefix + conn + "@" + item.attrName + `="` + attrLines[0], LongAttr: true})
			for _, line := range attrLines[1:] {
				continuationPrefix := childPrefix + "   "
				if !isLast {
					continuationPrefix = childPrefix + "│  "
				}
				*rows = append(*rows, Row{Text: continuationPrefix + line, LongAttr: true})
			}
			lastIdx := len(*rows) - 1
			(*rows)[lastIdx].Text += `"`
		case itemText:
			textLines := textRows(item.text, 120)
			if len(textLines) == 0 {
				break
			}
			*rows = append(*rows, Row{Text: childPrefix + conn + "“" + textLines[0], LongText: true})
			for _, line := range textLines[1:] {
				continuationPrefix := childPrefix + "   "
				if !isLast {
					continuationPrefix = childPrefix + "│  "
				}
				*rows = append(*rows, Row{Text: continuationPrefix + line, LongText: true})
			}
			lastIdx := len(*rows) - 1
			(*rows)[lastIdx].Text += "”"
		case itemWarning:
			*rows = append(*rows, Row{Text: childPrefix + conn + "⚠ " + item.text})
		case itemNode:
			flattenNode(rows, item.node, childPrefix, isLast, opts, limit)
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

func formatNode(n *xmltree.Node, limit int) string {
	parts := []string{n.Name}
	for _, a := range n.Attrs {
		if len([]rune(a.Value)) > limit {
			parts = append(parts, fmt.Sprintf("@%s=…", a.Name))
			continue
		}
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
	itemAttrLong
	itemText
	itemWarning
)

type childItem struct {
	kind     childKind
	node     *xmltree.Node
	text     string
	attrName string
}

func childItems(n *xmltree.Node, limit int, opts Options) []childItem {
	items := []childItem{}
	for _, a := range n.Attrs {
		if len([]rune(a.Value)) > limit {
			items = append(items, childItem{kind: itemAttrLong, attrName: a.Name, text: a.Value})
		}
	}
	if n.Text != "" && len([]rune(n.Text)) > limit {
		items = append(items, childItem{kind: itemText, text: n.Text})
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

func hasLongAttr(n *xmltree.Node, limit int) bool {
	for _, a := range n.Attrs {
		if len([]rune(a.Value)) > limit {
			return true
		}
	}
	return false
}

func textRows(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	if width <= 0 {
		width = 120
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
	if row.LongText {
		return highlightLongText(line)
	}
	if row.LongAttr {
		return highlightLongAttr(line)
	}
	idx := nodeStartIndex(line)
	if idx < 0 || idx >= len(line) {
		return line
	}
	return line[:idx] + highlightNodeText(line[idx:])
}

func highlightLongAttr(line string) string {
	idx := nodeStartIndex(line)
	if idx < 0 || idx >= len(line) {
		return valueStyle.Render(line)
	}
	return line[:idx] + valueStyle.Render(line[idx:])
}

func highlightLongText(line string) string {
	idx := nodeStartIndex(line)
	if idx < 0 || idx >= len(line) {
		return textStyle.Render(line)
	}
	return line[:idx] + textStyle.Render(line[idx:])
}

func nodeStartIndex(line string) int {
	for _, marker := range []string{"▾ ", "▸ "} {
		if idx := strings.Index(line, marker); idx >= 0 {
			return idx + len(marker)
		}
	}
	if idx := strings.LastIndex(line, "─ "); idx >= 0 {
		return idx + len("─ ")
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
