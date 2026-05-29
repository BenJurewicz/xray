package view

import (
	"regexp"
	"strings"
	"testing"

	"xray/internal/xmltree"
)

func TestFlattenRendersAttrsTextAndLongText(t *testing.T) {
	longText := strings.TrimSpace(strings.Repeat("long ", 30))
	doc, err := xmltree.Parse(strings.NewReader(`<root><user id="42">User</user><note>` + longText + `</note></root>`))
	if err != nil {
		t.Fatal(err)
	}
	rows := Flatten(doc, Options{InlineTextLimit: 80})
	joined := joinRows(rows)
	if !strings.Contains(joined, `user  @id="42"  User`) {
		t.Fatalf("missing attr/text row:\n%s", joined)
	}
	if !strings.Contains(joined, "“long long") {
		t.Fatalf("missing long text continuation:\n%s", joined)
	}
	if count := strings.Count(joined, "long"); count != 30 {
		t.Fatalf("long text word count=%d want 30:\n%s", count, joined)
	}
	if strings.Contains(joined, "long…") {
		t.Fatalf("long text contains truncation marker:\n%s", joined)
	}
}

func TestTextRowsWrapWithoutTruncating(t *testing.T) {
	text := "one two three four five six seven"
	rows := textRows(text, 13)
	if got := strings.Join(rows, " "); got != text {
		t.Fatalf("wrapped text changed:\n got %q\nwant %q", got, text)
	}
	if len(rows) < 2 {
		t.Fatalf("expected wrapping, got %#v", rows)
	}
}

func TestTextRowsPreserveNewlinesAndBlankLines(t *testing.T) {
	text := "alpha beta\n\ngamma delta epsilon"
	rows := textRows(text, 12)
	want := []string{"alpha beta", "", "gamma delta", "epsilon"}
	if strings.Join(rows, "|") != strings.Join(want, "|") {
		t.Fatalf("rows=%#v want %#v", rows, want)
	}
}

func TestLongTextRowsUseConsistentTextHighlighting(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root><summary>` + strings.Repeat("word ", 60) + `</summary></root>`))
	if err != nil {
		t.Fatal(err)
	}
	rows := Flatten(doc, Options{InlineTextLimit: 10})
	var longRows []Row
	for _, row := range rows {
		if row.LongText {
			longRows = append(longRows, row)
		}
	}
	if len(longRows) < 2 {
		t.Fatalf("expected wrapped long text rows, got %#v", rows)
	}
	for _, row := range longRows {
		highlighted := SyntaxHighlightRow(row)
		if !strings.Contains(highlighted, "\x1b[38;5;252m") {
			t.Fatalf("long text row missing text color: %q", highlighted)
		}
		if strings.Contains(highlighted, "\x1b[38;5;111m") {
			t.Fatalf("long text row was styled as tag: %q", highlighted)
		}
	}
}

func TestFlattenRendersMultilineTextAsSeparateRows(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root><description>
		First line with enough words to be long.

		Second line stays separate from the first line.
	</description></root>`))
	if err != nil {
		t.Fatal(err)
	}
	rows := Flatten(doc, Options{InlineTextLimit: 10})
	joined := joinRows(rows)
	if !strings.Contains(joined, "First line with enough words") || !strings.Contains(joined, "Second line stays separate") {
		t.Fatalf("missing multiline text rows:\n%s", joined)
	}
	if strings.Contains(joined, "long. Second") {
		t.Fatalf("newline was collapsed between text lines:\n%s", joined)
	}
	foundBlankLine := false
	for _, row := range rows {
		if row.LongText && strings.TrimSpace(row.Text) == "" {
			foundBlankLine = true
			break
		}
	}
	if !foundBlankLine {
		t.Fatalf("interior blank line was not preserved:\n%s", joined)
	}
}

func TestLongAttrsFoldInlineAndExpandAsValueRows(t *testing.T) {
	longValue := strings.Repeat("abcdef ", 20)
	doc, err := xmltree.Parse(strings.NewReader(`<root><item short="ok" token="` + longValue + `">Text</item></root>`))
	if err != nil {
		t.Fatal(err)
	}
	item := doc.Roots[0].Children[0]

	collapsed := Flatten(doc, Options{Expanded: map[int]bool{doc.Roots[0].ID: true}, InlineTextLimit: 20})
	collapsedJoined := joinRows(collapsed)
	if !strings.Contains(collapsedJoined, `item  @short="ok"  @token=…  Text`) {
		t.Fatalf("long attr was not folded inline:\n%s", collapsedJoined)
	}
	if strings.Contains(collapsedJoined, longValue) {
		t.Fatalf("collapsed rows included long attr value:\n%s", collapsedJoined)
	}

	expanded := Flatten(doc, Options{Expanded: map[int]bool{doc.Roots[0].ID: true, item.ID: true}, InlineTextLimit: 20})
	var attrRows []Row
	for _, row := range expanded {
		if row.LongAttr {
			attrRows = append(attrRows, row)
		}
	}
	if len(attrRows) == 0 {
		t.Fatalf("expected long attr child rows, got %#v", expanded)
	}
	if !strings.Contains(joinRows(attrRows), `@token="abcdef`) {
		t.Fatalf("missing attr child row:\n%s", joinRows(attrRows))
	}
	for _, row := range attrRows {
		highlighted := SyntaxHighlightRow(row)
		if !strings.Contains(highlighted, "\x1b[38;5;150m") {
			t.Fatalf("long attr row missing value color: %q", highlighted)
		}
		if strings.Contains(highlighted, "\x1b[38;5;252m") {
			t.Fatalf("long attr row styled as regular long text: %q", highlighted)
		}
	}
}

func TestFlattenFiltersWithAncestors(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<rss><channel><item><title>Foo</title></item><item><title>Bar</title></item></channel></rss>`))
	if err != nil {
		t.Fatal(err)
	}
	matches := xmltree.Search(doc, "text:Foo")
	rows := Flatten(doc, Options{Matches: matches, InlineTextLimit: 80})
	joined := joinRows(rows)
	for _, want := range []string{"rss", "channel", "item", "title  Foo"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Bar") {
		t.Fatalf("filter included non-matching sibling:\n%s", joined)
	}
}

func TestSyntaxHighlightPreservesTextAndAddsStyles(t *testing.T) {
	plain := `└─ ▾ user  @id="42"  User`
	highlighted := SyntaxHighlight(plain)
	if highlighted == plain {
		t.Fatalf("expected highlighted output to differ from plain text")
	}
	if got := stripANSI(highlighted); got != plain {
		t.Fatalf("highlight changed visible text:\n got %q\nwant %q", got, plain)
	}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

func joinRows(rows []Row) string {
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.Text)
		b.WriteByte('\n')
	}
	return b.String()
}
