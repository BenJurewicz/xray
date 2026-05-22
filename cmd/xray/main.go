package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
  ctrl-d/u     half-page down / up
  ctrl-f/b     page down / up
  ctrl-e/y     scroll down / up
  gg / G       top / bottom
  enter        toggle expand
  /            search
  c            clear search filter
  n / N        next / previous match
  ?            toggle help
  q            quit
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

	p := tea.NewProgram(newModel(doc, displayPath), tea.WithAltScreen())
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

type model struct {
	doc       *xmltree.Document
	expanded  map[int]bool
	matches   map[int]bool
	matchIDs  []int
	selected  int
	offset    int
	width     int
	height    int
	query     string
	mode      mode
	showHelp  bool
	lastError string
	filePath  string
	pendingG  bool
}

func newModel(doc *xmltree.Document, filePath string) model {
	exp := map[int]bool{}
	xmltree.Walk(doc, func(n *xmltree.Node) { exp[n.ID] = true })
	return model{doc: doc, expanded: exp, filePath: filePath}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if m.mode == modeSearch {
			return m.updateSearch(msg), nil
		}
		if m.pendingG {
			m.pendingG = false
			if msg.String() == "g" {
				m.goTop()
				return m, nil
			}
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
		case "ctrl+d":
			m.pageJump(m.halfPageStep())
		case "ctrl+u":
			m.pageJump(-m.halfPageStep())
		case "ctrl+f":
			m.pageJump(m.pageStep())
		case "ctrl+b":
			m.pageJump(-m.pageStep())
		case "ctrl+e":
			m.scroll(1)
		case "ctrl+y":
			m.scroll(-1)
		case "g":
			m.pendingG = true
		case "G":
			m.goBottom()
		case "left", "h":
			m.collapseOrParent()
		case "right", "l":
			m.expandOrChild()
		case "enter":
			m.toggleSelected()
		case "/":
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
	return m, nil
}

func (m model) updateSearch(k tea.KeyMsg) model {
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
	return m
}

func (m *model) applySearch() {
	q := strings.TrimSpace(m.query)
	if q == "" {
		m.clearSearch()
		return
	}
	m.matches = xmltree.Search(m.doc, q)
	m.matchIDs = xmltree.MatchIDs(m.matches)
	m.selected = 0
	if len(m.matchIDs) == 0 {
		m.lastError = "No matches"
	} else {
		m.lastError = ""
		rows := m.rows()
		for i, r := range rows {
			if r.Match {
				m.selected = i
				break
			}
		}
	}
}

func (m *model) clearSearch() {
	m.query = ""
	m.matches = nil
	m.matchIDs = nil
	m.selected = 0
	m.offset = 0
	m.lastError = ""
}

func (m model) hasSearch() bool {
	return m.query != "" || m.matches != nil || len(m.matchIDs) > 0 || m.lastError == "No matches"
}

func (m model) View() string {
	if m.showHelp {
		return m.fullHeight(helpStyle.Render(helpText), m.status())
	}
	rows := m.rows()
	visibleHeight := m.contentHeight()
	if visibleHeight < 1 {
		visibleHeight = 20
	}

	var lines []string
	end := min(len(rows), m.offset+visibleHeight)
	for i := m.offset; i < end; i++ {
		line := truncate(rows[i].Text, max(20, m.width))
		line = view.SyntaxHighlight(line)
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

func (m model) contentHeight() int {
	if m.height <= 0 {
		return 20
	}
	return max(1, m.height-1)
}

func (m model) fullHeight(content, footer string) string {
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

func (m model) rows() []view.Row {
	return view.Flatten(m.doc, view.Options{Expanded: m.expanded, Matches: m.matches, InlineTextLimit: xmltree.DefaultInlineTextLimit})
}

func (m *model) move(delta int) {
	rows := m.rows()
	m.selected = clamp(m.selected+delta, 0, len(rows)-1)
	m.ensureVisible(len(rows), m.contentHeight())
}

func (m model) pageStep() int {
	return max(1, m.contentHeight())
}

func (m model) halfPageStep() int {
	return max(1, m.contentHeight()/2)
}

func (m *model) pageJump(delta int) {
	rows := m.rows()
	visibleHeight := m.contentHeight()
	maxOffset := max(0, len(rows)-visibleHeight)
	rowInView := clamp(m.selected-m.offset, 0, max(0, visibleHeight-1))
	m.offset = clamp(m.offset+delta, 0, maxOffset)
	m.selected = clamp(m.offset+rowInView, 0, len(rows)-1)
}

func (m *model) scroll(delta int) {
	rows := m.rows()
	visibleHeight := m.contentHeight()
	m.offset = clamp(m.offset+delta, 0, max(0, len(rows)-visibleHeight))
}

func (m *model) goTop() {
	m.selected = 0
	m.offset = 0
}

func (m *model) goBottom() {
	rows := m.rows()
	m.selected = max(0, len(rows)-1)
	m.ensureVisible(len(rows), m.contentHeight())
}

func (m *model) toggleSelected() {
	if id := m.selectedNodeID(); id != 0 {
		m.expanded[id] = !m.expanded[id]
	}
}

func (m *model) collapseOrParent() {
	id := m.selectedNodeID()
	if id == 0 {
		return
	}
	if m.expanded[id] {
		m.expanded[id] = false
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
	m.selectNode(m.matchIDs[idx])
}

func (m model) selectedNodeID() int {
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

func (m model) findNode(id int) *xmltree.Node {
	var found *xmltree.Node
	xmltree.Walk(m.doc, func(n *xmltree.Node) {
		if n.ID == id {
			found = n
		}
	})
	return found
}

func (m model) status() string {
	left := m.filePath
	if left == "" {
		left = "xray"
	}

	if m.mode == modeSearch {
		left = "/" + m.query
		right := "tag: attr: text: value:  ·  enter apply  ·  esc cancel"
		return statusStyle.Render(alignStatus(left, right, max(20, m.width)))
	}

	controls := []string{"jk/hl nav", "^D/^U page", "/ search"}
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
	minLeft := min(width, 12)
	maxRight := max(1, width-minLeft-1)
	right = truncate(right, maxRight)
	left = truncate(left, max(1, width-lipgloss.Width(right)-1))
	leftWidth := lipgloss.Width(left)
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
