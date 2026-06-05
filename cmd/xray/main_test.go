package main

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"xray/internal/view"
	"xray/internal/xmltree"
)

func TestViewPinsFooterToBottom(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root><child>ok</child></root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 120
	m.height = 8

	out := m.View()
	lines := strings.Split(out, "\n")
	if len(lines) != m.height {
		t.Fatalf("rendered lines=%d want %d:\n%s", len(lines), m.height, out)
	}
	for i, line := range lines[:len(lines)-1] {
		if lipgloss.Width(line) != m.width {
			t.Fatalf("line %d width=%d want %d: %q", i, lipgloss.Width(line), m.width, line)
		}
	}
	footer := lines[len(lines)-1]
	if !strings.HasPrefix(footer, "testdata/sample.xml") {
		t.Fatalf("footer missing path: %q", footer)
	}
	if !strings.Contains(footer, "q quit") {
		t.Fatalf("footer missing controls: %q", footer)
	}
	if !strings.Contains(footer, "  jk move") {
		t.Fatalf("footer controls are not right-aligned: %q", footer)
	}
	if lipgloss.Width(footer) != m.width {
		t.Fatalf("footer width=%d want %d: %q", lipgloss.Width(footer), m.width, footer)
	}
}

func TestDisplayPathIsRelativeToWorkingDirectory(t *testing.T) {
	got := displayPath("testdata/sample.xml")
	if got != "testdata/sample.xml" {
		t.Fatalf("displayPath=%q", got)
	}
}

func TestSearchFooterShowsPromptAndCheatsheet(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root><child>ok</child></root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 110
	m.height = 8
	m.mode = modeSearch
	m.query = "tag:child"

	footer := lastLine(m.View())
	if !strings.Contains(footer, "/tag:child") {
		t.Fatalf("footer missing search prompt on left: %q", footer)
	}
	for _, want := range []string{"tag:", "attr:", "text:", "value:", "enter apply", "esc cancel"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("footer missing %q: %q", want, footer)
		}
	}
}

func TestClearSearchKeyOnlyShowsAfterSearch(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root><child>ok</child><other>no</other></root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 140
	m.height = 8
	if strings.Contains(lastLine(m.View()), "esc/c clear") {
		t.Fatalf("clear key shown before search: %q", lastLine(m.View()))
	}

	m.query = "tag:child"
	m.applySearch()
	if !strings.Contains(lastLine(m.View()), "esc/c clear") {
		t.Fatalf("clear key hidden after search: %q", lastLine(m.View()))
	}
	if m.matches == nil {
		t.Fatalf("expected active search")
	}

	updated, _ := m.Update(teaKey("c"))
	m = updated.(model)
	if m.matches != nil || m.query != "" || len(m.matchIDs) != 0 {
		t.Fatalf("search not cleared: query=%q matches=%v matchIDs=%v", m.query, m.matches, m.matchIDs)
	}
	if strings.Contains(lastLine(m.View()), "esc/c clear") {
		t.Fatalf("clear key still shown after clearing: %q", lastLine(m.View()))
	}
}

func TestEscClearsActiveSearchFilter(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root><child>ok</child><other>no</other></root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 100
	m.height = 8
	m.query = "tag:child"
	m.applySearch()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if m.matches != nil || m.query != "" || len(m.matchIDs) != 0 {
		t.Fatalf("search not cleared by esc: query=%q matches=%v matchIDs=%v", m.query, m.matches, m.matchIDs)
	}
}

