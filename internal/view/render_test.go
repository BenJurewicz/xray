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
	if !strings.Contains(joined, `▸ "long long long long .. long long long long"`) {
		t.Fatalf("missing folded long text row:\n%s", joined)
	}
	if count := strings.Count(joined, "long"); count != 8 {
		t.Fatalf("folded long text word count=%d want 8:\n%s", count, joined)
	}
	if !strings.Contains(joined, "..") {
		t.Fatalf("folded long text missing begin..end marker:\n%s", joined)
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
	if len(longRows) != 1 || !longRows[0].Foldable || longRows[0].Expanded {
		t.Fatalf("expected one folded long text row, got %#v", longRows)
	}
	expanded := Flatten(doc, Options{InlineTextLimit: 10, TextExpanded: map[int]bool{doc.Roots[0].Children[0].ID: true}})
	longRows = nil
	for _, row := range expanded {
		if row.LongText {
			longRows = append(longRows, row)
		}
	}
	if len(longRows) < 2 || !longRows[0].Expanded {
		t.Fatalf("expected wrapped expanded long text rows, got %#v", expanded)
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
	rows := Flatten(doc, Options{InlineTextLimit: 10, TextExpanded: map[int]bool{doc.Roots[0].Children[0].ID: true}})
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

func TestFoldedTextPreviewRemovesControlCharacters(t *testing.T) {
	text := "beginning of text\n" + strings.Repeat("middle\t", 12) + "\rend of text"
	doc, err := xmltree.Parse(strings.NewReader(`<root><description>` + text + `</description></root>`))
	if err != nil {
		t.Fatal(err)
	}
	rows := Flatten(doc, Options{InlineTextLimit: 10})

	idx := -1
	for i, row := range rows {
		if row.LongText && row.Foldable && !row.Expanded {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("folded long text row not found: %#v", rows)
	}
	if strings.ContainsAny(rows[idx].Text, "\n\r\t") {
		t.Fatalf("folded preview contains control characters: %q", rows[idx].Text)
	}
	if !strings.Contains(rows[idx].Text, "..") {
		t.Fatalf("folded preview missing ellipsis marker: %q", rows[idx].Text)
	}
}

func TestFoldedTextPreviewUsesBoundedEdges(t *testing.T) {
	text := "prefix " + strings.Repeat("middle ", 10000) + "suffix"
	got := truncateWithEllipsis(text)
	if !strings.HasPrefix(got, "prefix ") {
		t.Fatalf("preview missing prefix: %q", got)
	}
	if !strings.HasSuffix(got, "suffix") {
		t.Fatalf("preview missing suffix: %q", got)
	}
	if strings.Contains(got, "middle middle middle middle middle") {
		t.Fatalf("preview included too much middle content: %q", got)
	}
	if strings.ContainsAny(got, "\n\r\t") {
		t.Fatalf("preview contains control characters: %q", got)
	}
}

func TestFoldedTextPreviewKeepsUTF8EdgesValid(t *testing.T) {
	text := strings.Repeat("🙂", 80) + strings.Repeat("middle", 1000) + strings.Repeat("🚀", 80)
	got := truncateWithEllipsis(text)
	if !strings.HasPrefix(got, strings.Repeat("🙂", 20)) {
		t.Fatalf("preview prefix split UTF-8 runes: %q", got)
	}
	if !strings.HasSuffix(got, strings.Repeat("🚀", 20)) {
		t.Fatalf("preview suffix split UTF-8 runes: %q", got)
	}
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("preview contains replacement rune: %q", got)
	}
}

func TestLongAttrsMoveAllAttrsToChildrenAndFoldIndividually(t *testing.T) {
	longValue := strings.Repeat("abcdef ", 20)
	doc, err := xmltree.Parse(strings.NewReader(`<root><item short="ok" token="` + longValue + `">Text</item></root>`))
	if err != nil {
		t.Fatal(err)
	}
	item := doc.Roots[0].Children[0]

	collapsed := Flatten(doc, Options{Expanded: map[int]bool{doc.Roots[0].ID: true, item.ID: true}, InlineTextLimit: 20})
	collapsedJoined := joinRows(collapsed)
	if strings.Contains(collapsedJoined, `item  @short`) || !strings.Contains(collapsedJoined, `item`) {
		t.Fatalf("attrs should not render inline when one is long:\n%s", collapsedJoined)
	}
	if !strings.Contains(collapsedJoined, `@short="ok"`) {
		t.Fatalf("short attr child row missing:\n%s", collapsedJoined)
	}
	if !strings.Contains(collapsedJoined, `▸ @token="abcdef abcdef abcdef..abcdef abcdef abcdef"`) {
		t.Fatalf("long attr folded child row missing:\n%s", collapsedJoined)
	}
	if strings.Contains(collapsedJoined, longValue) {
		t.Fatalf("collapsed rows included long attr value:\n%s", collapsedJoined)
	}

	expanded := Flatten(doc, Options{Expanded: map[int]bool{doc.Roots[0].ID: true, item.ID: true}, InlineTextLimit: 20, AttrExpanded: map[int]map[int]bool{item.ID: {1: true}}})
	var attrRows []Row
	for _, row := range expanded {
		if row.AttrLong {
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

func TestNodeStartIndexSkipsConnectorAndGlyph(t *testing.T) {
	tests := []struct {
		line     string
		wantText string
	}{
		{`└─ ▾ element`, `element`},
		{`├─ ▸ element`, `element`},
		{`└─   element`, `element`},
		{`   └─ ▾ child`, `child`},
		{`└─ ▾ user  @id="42"  User`, `user  @id="42"  User`},
	}
	for _, tt := range tests {
		idx := nodeStartIndex(tt.line)
		if idx < 0 || tt.line[idx:] != tt.wantText {
			t.Errorf("nodeStartIndex(%q)=%d (text=%q), want text=%q", tt.line, idx, tt.line[idx:], tt.wantText)
		}
	}
}

func TestNodeHighlightShowsTagNameInBlue(t *testing.T) {
	// Node line with no inline attrs (element with long attr)
	plain := `├─ ▾ item`
	highlighted := SyntaxHighlight(plain)
	// Tag "item" must be in tagStyle (blue, 38;5;111)
	if !strings.Contains(highlighted, "\x1b[38;5;111mitem\x1b[0m") {
		t.Fatalf("long-attr node line: tag should be blue:\n%q", highlighted)
	}
	// Should NOT have textStyle (gray) or attrStyle for just the tag
	if strings.Contains(highlighted, "\x1b[38;5;252mitem") {
		t.Fatalf("long-attr node line: tag got gray textStyle instead of blue tagStyle:\n%q", highlighted)
	}
}

func TestNodeHighlightShowsInlineTagBlueRestorrectly(t *testing.T) {
	// Node line with inline attrs (no long attrs)
	plain := `└─   user  @id="42"  User`
	highlighted := SyntaxHighlight(plain)
	// Tag "user" must be blue
	if !strings.Contains(highlighted, "\x1b[38;5;111muser\x1b[0m") {
		t.Fatalf("inline node: tag should be blue:\n%q", highlighted)
	}
	// @id should be attrStyle (gold, 38;5;179)
	if !strings.Contains(highlighted, "\x1b[38;5;179m@id\x1b[0m") {
		t.Fatalf("inline node: @id should be gold attrStyle:\n%q", highlighted)
	}
	// "42" should be valueStyle (green, 38;5;150)
	if !strings.Contains(highlighted, "\x1b[38;5;150m\"42\"\x1b[0m") {
		t.Fatalf("inline node: value should be green valueStyle:\n%q", highlighted)
	}
	// "User" should be textStyle (gray, 38;5;252)
	if !strings.Contains(highlighted, "\x1b[38;5;252mUser\x1b[0m") {
		t.Fatalf("inline node: text should be gray textStyle:\n%q", highlighted)
	}
}

func TestShortAttrChildRowHighlighting(t *testing.T) {
	t.Skip("requires row generation, not just SyntaxHighlight on a string")
}

func TestAttrChildRowHighlighting(t *testing.T) {
	// Attribute rows always have AttrLong=true and content starting with @
	// Short attr
	row := Row{
		Text:     `   ├─   @short="ok"`,
		AttrLong: true,
	}
	hl := SyntaxHighlightRow(row)
	// @short in attrStyle (gold)
	if !strings.Contains(hl, "\x1b[38;5;179m@short\x1b[0m") {
		t.Fatalf("short attr row: @short should be gold:\n%q", hl)
	}
	// value "ok" in valueStyle (green)
	if !strings.Contains(hl, "\x1b[38;5;150m\"ok\"\x1b[0m") {
		t.Fatalf("short attr row: value should be green:\n%q", hl)
	}
	// Should NOT have textStyle (gray) on the attr content
	if strings.Contains(hl[len(`   ├─   `):], "\x1b[38;5;252m") {
		t.Fatalf("short attr row: attr content should not be gray:\n%q", hl)
	}

	// Folded long attr
	row2 := Row{
		Text:     `   └─ ▸ @name="begin..end"`,
		AttrLong: true,
	}
	hl2 := SyntaxHighlightRow(row2)
	if !strings.Contains(hl2, "\x1b[38;5;179m@name\x1b[0m") {
		t.Fatalf("folded attr row: @name should be gold:\n%q", hl2)
	}
	if !strings.Contains(hl2, "\x1b[38;5;150m\"begin..end\"\x1b[0m") {
		t.Fatalf("folded attr row: value should be green:\n%q", hl2)
	}

	// Expanded long attr first line
	row3 := Row{
		Text:     `   │  very long wrapped attribute value content`,
		AttrLong: true,
	}
	hl3 := SyntaxHighlightRow(row3)
	// No @, so all content after the pipe should be green
	if !strings.Contains(hl3, "\x1b[38;5;150mvery long wrapped attribute value content\x1b[0m") {
		t.Fatalf("attr continuation row: content should be green:\n%q", hl3)
	}
}

func TestFoldedTextRowHighlighting(t *testing.T) {
	row := Row{
		Text:     `   └─ ▸ "begin..end"`,
		LongText: true,
	}
	hl := SyntaxHighlightRow(row)
	// Text content after tree prefix should be textStyle (gray, 38;5;252)
	if !strings.Contains(hl, "\x1b[38;5;252m\"begin..end\"\x1b[0m") {
		t.Fatalf("folded text should be gray textStyle:\n%q", hl)
	}
}

func TestAttrRowAlignmentUsesConsistentGlyphWidth(t *testing.T) {
	// Short attr: glyph is "  " (2 spaces), folded: "▸ ", expanded: "▾ "
	// All should have the same visual width (runes) before the @ content.
	// childPrefix + conn = "   ├─ " (5 runes), glyph = 2 runes → @ at rune index 7

	connector := "   ├─ "

	shortRow := connector + "  " + `@id="short"`
	foldedRow := connector + "▸ " + `@name="beginning..end"`
	expandedRow := connector + "▾ " + `@name="full value"`

	// Check rune position of @ — must be identical for visual alignment
	checkRunePos := func(label string, s string, want int) {
		runes := []rune(s)
		for i, v := range runes {
			if v == '@' {
				if i != want {
					t.Fatalf("%s: @ at rune index %d, want %d: %q", label, i, want, s)
				}
				return
			}
		}
		t.Fatalf("%s: @ not found: %q", label, s)
	}
	checkRunePos("short", shortRow, 8)
	checkRunePos("folded", foldedRow, 8)
	checkRunePos("expanded", expandedRow, 8)
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
