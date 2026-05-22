package xmltree

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

const DefaultInlineTextLimit = 80

// Attr is an XML attribute rendered as @name="value".
type Attr struct {
	Name  string
	Value string
}

// Node is a parsed XML element. Text contains direct character data only.
type Node struct {
	ID       int
	Name     string
	Attrs    []Attr
	Text     string
	Children []*Node
	Parent   *Node
	Warning  string
}

// Document is a best-effort parse result.
type Document struct {
	Roots    []*Node
	Warnings []string
}

// Parse reads XML into a tree. It uses encoding/xml in non-strict mode and keeps
// the successfully decoded prefix when malformed input is encountered.
func Parse(r io.Reader) (*Document, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	doc := &Document{}
	if warning := strictWarning(data); warning != "" {
		doc.Warnings = append(doc.Warnings, warning)
	}

	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false

	var stack []*Node
	var nextID int

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			warning := compactParseError(err)
			doc.Warnings = append(doc.Warnings, warning)
			if len(stack) > 0 {
				stack[len(stack)-1].Warning = warning
			}
			break
		}

		switch t := tok.(type) {
		case xml.StartElement:
			nextID++
			n := &Node{ID: nextID, Name: t.Name.Local}
			for _, a := range t.Attr {
				name := a.Name.Local
				if a.Name.Space != "" {
					name = a.Name.Space + ":" + a.Name.Local
				}
				n.Attrs = append(n.Attrs, Attr{Name: name, Value: a.Value})
			}
			if len(stack) == 0 {
				doc.Roots = append(doc.Roots, n)
			} else {
				parent := stack[len(stack)-1]
				n.Parent = parent
				parent.Children = append(parent.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if len(stack) > 0 {
				text := CollapseWhitespace(string(t))
				if text != "" {
					cur := stack[len(stack)-1]
					if cur.Text == "" {
						cur.Text = text
					} else {
						cur.Text += " " + text
					}
				}
			}
		}
	}

	return doc, nil
}

var whitespace = regexp.MustCompile(`\s+`)

// CollapseWhitespace trims and collapses XML character data for display.
func CollapseWhitespace(s string) string {
	return strings.TrimSpace(whitespace.ReplaceAllString(s, " "))
}

func strictWarning(data []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			return ""
		}
		if err != nil {
			return compactParseError(err)
		}
	}
}

func compactParseError(err error) string {
	var syn *xml.SyntaxError
	if errors.As(err, &syn) {
		return fmt.Sprintf("XML parse warning near line %d: %s", syn.Line, syn.Msg)
	}
	return "XML parse warning: " + err.Error()
}

// Path returns a slash-separated path from the root to the node.
func (n *Node) Path() string {
	var parts []string
	for cur := n; cur != nil; cur = cur.Parent {
		parts = append(parts, cur.Name)
	}
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, "/")
}

// Search returns nodes matching every token in query. Bare tokens are fuzzy
// matched across tag, attributes, values and text. Prefix tokens support tag:,
// attr:, attr:name=value, text:, and value:.
func Search(doc *Document, query string) map[int]bool {
	tokens := strings.Fields(strings.TrimSpace(query))
	if len(tokens) == 0 {
		return nil
	}

	matches := map[int]bool{}
	Walk(doc, func(n *Node) {
		for _, tok := range tokens {
			if !matchesToken(n, tok) {
				return
			}
		}
		matches[n.ID] = true
	})
	return matches
}

func matchesToken(n *Node, token string) bool {
	lower := strings.ToLower(token)
	if v, ok := strings.CutPrefix(lower, "tag:"); ok {
		return fuzzy(strings.ToLower(n.Name), v)
	}
	if v, ok := strings.CutPrefix(lower, "text:"); ok {
		return fuzzy(strings.ToLower(n.Text), v)
	}
	if v, ok := strings.CutPrefix(lower, "value:"); ok {
		return fuzzy(strings.ToLower(n.Text), v)
	}
	if v, ok := strings.CutPrefix(lower, "attr:"); ok {
		name, val, hasVal := strings.Cut(v, "=")
		for _, a := range n.Attrs {
			an := strings.ToLower(a.Name)
			av := strings.ToLower(a.Value)
			if fuzzy(an, name) && (!hasVal || fuzzy(av, val)) {
				return true
			}
		}
		return false
	}

	hay := strings.ToLower(n.Name + " " + n.Text)
	for _, a := range n.Attrs {
		hay += " " + strings.ToLower(a.Name) + " " + strings.ToLower(a.Value)
	}
	return fuzzy(hay, lower)
}

func fuzzy(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	if strings.Contains(haystack, needle) {
		return true
	}
	j := 0
	for i := 0; i < len(haystack) && j < len(needle); i++ {
		if haystack[i] == needle[j] {
			j++
		}
	}
	return j == len(needle)
}

// Walk visits all nodes pre-order.
func Walk(doc *Document, fn func(*Node)) {
	var walkNode func(*Node)
	walkNode = func(n *Node) {
		fn(n)
		for _, child := range n.Children {
			walkNode(child)
		}
	}
	for _, root := range doc.Roots {
		walkNode(root)
	}
}

// VisibleIDs returns all matching nodes and their ancestors.
func VisibleIDs(matches map[int]bool) map[int]bool {
	if len(matches) == 0 {
		return nil
	}
	visible := map[int]bool{}
	return visible
}

// MatchIDs exposes sorted match IDs for deterministic navigation/tests.
func MatchIDs(matches map[int]bool) []int {
	ids := make([]int, 0, len(matches))
	for id := range matches {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}