func TestVimNavigationKeys(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root>` + strings.Repeat(`<item>value</item>`, 30) + `</root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 100
	m.height = 10

	updated, _ := m.Update(teaKey("d"))
	m = updated.(model)
	if m.offset != 4 || m.selected != 4 {
		t.Fatalf("d selected=%d offset=%d want 4/4", m.selected, m.offset)
	}

	updated, _ = m.Update(teaKey("u"))
	m = updated.(model)
	if m.offset != 0 || m.selected != 0 {
		t.Fatalf("u selected=%d offset=%d want 0/0", m.selected, m.offset)
	}

	updated, _ = m.Update(teaKey("f"))
	m = updated.(model)
	if m.offset != 9 || m.selected != 9 {
		t.Fatalf("f selected=%d offset=%d want 9/9", m.selected, m.offset)
	}

	updated, _ = m.Update(teaKey("b"))
	m = updated.(model)
	if m.offset != 0 || m.selected != 0 {
		t.Fatalf("b selected=%d offset=%d want 0/0", m.selected, m.offset)
	}

	updated, _ = m.Update(teaKey("G"))
	m = updated.(model)
	if m.selected != len(m.rows())-1 {
		t.Fatalf("G selected=%d want bottom %d", m.selected, len(m.rows())-1)
	}

	updated, _ = m.Update(teaKey("g"))
	m = updated.(model)
	if m.selected != 0 || m.offset != 0 {
		t.Fatalf("g selected=%d offset=%d want top", m.selected, m.offset)
	}

	updated, _ = m.Update(teaKey("e"))
	m = updated.(model)
	if m.offset != 1 {
		t.Fatalf("e offset=%d want 1", m.offset)
	}
	updated, _ = m.Update(teaKey("y"))
	m = updated.(model)
	if m.offset != 0 {
		t.Fatalf("y offset=%d want 0", m.offset)
	}
}

