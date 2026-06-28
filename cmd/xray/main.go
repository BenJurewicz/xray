package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"xray/internal/view"
	"xray/internal/xmltree"
)

const helpText = `xray - interactive XML tree viewer

Usage:
  xray file.xml
  cat file.xml | xray -

Keys:
  ↑/k ↓/j      move
  ←/h →/l      collapse / expand
  H / L        fold all / unfold all
  d/u          half-page down / up
  f/b          page down / up
  e/y          scroll down / up
  g / G        top / bottom
  o / i        jump back / forward
  enter        toggle expand
  /            search
  c            clear search filter
  n / N        next / previous match
  ?            toggle help
  q            quit

Search:
  Bare text searches tags, attribute names, attribute values, and text.
    invoice
    paid

  Prefix filters narrow the search:
    tag:item        match element names
    attr:id         match attribute names
    attr:id=42      match attribute name + value
    text:paid       match text content
    value:paid      alias for text:paid

  Filters compose with spaces, so every token must match:
    tag:item attr:status=paid

  enter applies the search. esc cancels while typing. After a search is
  active, esc or c clears it. n/N move between matches. o/i move backward
  and forward through the jump list, including search jumps.
`

func main() {
	if len(os.Args) != 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		fmt.Fprint(os.Stdout, helpText)
		if len(os.Args) == 1 || os.Args[1] == "-h" || os.Args[1] == "--help" {
			return
		}
		os.Exit(2)
	}

	r, displayPath, closeFn, err := openInput(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "xray:", err)
		os.Exit(1)
	}
	defer closeFn()

	doc, err := xmltree.Parse(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, "xray:", err)
		os.Exit(1)
	}

	m := newModel(doc, displayPath)
	p := tea.NewProgram(&m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "xray:", err)
		os.Exit(1)
	}
}

func openInput(arg string) (io.Reader, string, func(), error) {
	if arg == "-" {
		return os.Stdin, "stdin", func() {}, nil
	}
	f, err := os.Open(arg)
	if err != nil {
		return nil, "", func() {}, err
	}
	return f, displayPath(arg), func() { _ = f.Close() }, nil
}

func displayPath(path string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Clean(path)
	}
	if rel, err := filepath.Rel(cwd, path); err == nil && rel != "" {
		return rel
	}
	return filepath.Clean(path)
}

type mode int

const (
	modeNormal mode = iota
	modeSearch
)

type location struct {
	selected  int
	offset    int
	query     string
	matches   map[int]bool
	matchIDs  []int
	lastError string
}

type model struct {
	doc          *xmltree.Document
	expanded     map[int]bool
	attrExpanded map[int]map[int]bool
	textExpanded map[int]bool
	matches      map[int]bool
	matchIDs     []int
	selected     int
	offset       int
	width        int
	height       int
	query        string
	mode         mode
	showHelp     bool
	lastError    string
	filePath     string
	jumps        []location
	jumpIndex    int
	searchOrigin *location
	cachedRows   []view.Row
	rowsValid    bool
}

func newModel(doc *xmltree.Document, filePath string) model {
	exp := map[int]bool{}
	xmltree.Walk(doc, func(n *xmltree.Node) { exp[n.ID] = true })
	return model{doc: doc, expanded: exp, attrExpanded: map[int]map[int]bool{}, textExpanded: map[int]bool{}, filePath: filePath, jumpIndex: -1}
}

