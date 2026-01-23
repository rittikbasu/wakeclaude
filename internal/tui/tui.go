package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"wakeclaude/internal/app"
)

type Result struct {
	ProjectPath string
	SessionID   string
	SessionPath string
	NewSession  bool
	Model       string
}

type stage int

const (
	stageProjects stage = iota
	stageSessions
	stageModels
)

var ErrUserQuit = errors.New("user quit")

func Run(projects []app.Project) (Result, error) {
	m := newModel(projects)
	program := tea.NewProgram(m, tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return Result{}, err
	}

	model, ok := final.(model)
	if !ok {
		return Result{}, fmt.Errorf("unexpected model type")
	}
	if model.err != nil {
		return Result{}, model.err
	}
	if model.result == nil {
		return Result{}, ErrUserQuit
	}

	return *model.result, nil
}

type itemKind int

const (
	itemProject itemKind = iota
	itemSession
	itemNewSession
	itemModel
)

type listItem struct {
	title  string
	meta   string
	filter string
	kind   itemKind
	index  int
	pinned bool
}

type model struct {
	stage    stage
	projects []app.Project
	sessions []app.Session
	project  app.Project
	selectedSession *app.Session
	selectedNew     bool
	models          []modelOption

	searchValue string
	items  []listItem
	all    []listItem
	cursor int
	offset int
	width  int
	height int

	result *Result
	err    error
}

type modelOption struct {
	Label string
	Value string
}

var defaultModels = []modelOption{
	{Label: "Default (auto)", Value: "auto"},
	{Label: "Sonnet (latest)", Value: "sonnet"},
	{Label: "Opus (latest)", Value: "opus"},
	{Label: "Haiku (latest)", Value: "haiku"},
}