func TestControlNavigationAliasesStillWork(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root>` + strings.Repeat(`<item>value</item>`, 30) + `</root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 100
	m.height = 10

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = updated.(model)
	if m.offset != 4 || m.selected != 4 {
		t.Fatalf("ctrl-d selected=%d offset=%d want 4/4", m.selected, m.offset)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = updated.(model)
	if m.offset != 0 || m.selected != 0 {
		t.Fatalf("ctrl-u selected=%d offset=%d want 0/0", m.selected, m.offset)
	}
}

func TestFoldableAttrAndTextKeysToggleIndependently(t *testing.T) {
	longAttr := strings.Repeat("abcdef ", 12)
	longText := strings.Repeat("word ", 30)
	doc, err := xmltree.Parse(strings.NewReader(`<root><item short="ok" token="` + longAttr + `">` + longText + `</item></root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 160
	m.height = 20

	attrRow := findRowIndex(m.rows(), func(r view.Row) bool { return r.AttrIdx == 1 && r.Foldable && !r.Expanded })
	if attrRow < 0 {
		t.Fatalf("folded attr row not found: %#v", m.rows())
	}
	m.selected = attrRow
	updated, _ := m.Update(teaKey("l"))
	m = updated.(model)
	if !m.attrExpanded[doc.Roots[0].Children[0].ID][1] {
		t.Fatalf("l did not expand selected attr")
	}
	updated, _ = m.Update(teaKey("h"))
	m = updated.(model)
	if m.attrExpanded[doc.Roots[0].Children[0].ID][1] {
		t.Fatalf("h did not fold selected attr")
	}

	textRow := findRowIndex(m.rows(), func(r view.Row) bool { return r.LongText && r.Foldable && !r.Expanded })
	if textRow < 0 {
		t.Fatalf("folded text row not found: %#v", m.rows())
	}
	m.selected = textRow
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !m.textExpanded[doc.Roots[0].Children[0].ID] {
		t.Fatalf("enter did not expand selected text")
	}
}

func TestJumpListBackAndForward(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root>` + strings.Repeat(`<item>value</item>`, 30) + `</root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 100
	m.height = 10

	updated, _ := m.Update(teaKey("G"))
	m = updated.(model)
	bottom := m.selected
	updated, _ = m.Update(teaKey("g"))
	m = updated.(model)
	if m.selected != 0 {
		t.Fatalf("g selected=%d want top", m.selected)
	}

	updated, _ = m.Update(teaKey("o"))
	m = updated.(model)
	if m.selected != bottom {
		t.Fatalf("o selected=%d want previous bottom %d", m.selected, bottom)
	}
	updated, _ = m.Update(teaKey("i"))
	m = updated.(model)
	if m.selected != 0 {
		t.Fatalf("i selected=%d want forward top", m.selected)
	}
}

func TestSearchRecordsJumpAndOBackRestoresPreviousView(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root>` + strings.Repeat(`<other>no</other>`, 20) + `<target>yes</target></root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 100
	m.height = 8
	updated, _ := m.Update(teaKey("G"))
	m = updated.(model)
	previous := m.selected

	updated, _ = m.Update(teaKey("/"))
	m = updated.(model)
	for _, r := range "tag:target" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(model)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if !m.hasSearch() || m.query != "tag:target" {
		t.Fatalf("expected active target search, query=%q", m.query)
	}

	updated, _ = m.Update(teaKey("o"))
	m = updated.(model)
	if m.hasSearch() || m.query != "" {
		t.Fatalf("o should restore pre-search unfiltered view, query=%q", m.query)
	}
	if m.selected != previous {
		t.Fatalf("o selected=%d want previous %d", m.selected, previous)
	}
}

func TestHelpExplainsSearch(t *testing.T) {
	for _, want := range []string{"Search:", "tag:item", "attr:id=42", "value:paid", "Filters compose", "o/i move backward"} {
		if !strings.Contains(helpText, want) {
			t.Fatalf("help missing %q", want)
		}
	}
}

func TestHelpPageIsSyntaxHighlighted(t *testing.T) {
	rendered := renderHelp(helpText)
	if rendered == helpText {
		t.Fatalf("expected highlighted help to differ from plain text")
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("expected ANSI styling in help output")
	}
	if got := stripANSI(rendered); !strings.Contains(got, "tag:item") || !strings.Contains(got, "q            quit") {
		t.Fatalf("highlighted help changed visible content: %q", got)
	}
}

func TestJKScrollsOnlyAtViewportEdges(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root>` + strings.Repeat(`<item>value</item>`, 30) + `</root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 100
	m.height = 6

	for range 3 {
		updated, _ := m.Update(teaKey("j"))
		m = updated.(model)
	}
	if m.selected != 3 || m.offset != 0 {
		t.Fatalf("j moved viewport before bottom edge: selected=%d offset=%d", m.selected, m.offset)
	}

	updated, _ := m.Update(teaKey("j"))
	m = updated.(model)
	if m.selected != 4 || m.offset != 0 {
		t.Fatalf("j should reach bottom edge before scrolling: selected=%d offset=%d", m.selected, m.offset)
	}

	updated, _ = m.Update(teaKey("j"))
	m = updated.(model)
	if m.selected != 5 || m.offset != 1 {
		t.Fatalf("j should scroll one line after bottom edge: selected=%d offset=%d", m.selected, m.offset)
	}

	updated, _ = m.Update(teaKey("k"))
	m = updated.(model)
	if m.selected != 4 || m.offset != 1 {
		t.Fatalf("k moved viewport before top edge: selected=%d offset=%d", m.selected, m.offset)
	}

	updated, _ = m.Update(teaKey("k"))
	m = updated.(model)
	if m.selected != 3 || m.offset != 1 {
		t.Fatalf("k should move selection within viewport: selected=%d offset=%d", m.selected, m.offset)
	}
}

func TestJStopsAtLastRenderedRow(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root>` + strings.Repeat(`<item>value</item>`, 12) + `</root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 100
	m.height = 6

	for range 100 {
		updated, _ := m.Update(teaKey("j"))
		m = updated.(model)
	}

	last := len(m.rows()) - 1
	if m.selected != last {
		t.Fatalf("j should stop at last row: selected=%d want %d", m.selected, last)
	}
	if m.offset > last {
		t.Fatalf("offset moved past last row: offset=%d last=%d", m.offset, last)
	}

	updated, _ := m.Update(teaKey("j"))
	m = updated.(model)
	if m.selected != last {
		t.Fatalf("extra j moved past last row: selected=%d want %d", m.selected, last)
	}
}

func TestCursorClampsWhenRenderedRowsShrink(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root><parent>` + strings.Repeat(`<item>value</item>`, 12) + `</parent></root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 100
	m.height = 6
	m.selected = len(m.rows()) - 1

	parent := doc.Roots[0].Children[0]
	m.expanded[parent.ID] = false
	updated, _ := m.Update(teaKey("j"))
	m = updated.(model)

	last := len(m.rows()) - 1
	if m.selected != last {
		t.Fatalf("selected not clamped after rows shrink: selected=%d want %d", m.selected, last)
	}
	if m.offset > max(0, len(m.rows())-m.contentHeight()) {
		t.Fatalf("offset not clamped after rows shrink: offset=%d rows=%d", m.offset, len(m.rows()))
	}
}

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return lines[len(lines)-1]
}

func findRowIndex(rows []view.Row, pred func(view.Row) bool) int {
	for i, row := range rows {
		if pred(row) {
			return i
		}
	}
	return -1
}

func teaKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