func (m *model) Init() tea.Cmd { return nil }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if m.width != msg.Width {
			m.invalidateRows()
		}
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if m.mode == modeSearch {
			m.updateSearch(msg)
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "ctrl+d", "d":
			m.pageJump(m.halfPageStep())
		case "ctrl+u", "u":
			m.pageJump(-m.halfPageStep())
		case "ctrl+f", "f":
			m.pageJump(m.pageStep())
		case "ctrl+b", "b":
			m.pageJump(-m.pageStep())
		case "ctrl+e", "e":
			m.scroll(1)
		case "ctrl+y", "y":
			m.scroll(-1)
		case "g":
			m.goTop()
		case "G":
			m.goBottom()
		case "ctrl+o", "o":
			m.jumpBack()
		case "ctrl+i", "tab", "i":
			m.jumpForward()
		case "right", "l":
			row, ok := m.selectedRow()
			if ok && row.Foldable {
				m.toggleFold(row)
			} else {
				m.expandOrChild()
			}
		case "L":
			m.unfoldAll()
		case "left", "h":
			row, ok := m.selectedRow()
			if ok && row.Foldable && row.Expanded {
				m.toggleFold(row)
			} else {
				m.collapseOrParent()
			}
		case "H":
			m.foldAll()
		case "enter":
			row, ok := m.selectedRow()
			if ok && row.Foldable {
				m.toggleFold(row)
			} else {
				m.toggleSelected()
			}
		case "/":
			origin := m.currentLocation()
			m.searchOrigin = &origin
			m.mode = modeSearch
			m.query = ""
		case "esc":
			if m.hasSearch() {
				m.clearSearch()
			}
		case "c":
			if m.hasSearch() {
				m.clearSearch()
			}
		case "n":
			m.nextMatch(1)
		case "N":
			m.nextMatch(-1)
		}
	}
	m.normalizeCursor()
	return m, nil
}

func (m *model) updateSearch(k tea.KeyMsg) {
	switch k.String() {
	case "esc":
		if m.hasSearch() && strings.TrimSpace(m.query) == "" {
			m.clearSearch()
		}
		m.mode = modeNormal
	case "enter":
		m.applySearch()
		m.mode = modeNormal
	case "backspace", "ctrl+h":
		if len(m.query) > 0 {
			m.query = m.query[:len(m.query)-1]
		}
	default:
		if s := k.String(); len(s) == 1 && s >= " " {
			m.query += s
		}
	}
	m.normalizeCursor()
}

func (m *model) applySearch() {
	q := strings.TrimSpace(m.query)
	if q == "" {
		m.clearSearch()
		return
	}
	m.matches = xmltree.Search(m.doc, q)
	m.matchIDs = xmltree.MatchIDs(m.matches)
	m.invalidateRows()
	if len(m.matchIDs) == 0 {
		m.lastError = "No matches"
		m.selected = 0
	} else {
		m.lastError = ""
		rows := m.rows()
		for i, r := range rows {
			if r.Match {
				if m.searchOrigin != nil {
					m.recordLocation(*m.searchOrigin)
				} else if m.selected != i || m.offset != 0 {
					m.recordJump()
				}
				m.selected = i
				m.ensureVisible(len(rows), m.contentHeight())
				break
			}
		}
	}
	m.searchOrigin = nil
}

func (m *model) clearSearch() {
	m.query = ""
	m.matches = nil
	m.matchIDs = nil
	m.selected = 0
	m.offset = 0
	m.lastError = ""
	m.searchOrigin = nil
	m.invalidateRows()
}

func (m *model) hasSearch() bool {
	return m.query != "" || m.matches != nil || len(m.matchIDs) > 0 || m.lastError == "No matches"
}