func newModel(projects []app.Project) model {
	m := model{
		stage:    stageProjects,
		projects: projects,
		models:   defaultModels,
	}

	m.setProjectItems()
	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.err = ErrUserQuit
			return m, tea.Quit
		case "esc":
			if m.stage == stageSessions {
				m.stage = stageProjects
				m.project = app.Project{}
				m.sessions = nil
				m.selectedSession = nil
				m.selectedNew = false
				m.setProjectItems()
				return m, nil
			}
			if m.stage == stageModels {
				m.stage = stageSessions
				m.setSessionItems()
				return m, nil
			}
			m.err = ErrUserQuit
			return m, tea.Quit
		case "enter":
			return m, m.selectCurrent()
		case "up", "k":
			m.moveCursor(-1)
			return m, nil
		case "down", "j":
			m.moveCursor(1)
			return m, nil
		case "pgup":
			m.moveCursor(-m.visibleCount())
			return m, nil
		case "pgdown":
			m.moveCursor(m.visibleCount())
			return m, nil
		case "home":
			m.cursor = 0
			m.ensureCursorVisible()
			return m, nil
		case "end":
			m.cursor = max(0, len(m.items)-1)
			m.ensureCursorVisible()
			return m, nil
		}
		if m.handleSearchInput(msg) {
			return m, nil
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.err != nil && !errors.Is(m.err, ErrUserQuit) {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	var b strings.Builder
	for _, line := range asciiArtLines {
		b.WriteString(renderLine(line, safeWidth(m.width)))
		b.WriteString("\n")
	}
	b.WriteString(renderLine("Schedule prompts to run at specific times with Wake Claude.", safeWidth(m.width)))
	b.WriteString("\n")

	switch m.stage {
	case stageProjects:
		b.WriteString(renderLine("Select a project to continue.", safeWidth(m.width)))
		b.WriteString("\n")
	case stageSessions:
		label := m.project.DisplayName
		if label == "" {
			label = m.project.Path
		}
		b.WriteString(renderLine(fmt.Sprintf("Project: %s", label), safeWidth(m.width)))
		b.WriteString("\n")
		b.WriteString(renderLine("Select a session to resume (or start a new one).", safeWidth(m.width)))
		b.WriteString("\n")
	case stageModels:
		label := m.project.DisplayName
		if label == "" {
			label = m.project.Path
		}
		b.WriteString(renderLine(fmt.Sprintf("Project: %s", label), safeWidth(m.width)))
		b.WriteString("\n")
		if m.selectedSession != nil {
			b.WriteString(renderLine(fmt.Sprintf("Session: %s", m.selectedSession.Preview), safeWidth(m.width)))
			b.WriteString("\n")
		} else if m.selectedNew {
			b.WriteString(renderLine("Session: Start a new session", safeWidth(m.width)))
			b.WriteString("\n")
		}
		b.WriteString(renderLine("Select a Claude model.", safeWidth(m.width)))
		b.WriteString("\n")
	}

	b.WriteString(m.renderSearchLine())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", max(10, min(safeWidth(m.width), 60))))
	b.WriteString("\n")

	if len(m.items) == 0 {
		b.WriteString("No matches.\n")
	} else {
		start, end := m.visibleRange()
		for i := start; i < end; i++ {
			selected := i == m.cursor
			b.WriteString(renderItem(m.items[i], selected, safeWidth(m.width)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString("up/down move | enter select | esc back | q quit\n")
	return b.String()
}

func (m *model) setProjectItems() {
	m.searchValue = ""
	m.selectedSession = nil
	m.selectedNew = false
	items := make([]listItem, 0, len(m.projects))
	for i, project := range m.projects {
		display := project.DisplayName
		if display == "" {
			display = project.Path
		}
		sessionLabel := sessionCountLabel(project.SessionCount)
		title := fmt.Sprintf("%s (%s)", display, sessionLabel)
		meta := project.LastActive
		filter := strings.ToLower(strings.Join([]string{display, project.Path, project.CWD}, " "))
		items = append(items, listItem{
			title:  title,
			meta:   meta,
			filter: filter,
			kind:   itemProject,
			index:  i,
		})
	}
	m.all = items
	m.applyFilter()
}

func (m *model) setSessionItems() {
	m.searchValue = ""
	items := make([]listItem, 0, len(m.sessions)+1)
	items = append(items, listItem{
		title:  "Start a new session",
		meta:   "new",
		filter: "new session start",
		kind:   itemNewSession,
		pinned: true,
	})

	for i, session := range m.sessions {
		title := session.Preview
		if title == "" {
			continue
		}
		meta := session.RelTime
		filter := strings.ToLower(strings.Join([]string{title, session.ID}, " "))
		items = append(items, listItem{
			title:  title,
			meta:   meta,
			filter: filter,
			kind:   itemSession,
			index:  i,
		})
	}

	m.all = items
	m.applyFilter()
}

func (m *model) setModelItems() {
	m.searchValue = ""
	items := make([]listItem, 0, len(m.models))
	for i, option := range m.models {
		title := option.Label
		meta := option.Value
		filter := strings.ToLower(strings.Join([]string{option.Label, option.Value}, " "))
		items = append(items, listItem{
			title:  title,
			meta:   meta,
			filter: filter,
			kind:   itemModel,
			index:  i,
		})
	}
	m.all = items
	m.applyFilter()
}

func (m *model) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.searchValue))
	if query == "" {
		m.items = append([]listItem(nil), m.all...)
	} else {
		filtered := make([]listItem, 0, len(m.all))
		for _, item := range m.all {
			if item.pinned {
				filtered = append(filtered, item)
				continue
			}
			if strings.Contains(item.filter, query) {
				filtered = append(filtered, item)
			}
		}
		m.items = filtered
	}

	m.cursor = clamp(m.cursor, 0, max(0, len(m.items)-1))
	m.ensureCursorVisible()
}

func (m *model) handleSearchInput(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyRunes:
		m.searchValue += string(msg.Runes)
		m.applyFilter()
		return true
	case tea.KeySpace:
		m.searchValue += " "
		m.applyFilter()
		return true
	case tea.KeyBackspace, tea.KeyDelete:
		if m.searchValue == "" {
			return true
		}
		m.searchValue = trimLastRune(m.searchValue)
		m.applyFilter()
		return true
	case tea.KeyCtrlU:
		if m.searchValue == "" {
			return true
		}
		m.searchValue = ""
		m.applyFilter()
		return true
	}

	return false
}

func (m model) renderSearchLine() string {
	value := m.searchValue
	line := fmt.Sprintf("Search: %s_", value)
	return renderLine(line, safeWidth(m.width))
}

func (m *model) selectCurrent() tea.Cmd {
	if len(m.items) == 0 {
		return nil
	}

	item := m.items[m.cursor]
	switch item.kind {
	case itemProject:
		project := m.projects[item.index]
		sessions, err := app.ListSessions(project.Path)
		if err != nil {
			m.err = err
			return tea.Quit
		}
		m.stage = stageSessions
		m.project = project
		m.sessions = sessions
		m.setSessionItems()
		return nil
	case itemNewSession:
		m.selectedSession = nil
		m.selectedNew = true
		m.stage = stageModels
		m.setModelItems()
		return nil
	case itemSession:
		session := m.sessions[item.index]
		m.selectedSession = &session
		m.selectedNew = false
		m.stage = stageModels
		m.setModelItems()
		return nil
	case itemModel:
		if item.index < 0 || item.index >= len(m.models) {
			return nil
		}
		option := m.models[item.index]
		if m.selectedNew {
			m.result = &Result{
				ProjectPath: m.project.Path,
				NewSession:  true,
				Model:       option.Value,
			}
			return tea.Quit
		}
		if m.selectedSession != nil {
			m.result = &Result{
				ProjectPath: m.project.Path,
				SessionID:   m.selectedSession.ID,
				SessionPath: m.selectedSession.Path,
				Model:       option.Value,
			}
			return tea.Quit
		}
		return nil
	default:
		return nil
	}
}

func (m *model) moveCursor(delta int) {
	if len(m.items) == 0 {
		return
	}
	m.cursor = clamp(m.cursor+delta, 0, len(m.items)-1)
	m.ensureCursorVisible()
}

func (m *model) ensureCursorVisible() {
	visible := m.visibleCount()
	if visible <= 0 {
		m.offset = 0
		return
	}

	if m.cursor < m.offset {
		m.offset = m.cursor
		return
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

func (m model) visibleCount() int {
	available := m.height - m.headerLines() - 2
	if available < 3 {
		return 3
	}
	return available
}

func (m model) headerLines() int {
	lines := len(asciiArtLines) + 1
	switch m.stage {
	case stageProjects:
		lines += 1
	case stageSessions:
		lines += 2
	case stageModels:
		lines += 3
	default:
		lines += 1
	}
	lines += 2
	return lines
}

func (m model) visibleRange() (int, int) {
	if len(m.items) == 0 {
		return 0, 0
	}
	visible := m.visibleCount()
	start := clamp(m.offset, 0, len(m.items)-1)
	end := min(len(m.items), start+visible)
	return start, end
}

func renderItem(item listItem, selected bool, width int) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	line := fmt.Sprintf("%s%-9s %s", prefix, item.meta, item.title)
	return renderLine(line, width)
}

func renderLine(text string, width int) string {
	text = truncateToWidth(text, width)
	if width <= 0 {
		return text
	}
	trimmed := []rune(text)
	if len(trimmed) >= width {
		return text
	}
	return text + strings.Repeat(" ", width-len(trimmed))
}

func sessionCountLabel(count int) string {
	if count == 1 {
		return "1 session"
	}
	return fmt.Sprintf("%d sessions", count)
}

func truncateToWidth(text string, width int) string {
	if width <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	if width <= 3 {
		return string(runes[:width])
	}

	return string(runes[:width-3]) + "..."
}

func trimLastRune(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return ""
	}
	return string(runes[:len(runes)-1])
}

func safeWidth(width int) int {
	if width <= 0 {
		return 80
	}
	return width
}

var asciiArtLines = []string{
	"              _          _              _     ",
	" __ __ ____ _| |_____ __| |__ _ _  _ __| |___ ",
	" \\ V  V / _` | / / -_/ _| / _` | || / _` / -_)",
	"  \\_/\\_/\\__,_|_\\_\\___\\__|_\\__,_|\\_,_\\__,_\\___|",
	"                                              ",
	"",
}

func clamp(value, minVal, maxVal int) int {
	if value < minVal {
		return minVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
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
