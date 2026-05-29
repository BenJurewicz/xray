package xmltree

import (
	"strings"
	"testing"
)

func TestParseCollapsesWhitespaceAndAttrs(t *testing.T) {
	doc, err := Parse(strings.NewReader(`<user id="42" role="admin">
		<name>
			User
		</name>
	</user>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Roots) != 1 {
		t.Fatalf("roots=%d", len(doc.Roots))
	}
	root := doc.Roots[0]
	if root.Name != "user" || len(root.Attrs) != 2 {
		t.Fatalf("bad root: %#v", root)
	}
	if got := root.Children[0].Text; got != "User" {
		t.Fatalf("text=%q", got)
	}
}

func TestCollapseWhitespacePreservesNewlines(t *testing.T) {
	input := "\n\t  First   line\n\n\tSecond\t line   with   spaces\n  Third line  \n\n"
	want := "First line\n\nSecond line with spaces\nThird line"
	if got := CollapseWhitespace(input); got != want {
		t.Fatalf("CollapseWhitespace()=%q want %q", got, want)
	}
}

func TestSearchPrefixesAndFuzzy(t *testing.T) {
	doc, err := Parse(strings.NewReader(`<invoice id="abc-42"><status>paid</status><customer>User</customer></invoice>`))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]int{
		"tag:invoice":         1,
		"attr:id=42":          1,
		"text:paid":           1,
		"value:User":          1,
		"inv paid":            0,
		"tag:status value:pd": 1,
	}
	for q, want := range cases {
		got := len(Search(doc, q))
		if got != want {
			t.Fatalf("Search(%q)=%d want %d", q, got, want)
		}
	}
}

func TestMalformedXMLProducesWarningAndPartialTree(t *testing.T) {
	doc, err := Parse(strings.NewReader(`<root><child>ok</root>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Roots) != 1 {
		t.Fatalf("expected partial root")
	}
	if len(doc.Warnings) == 0 {
		t.Fatalf("expected warning")
	}
}
