package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"xray/internal/xmltree"
)

func TestViewPinsFooterToBottom(t *testing.T) {
	doc, err := xmltree.Parse(strings.NewReader(`<root><child>ok</child></root>`))
	if err != nil {
		t.Fatal(err)
	}
	m := newModel(doc, "testdata/sample.xml")
	m.width = 80
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
	if !strings.Contains(footer, "  jk/hl nav") {
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
	m.width = 100
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

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlF})
	m = updated.(model)
	if m.offset != 9 || m.selected != 9 {
		t.Fatalf("ctrl-f selected=%d offset=%d want 9/9", m.selected, m.offset)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlB})
	m = updated.(model)
	if m.offset != 0 || m.selected != 0 {
		t.Fatalf("ctrl-b selected=%d offset=%d want 0/0", m.selected, m.offset)
	}

	updated, _ = m.Update(teaKey("G"))
	m = updated.(model)
	if m.selected != len(m.rows())-1 {
		t.Fatalf("G selected=%d want bottom %d", m.selected, len(m.rows())-1)
	}

	updated, _ = m.Update(teaKey("g"))
	m = updated.(model)
	updated, _ = m.Update(teaKey("g"))
	m = updated.(model)
	if m.selected != 0 || m.offset != 0 {
		t.Fatalf("gg selected=%d offset=%d want top", m.selected, m.offset)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = updated.(model)
	if m.offset != 1 {
		t.Fatalf("ctrl-e offset=%d want 1", m.offset)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = updated.(model)
	if m.offset != 0 {
		t.Fatalf("ctrl-y offset=%d want 0", m.offset)
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

func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return lines[len(lines)-1]
}

func teaKey(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
