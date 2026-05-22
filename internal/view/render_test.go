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