func (m *model) View() string {
	if m.showHelp {
		return m.fullHeight(renderHelp(helpText), m.status())
	}
	rows := m.rows()
	visibleHeight := m.contentHeight()
	if visibleHeight < 1 {
		visibleHeight = 20
	}

	var lines []string
	end := min(len(rows), m.offset+visibleHeight)
	for i := m.offset; i < end; i++ {
		row := rows[i]
		row.Text = truncate(row.Text, max(20, m.width))
		line := view.SyntaxHighlightRow(row)
		if i == m.selected {
			line = selectedStyle.Render(line)
		} else if rows[i].Match {
			line = matchStyle.Render(line)
		} else if strings.HasPrefix(strings.TrimSpace(rows[i].Text), "⚠") {
			line = warnStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return m.fullHeight(strings.Join(lines, "\n"), m.status())
}

func (m *model) contentHeight() int {
	if m.height <= 0 {
		return 20
	}
	return max(1, m.height-1)
}

func (m *model) fullHeight(content, footer string) string {
	if m.height <= 0 {
		if content == "" {
			return footer
		}
		return content + "\n" + footer
	}

	contentLines := splitLines(content)
	maxContent := max(0, m.height-1)
	width := max(1, m.width)
	if len(contentLines) > maxContent {
		contentLines = contentLines[:maxContent]
	}

	var b strings.Builder
	for _, line := range contentLines {
		b.WriteString(padLine(line, width))
		b.WriteByte('\n')
	}
	for i := len(contentLines); i < maxContent; i++ {
		b.WriteString(strings.Repeat(" ", width))
		b.WriteByte('\n')
	}
	b.WriteString(padLine(footer, width))
	return b.String()
}

func padLine(line string, width int) string {
	if width <= 0 {
		return line
	}
	visible := lipgloss.Width(line)
	if visible >= width {
		return line
	}
	return line + strings.Repeat(" ", width-visible)
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

func (m *model) rows() []view.Row {
	if m.rowsValid {
		return m.cachedRows
	}
	m.cachedRows = view.Flatten(m.doc, view.Options{
		Expanded:        m.expanded,
		Matches:         m.matches,
		InlineTextLimit: xmltree.DefaultInlineTextLimit,
		AttrExpanded:    m.attrExpanded,
		TextExpanded:    m.textExpanded,
		WrapWidth:       m.width,
	})
	m.rowsValid = true
	return m.cachedRows
}

func (m *model) invalidateRows() {
	m.cachedRows = nil
	m.rowsValid = false
}

func (m *model) selectedRow() (view.Row, bool) {
	rows := m.rows()
	if m.selected < 0 || m.selected >= len(rows) {
		return view.Row{}, false
	}
	return rows[m.selected], true
}

func (m *model) move(delta int) {
	rows := m.rows()
	if len(rows) == 0 {
		m.selected = 0
		m.offset = 0
		return
	}
	m.selected = clamp(m.selected+delta, 0, len(rows)-1)
	m.ensureVisible(len(rows), m.contentHeight())
}

func (m *model) normalizeCursor() {
	rows := m.rows()
	if len(rows) == 0 {
		m.selected = 0
		m.offset = 0
		return
	}
	selected := m.selected
	offset := m.offset
	m.selected = clamp(m.selected, 0, len(rows)-1)
	m.offset = clamp(m.offset, 0, max(0, len(rows)-m.contentHeight()))
	if m.selected != selected || m.offset != offset {
		m.ensureVisible(len(rows), m.contentHeight())
	}
}

func (m *model) pageStep() int {
	return max(1, m.contentHeight())
}

func (m *model) halfPageStep() int {
	return max(1, m.contentHeight()/2)
}

func (m *model) pageJump(delta int) {
	rows := m.rows()
	visibleHeight := m.contentHeight()
	maxOffset := max(0, len(rows)-visibleHeight)
	rowInView := clamp(m.selected-m.offset, 0, max(0, visibleHeight-1))
	nextOffset := clamp(m.offset+delta, 0, maxOffset)
	nextSelected := clamp(nextOffset+rowInView, 0, len(rows)-1)
	if nextSelected != m.selected || nextOffset != m.offset {
		m.recordJump()
	}
	m.offset = nextOffset
	m.selected = nextSelected
}

func (m *model) scroll(delta int) {
	rows := m.rows()
	visibleHeight := m.contentHeight()
	m.offset = clamp(m.offset+delta, 0, max(0, len(rows)-visibleHeight))
}

func (m *model) goTop() {
	if m.selected != 0 || m.offset != 0 {
		m.recordJump()
	}
	m.selected = 0
	m.offset = 0
}

func (m *model) goBottom() {
	rows := m.rows()
	if m.selected != max(0, len(rows)-1) {
		m.recordJump()
	}
	m.selected = max(0, len(rows)-1)
	m.ensureVisible(len(rows), m.contentHeight())
}

func (m *model) toggleSelected() {
	if id := m.selectedNodeID(); id != 0 {
		m.expanded[id] = !m.expanded[id]
		m.invalidateRows()
	}
}

func (m *model) toggleFold(row view.Row) {
	if row.AttrIdx >= 0 {
		if m.attrExpanded[row.NodeID] == nil {
			m.attrExpanded[row.NodeID] = map[int]bool{}
		}
		m.attrExpanded[row.NodeID][row.AttrIdx] = !row.Expanded
	} else if row.LongText {
		m.textExpanded[row.NodeID] = !row.Expanded
	}
	m.invalidateRows()
}

func (m *model) foldAll() {
	m.attrExpanded = map[int]map[int]bool{}
	m.textExpanded = map[int]bool{}
	xmltree.Walk(m.doc, func(n *xmltree.Node) {
		m.expanded[n.ID] = false
	})
	m.invalidateRows()
}

func (m *model) unfoldAll() {
	m.attrExpanded = map[int]map[int]bool{}
	m.textExpanded = map[int]bool{}
	xmltree.Walk(m.doc, func(n *xmltree.Node) {
		m.expanded[n.ID] = true
		for i, attr := range n.Attrs {
			if utf8.RuneCountInString(attr.Value) > xmltree.DefaultInlineTextLimit {
				if m.attrExpanded[n.ID] == nil {
					m.attrExpanded[n.ID] = map[int]bool{}
				}
				m.attrExpanded[n.ID][i] = true
			}
		}
		if utf8.RuneCountInString(n.Text) > xmltree.DefaultInlineTextLimit {
			m.textExpanded[n.ID] = true
		}
	})
	m.invalidateRows()
}

func (m *model) collapseOrParent() {
	id := m.selectedNodeID()
	if id == 0 {
		return
	}
	if m.expanded[id] {
		m.expanded[id] = false
		m.invalidateRows()
		return
	}
	n := m.findNode(id)
	if n != nil && n.Parent != nil {
		m.selectNode(n.Parent.ID)
	}
}

func (m *model) expandOrChild() {
	id := m.selectedNodeID()
	if id == 0 {
		return
	}
	if !m.expanded[id] {
		m.expanded[id] = true
		m.invalidateRows()
		return
	}
	n := m.findNode(id)
	if n != nil && len(n.Children) > 0 {
		m.selectNode(n.Children[0].ID)
	}
}

func (m *model) nextMatch(delta int) {
	if len(m.matchIDs) == 0 {
		return
	}
	curID := m.selectedNodeID()
	idx := 0
	for i, id := range m.matchIDs {
		if id == curID {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(m.matchIDs)) % len(m.matchIDs)
	if m.matchIDs[idx] != curID {
		m.recordJump()
	}
	m.selectNode(m.matchIDs[idx])
}

func (m *model) recordJump() {
	m.recordLocation(m.currentLocation())
}

func (m *model) currentLocation() location {
	return location{
		selected:  m.selected,
		offset:    m.offset,
		query:     m.query,
		matches:   cloneMatches(m.matches),
		matchIDs:  append([]int(nil), m.matchIDs...),
		lastError: m.lastError,
	}
}

func (m *model) recordLocation(loc location) {
	if m.jumpIndex < len(m.jumps)-1 {
		m.jumps = append([]location{}, m.jumps[:m.jumpIndex+1]...)
	}
	if m.jumpIndex >= 0 && m.jumpIndex < len(m.jumps) && sameLocation(m.jumps[m.jumpIndex], loc) {
		return
	}
	m.jumps = append(m.jumps, loc)
	m.jumpIndex = len(m.jumps) - 1
}

func (m *model) jumpBack() {
	if len(m.jumps) == 0 {
		m.recordJump()
		if len(m.jumps) == 0 {
			return
		}
	}
	if m.jumpIndex == len(m.jumps)-1 {
		cur := m.currentLocation()
		if !sameLocation(m.jumps[m.jumpIndex], cur) {
			m.jumps = append(m.jumps, cur)
			m.jumpIndex = len(m.jumps) - 1
		}
	}
	if m.jumpIndex <= 0 {
		return
	}
	m.jumpIndex--
	m.goLocation(m.jumps[m.jumpIndex])
}

func (m *model) jumpForward() {
	if m.jumpIndex < 0 || m.jumpIndex >= len(m.jumps)-1 {
		return
	}
	m.jumpIndex++
	m.goLocation(m.jumps[m.jumpIndex])
}

func (m *model) goLocation(loc location) {
	m.query = loc.query
	m.matches = cloneMatches(loc.matches)
	m.matchIDs = append([]int(nil), loc.matchIDs...)
	m.lastError = loc.lastError
	m.searchOrigin = nil
	m.invalidateRows()
	rows := m.rows()
	m.selected = clamp(loc.selected, 0, len(rows)-1)
	m.offset = clamp(loc.offset, 0, max(0, len(rows)-m.contentHeight()))
}

func cloneMatches(matches map[int]bool) map[int]bool {
	if matches == nil {
		return nil
	}
	clone := make(map[int]bool, len(matches))
	for k, v := range matches {
		clone[k] = v
	}
	return clone
}

func sameLocation(a, b location) bool {
	return a.selected == b.selected && a.offset == b.offset && a.query == b.query && a.lastError == b.lastError
}

func (m *model) selectedNodeID() int {
	rows := m.rows()
	if m.selected >= 0 && m.selected < len(rows) {
		return rows[m.selected].NodeID
	}
	return 0
}

func (m *model) selectNode(id int) {
	rows := m.rows()
	for i, r := range rows {
		if r.NodeID == id {
			m.selected = i
			m.ensureVisible(len(rows), m.contentHeight())
			return
		}
	}
}

func (m *model) findNode(id int) *xmltree.Node {
	var found *xmltree.Node
	xmltree.Walk(m.doc, func(n *xmltree.Node) {
		if n.ID == id {
			found = n
		}
	})
	return found
}

func (m *model) status() string {
	left := m.filePath
	if left == "" {
		left = "xray"
	}

	if m.mode == modeSearch {
		left = "/" + m.query
		right := "tag: attr: text: value:  ·  enter apply  ·  esc cancel"
		return statusStyle.Render(alignStatus(left, right, max(20, m.width)))
	}

	controls := []string{"jk move", "hl fold", "du/fb page", "oi jumps", "/ search"}
	if m.hasSearch() {
		controls = append(controls, "esc/c clear")
	}
	controls = append(controls, "? help", "q quit")
	right := strings.Join(controls, "  ·  ")

	if m.query != "" {
		left += fmt.Sprintf("  ·  query: %q matches:%d", m.query, len(m.matchIDs))
	}
	if m.lastError != "" {
		left += "  ·  " + m.lastError
	}
	return statusStyle.Render(alignStatus(left, right, max(20, m.width)))
}

func alignStatus(left, right string, width int) string {
	if width <= 1 {
		return truncate(left, width)
	}
	left = truncate(left, width)
	leftWidth := lipgloss.Width(left)
	if leftWidth >= width-1 {
		return left
	}
	right = truncate(right, max(1, width-leftWidth-1))
	rightWidth := lipgloss.Width(right)
	return left + strings.Repeat(" ", max(1, width-leftWidth-rightWidth)) + right
}

func (m *model) ensureVisible(total, height int) {
	if m.selected < m.offset {
		m.offset = m.selected
	}
	if m.selected >= m.offset+height {
		m.offset = m.selected - height + 1
	}
	m.offset = clamp(m.offset, 0, max(0, total-height))
}

var (
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	matchStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("229"))
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Padding(1, 2)
)

func renderHelp(text string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case i == 0:
			lines[i] = ansi("111", line)
		case strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "tag:") && !strings.HasPrefix(trimmed, "attr:") && !strings.HasPrefix(trimmed, "text:") && !strings.HasPrefix(trimmed, "value:"):
			lines[i] = ansi("179", line)
		case strings.HasPrefix(line, "  ") && strings.Contains(line, "      "):
			key, desc, ok := strings.Cut(line, "      ")
			if ok {
				lines[i] = ansi("150", key) + ansi("245", "      ") + ansi("252", desc)
			}
		case strings.HasPrefix(line, "    ") && trimmed != "":
			lines[i] = ansi("252", line)
		}
	}
	return helpStyle.Render(strings.Join(lines, "\n"))
}

func ansi(color, text string) string {
	if text == "" {
		return text
	}
	return "\x1b[38;5;" + color + "m" + text + "\x1b[0m"
}

func clamp(v, low, high int) int {
	if high < low {
		return low
	}
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}

func truncate(s string, width int) string {
	r := []rune(s)
	if width <= 1 || len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
