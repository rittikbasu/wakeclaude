package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"wakeclaude/internal/app"
)

type Result struct {
	ProjectPath string   `json:"projectPath"`
	SessionID   string   `json:"sessionId,omitempty"`
	SessionPath string   `json:"sessionPath,omitempty"`
	NewSession  bool     `json:"newSession"`
	Model       string   `json:"model"`
	Prompt      string   `json:"prompt"`
	Schedule    Schedule `json:"schedule"`
}

type Schedule struct {
	Type     string `json:"type"`
	Date     string `json:"date,omitempty"`
	Time     string `json:"time,omitempty"`
	Weekday  string `json:"weekday,omitempty"`
	Timezone string `json:"timezone,omitempty"`
}

type stage int

const (
	stageProjects stage = iota
	stageSessions
	stageModels
	stagePrompt
	stageScheduleType
	stageScheduleDate
	stageScheduleWeekday
	stageScheduleTime
)

var ErrUserQuit = errors.New("user quit")

func Run(projects []app.Project, models []app.ModelOption) (Result, error) {
	m := newModel(projects, models)
	program := tea.NewProgram(m, tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return Result{}, err
	}

	var state model
	switch typed := final.(type) {
	case model:
		state = typed
	case *model:
		state = *typed
	default:
		return Result{}, fmt.Errorf("unexpected model type")
	}
	if state.err != nil {
		return Result{}, state.err
	}
	if state.result == nil {
		return Result{}, ErrUserQuit
	}

	return *state.result, nil
}

type itemKind int

const (
	itemProject itemKind = iota
	itemSession
	itemNewSession
	itemModel
	itemScheduleType
	itemWeekday
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
	stage           stage
	projects        []app.Project
	sessions        []app.Session
	project         app.Project
	selectedSession *app.Session
	selectedNew     bool
	selectedModel   app.ModelOption
	models          []app.ModelOption

	promptText string
	schedule   Schedule
	inputError string

	searchInput textinput.Model
	promptInput textarea.Model
	dateInput   textinput.Model
	timeInput   textinput.Model

	items  []listItem
	all    []listItem
	cursor int
	offset int
	width  int
	height int

	result *Result
	err    error
}

func newModel(projects []app.Project, models []app.ModelOption) model {
	if len(models) == 0 {
		models = []app.ModelOption{{Label: "Default (auto)", Value: "auto"}}
	}

	search := textinput.New()
	search.Prompt = "Search: "
	search.Placeholder = "type to filter"
	search.CharLimit = 256

	prompt := textarea.New()
	prompt.Placeholder = "Type the prompt you want to run..."
	prompt.ShowLineNumbers = false
	prompt.CharLimit = 0
	prompt.Blur()

	dateInput := textinput.New()
	dateInput.Prompt = ""
	dateInput.Placeholder = "YYYY-MM-DD"
	dateInput.CharLimit = 10
	dateInput.Blur()

	timeInput := textinput.New()
	timeInput.Prompt = ""
	timeInput.Placeholder = "HH:MM"
	timeInput.CharLimit = 5
	timeInput.Blur()

	m := model{
		stage:       stageProjects,
		projects:    projects,
		models:      models,
		searchInput: search,
		promptInput: prompt,
		dateInput:   dateInput,
		timeInput:   timeInput,
	}

	m.setProjectItems()
	m.applyInputSizing()
	m.searchInput.Focus()
	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msgTyped := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msgTyped.Width
		m.height = msgTyped.Height
		m.applyInputSizing()
		return m, nil
	case tea.KeyMsg:
		switch msgTyped.String() {
		case "ctrl+c", "q":
			m.err = ErrUserQuit
			return m, tea.Quit
		case "esc":
			return m.handleBack()
		}
	}

	switch m.stage {
	case stagePrompt:
		return m.updatePrompt(msg)
	case stageScheduleDate, stageScheduleTime:
		return m.updateScheduleInput(msg)
	case stageProjects, stageSessions, stageModels, stageScheduleType, stageScheduleWeekday:
		return m.updateList(msg)
	default:
		return m, nil
	}
}

func (m model) View() string {
	if m.err != nil && !errors.Is(m.err, ErrUserQuit) {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	var b strings.Builder
	lineWidth := renderWidth(m.width)
	for _, line := range asciiArtLines {
		b.WriteString(renderLine(line, lineWidth))
		b.WriteString("\n")
	}
	b.WriteString(renderLine("Schedule prompts to run at specific times with Wake Claude.", lineWidth))
	b.WriteString("\n")

	switch m.stage {
	case stagePrompt:
		m.renderPrompt(&b, lineWidth)
		return b.String()
	case stageScheduleDate:
		m.renderScheduleDate(&b, lineWidth)
		return b.String()
	case stageScheduleTime:
		m.renderScheduleTime(&b, lineWidth)
		return b.String()
	default:
		m.renderList(&b, lineWidth)
		return b.String()
	}
}

func (m model) renderPrompt(b *strings.Builder, width int) {
	m.renderContextHeader(b, width)
	b.WriteString(renderLine("Enter the prompt to run.", width))
	b.WriteString("\n")
	b.WriteString(m.promptInput.View())
	b.WriteString("\n")
	if m.inputError != "" {
		b.WriteString(renderLine(fmt.Sprintf("Error: %s", m.inputError), width))
		b.WriteString("\n")
	}
	b.WriteString("ctrl+d continue | esc back | q quit\n")
}

func (m model) renderScheduleDate(b *strings.Builder, width int) {
	m.renderContextHeader(b, width)
	b.WriteString(renderLine("One-time schedule.", width))
	b.WriteString("\n")
	b.WriteString(renderLine("Date (YYYY-MM-DD):", width))
	b.WriteString("\n")
	b.WriteString(m.dateInput.View())
	b.WriteString("\n")
	if m.inputError != "" {
		b.WriteString(renderLine(fmt.Sprintf("Error: %s", m.inputError), width))
		b.WriteString("\n")
	}
	b.WriteString("enter confirm | esc back | q quit\n")
}

func (m model) renderScheduleTime(b *strings.Builder, width int) {
	m.renderContextHeader(b, width)
	switch m.schedule.Type {
	case "daily":
		b.WriteString(renderLine("Daily schedule.", width))
		b.WriteString("\n")
	case "weekly":
		if m.schedule.Weekday != "" {
			b.WriteString(renderLine(fmt.Sprintf("Weekly on %s.", m.schedule.Weekday), width))
			b.WriteString("\n")
		}
	case "once":
		if m.schedule.Date != "" {
			b.WriteString(renderLine(fmt.Sprintf("One-time on %s.", m.schedule.Date), width))
			b.WriteString("\n")
		}
	}
	b.WriteString(renderLine("Time (24-hour HH:MM):", width))
	b.WriteString("\n")
	b.WriteString(m.timeInput.View())
	b.WriteString("\n")
	if m.inputError != "" {
		b.WriteString(renderLine(fmt.Sprintf("Error: %s", m.inputError), width))
		b.WriteString("\n")
	}
	b.WriteString("enter confirm | esc back | q quit\n")
}

func (m model) renderList(b *strings.Builder, width int) {
	switch m.stage {
	case stageProjects:
		b.WriteString(renderLine("Select a project to continue.", width))
		b.WriteString("\n")
	case stageSessions:
		b.WriteString(renderLine(fmt.Sprintf("Project: %s", m.projectLabel()), width))
		b.WriteString("\n")
		b.WriteString(renderLine("Select a session to resume (or start a new one).", width))
		b.WriteString("\n")
	case stageModels:
		b.WriteString(renderLine(fmt.Sprintf("Project: %s", m.projectLabel()), width))
		b.WriteString("\n")
		b.WriteString(renderLine(fmt.Sprintf("Session: %s", m.sessionLabel()), width))
		b.WriteString("\n")
		b.WriteString(renderLine("Select a Claude model.", width))
		b.WriteString("\n")
	case stageScheduleType:
		m.renderContextHeader(b, width)
		b.WriteString(renderLine("Select when to run it.", width))
		b.WriteString("\n")
	case stageScheduleWeekday:
		m.renderContextHeader(b, width)
		b.WriteString(renderLine("Schedule: Weekly.", width))
		b.WriteString("\n")
		b.WriteString(renderLine("Select the day of week.", width))
		b.WriteString("\n")
	}

	b.WriteString(m.searchInput.View())
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", max(10, min(width, 60))))
	b.WriteString("\n")

	if len(m.items) == 0 {
		b.WriteString("No matches.\n")
	} else {
		start, end := m.visibleRange()
		for i := start; i < end; i++ {
			selected := i == m.cursor
			b.WriteString(renderItem(m.items[i], selected, width))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString("up/down move | enter select | esc back | q quit\n")
}

func (m model) renderContextHeader(b *strings.Builder, width int) {
	b.WriteString(renderLine(fmt.Sprintf("Project: %s", m.projectLabel()), width))
	b.WriteString("\n")
	b.WriteString(renderLine(fmt.Sprintf("Session: %s", m.sessionLabel()), width))
	b.WriteString("\n")
	if label := m.modelLabel(); label != "" {
		b.WriteString(renderLine(fmt.Sprintf("Model: %s", label), width))
		b.WriteString("\n")
	}
	if preview := m.promptPreview(); preview != "" && m.stage != stagePrompt {
		b.WriteString(renderLine(fmt.Sprintf("Prompt: %s", preview), width))
		b.WriteString("\n")
	}
}

func (m model) projectLabel() string {
	if m.project.DisplayName != "" {
		return m.project.DisplayName
	}
	return m.project.Path
}

func (m model) sessionLabel() string {
	if m.selectedSession != nil {
		return m.selectedSession.Preview
	}
	if m.selectedNew {
		return "Start a new session"
	}
	return ""
}

func (m model) modelLabel() string {
	if m.selectedModel.Label != "" {
		return m.selectedModel.Label
	}
	return m.selectedModel.Value
}

func (m model) promptPreview() string {
	value := strings.TrimSpace(m.currentPrompt())
	if value == "" {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	return truncateString(value, 80)
}

func (m model) currentPrompt() string {
	if m.promptText != "" {
		return m.promptText
	}
	return m.promptInput.Value()
}

func (m *model) setProjectItems() {
	m.selectedSession = nil
	m.selectedNew = false
	m.selectedModel = app.ModelOption{}
	m.promptText = ""
	m.inputError = ""
	m.schedule = Schedule{}
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	m.promptInput.SetValue("")
	m.dateInput.SetValue("")
	m.timeInput.SetValue("")
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
	m.inputError = ""
	m.searchInput.SetValue("")
	m.searchInput.Focus()
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
	m.inputError = ""
	m.searchInput.SetValue("")
	m.searchInput.Focus()
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

func (m *model) setScheduleTypeItems() {
	m.inputError = ""
	m.searchInput.SetValue("")
	options := scheduleTypeOptions
	items := make([]listItem, 0, len(options))
	for i, option := range options {
		items = append(items, listItem{
			title:  option.Label,
			meta:   option.Meta,
			filter: strings.ToLower(option.Label + " " + option.Meta),
			kind:   itemScheduleType,
			index:  i,
		})
	}
	m.all = items
	m.applyFilter()
}

func (m *model) setWeekdayItems() {
	m.inputError = ""
	m.searchInput.SetValue("")
	options := weekdayOptions
	items := make([]listItem, 0, len(options))
	for i, option := range options {
		items = append(items, listItem{
			title:  option.Label,
			meta:   option.Meta,
			filter: strings.ToLower(option.Label + " " + option.Meta),
			kind:   itemWeekday,
			index:  i,
		})
	}
	m.all = items
	m.applyFilter()
}

func (m *model) handleBack() (tea.Model, tea.Cmd) {
	switch m.stage {
	case stageSessions:
		m.stage = stageProjects
		m.project = app.Project{}
		m.sessions = nil
		m.selectedSession = nil
		m.selectedNew = false
		m.selectedModel = app.ModelOption{}
		m.setProjectItems()
		return m, nil
	case stageModels:
		m.stage = stageSessions
		m.selectedModel = app.ModelOption{}
		m.promptText = ""
		m.schedule = Schedule{}
		m.promptInput.SetValue("")
		m.dateInput.SetValue("")
		m.timeInput.SetValue("")
		m.promptInput.Blur()
		m.setSessionItems()
		return m, nil
	case stagePrompt:
		m.stage = stageModels
		m.promptText = strings.TrimSpace(m.promptInput.Value())
		m.promptInput.Blur()
		m.setModelItems()
		return m, nil
	case stageScheduleType:
		m.startPromptStage()
		return m, nil
	case stageScheduleDate:
		m.startScheduleTypeStage()
		return m, nil
	case stageScheduleWeekday:
		m.startScheduleTypeStage()
		return m, nil
	case stageScheduleTime:
		if m.schedule.Type == "once" {
			m.startScheduleDateStage()
			return m, nil
		}
		if m.schedule.Type == "weekly" {
			m.startScheduleWeekdayStage()
			return m, nil
		}
		m.startScheduleTypeStage()
		return m, nil
	default:
		m.err = ErrUserQuit
		return m, tea.Quit
	}
}

func (m *model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
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
	}
	prev := m.searchInput.Value()
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	if m.searchInput.Value() != prev {
		m.applyFilter()
	}
	return m, cmd
}

func (m *model) applyInputSizing() {
	width := renderWidth(m.width)
	if width <= 0 {
		width = 80
	}
	m.searchInput.Width = width
	m.promptInput.SetWidth(width)
	m.promptInput.SetHeight(promptHeight(m.height))
	m.dateInput.Width = width
	m.timeInput.Width = width
}

func (m *model) updatePrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	prev := m.promptInput.Value()
	m.promptInput, cmd = m.promptInput.Update(msg)

	key, ok := msg.(tea.KeyMsg)
	if ok && key.Type == tea.KeyCtrlD {
		value := strings.TrimSpace(m.promptInput.Value())
		if value == "" {
			m.inputError = "Prompt cannot be empty."
			return m, cmd
		}
		m.promptText = value
		m.startScheduleTypeStage()
		return m, cmd
	}

	if m.promptInput.Value() != prev {
		m.inputError = ""
	}

	return m, cmd
}

func (m *model) updateScheduleInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.stage {
	case stageScheduleDate:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			var cmd tea.Cmd
			m.dateInput, cmd = m.dateInput.Update(msg)
			return m, cmd
		}
		prev := m.dateInput.Value()
		var cmd tea.Cmd
		m.dateInput, cmd = m.dateInput.Update(msg)
		if key.Type == tea.KeyEnter {
			value := strings.TrimSpace(m.dateInput.Value())
			if !isValidDate(value) {
				m.inputError = "Enter date as YYYY-MM-DD."
				return m, cmd
			}
			m.schedule.Date = value
			m.startScheduleTimeStage()
			return m, cmd
		}
		if m.dateInput.Value() != prev {
			m.inputError = ""
		}
		return m, cmd
	case stageScheduleTime:
		key, ok := msg.(tea.KeyMsg)
		if !ok {
			var cmd tea.Cmd
			m.timeInput, cmd = m.timeInput.Update(msg)
			return m, cmd
		}
		if key.Type == tea.KeyEnter {
			value := strings.TrimSpace(m.timeInput.Value())
			if !isValidTime(value) {
				m.inputError = "Enter time as HH:MM (24-hour)."
				return m, nil
			}
			m.schedule.Time = value
			m.schedule.Timezone = time.Now().Location().String()
			m.finishResult()
			return m, tea.Quit
		}

		value, pos, changed := applyTimeMask(m.timeInput.Value(), m.timeInput.Position(), key)
		if changed {
			m.timeInput.SetValue(value)
			m.timeInput.SetCursor(pos)
			m.inputError = ""
			return m, nil
		}
		return m, nil
	default:
		return m, nil
	}
}

func (m *model) startPromptStage() {
	m.stage = stagePrompt
	m.inputError = ""
	m.searchInput.Blur()
	m.dateInput.Blur()
	m.timeInput.Blur()
	m.promptInput.SetValue(m.promptText)
	m.promptInput.Focus()
}

func (m *model) startScheduleTypeStage() {
	m.stage = stageScheduleType
	m.inputError = ""
	m.promptInput.Blur()
	m.dateInput.Blur()
	m.timeInput.Blur()
	m.searchInput.Focus()
	m.setScheduleTypeItems()
}

func (m *model) startScheduleDateStage() {
	m.stage = stageScheduleDate
	m.inputError = ""
	m.searchInput.Blur()
	m.promptInput.Blur()
	m.timeInput.Blur()
	m.dateInput.Focus()
	if strings.TrimSpace(m.schedule.Date) != "" {
		m.dateInput.SetValue(m.schedule.Date)
	} else if strings.TrimSpace(m.dateInput.Value()) == "" {
		m.dateInput.SetValue(time.Now().Format("2006-01-02"))
	}
}

func (m *model) startScheduleWeekdayStage() {
	m.stage = stageScheduleWeekday
	m.inputError = ""
	m.searchInput.Focus()
	m.promptInput.Blur()
	m.dateInput.Blur()
	m.timeInput.Blur()
	m.setWeekdayItems()
}

func (m *model) startScheduleTimeStage() {
	m.stage = stageScheduleTime
	m.inputError = ""
	m.searchInput.Blur()
	m.promptInput.Blur()
	m.dateInput.Blur()
	m.timeInput.Focus()
	if strings.TrimSpace(m.schedule.Time) != "" && isValidTime(m.schedule.Time) {
		m.timeInput.SetValue(normalizeTimeValue(m.schedule.Time))
	} else if strings.TrimSpace(m.timeInput.Value()) == "" || !isValidTime(m.timeInput.Value()) {
		m.timeInput.SetValue(normalizeTimeValue(time.Now().Format("15:04")))
	} else {
		m.timeInput.SetValue(normalizeTimeValue(m.timeInput.Value()))
	}
	m.timeInput.SetCursor(0)
}

func (m *model) finishResult() {
	m.result = &Result{
		ProjectPath: m.project.Path,
		Model:       m.selectedModel.Value,
		Prompt:      m.promptText,
		Schedule:    m.schedule,
	}
	if m.selectedNew {
		m.result.NewSession = true
		return
	}
	if m.selectedSession != nil {
		m.result.SessionID = m.selectedSession.ID
		m.result.SessionPath = m.selectedSession.Path
	}
}

func (m *model) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.searchInput.Value()))
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
		m.selectedModel = option
		m.startPromptStage()
		return nil
	case itemScheduleType:
		if item.index < 0 || item.index >= len(scheduleTypeOptions) {
			return nil
		}
		option := scheduleTypeOptions[item.index]
		m.schedule.Type = option.Value
		m.schedule.Date = ""
		m.schedule.Time = ""
		m.schedule.Weekday = ""
		m.schedule.Timezone = ""
		switch option.Value {
		case "once":
			m.startScheduleDateStage()
		case "weekly":
			m.startScheduleWeekdayStage()
		default:
			m.startScheduleTimeStage()
		}
		return nil
	case itemWeekday:
		if item.index < 0 || item.index >= len(weekdayOptions) {
			return nil
		}
		option := weekdayOptions[item.index]
		m.schedule.Weekday = option.Label
		m.startScheduleTimeStage()
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
	case stageScheduleType:
		lines += 5
	case stageScheduleWeekday:
		lines += 6
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
	return truncateToWidth(text, width)
}

func sessionCountLabel(count int) string {
	if count == 1 {
		return "1 session"
	}
	return fmt.Sprintf("%d sessions", count)
}

type scheduleOption struct {
	Value string
	Label string
	Meta  string
}

var scheduleTypeOptions = []scheduleOption{
	{Value: "once", Label: "One-time (pick date and time)", Meta: "once"},
	{Value: "daily", Label: "Daily (pick time)", Meta: "daily"},
	{Value: "weekly", Label: "Weekly (pick day and time)", Meta: "weekly"},
}

var weekdayOptions = []scheduleOption{
	{Value: "monday", Label: "Monday", Meta: "mon"},
	{Value: "tuesday", Label: "Tuesday", Meta: "tue"},
	{Value: "wednesday", Label: "Wednesday", Meta: "wed"},
	{Value: "thursday", Label: "Thursday", Meta: "thu"},
	{Value: "friday", Label: "Friday", Meta: "fri"},
	{Value: "saturday", Label: "Saturday", Meta: "sat"},
	{Value: "sunday", Label: "Sunday", Meta: "sun"},
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

func truncateString(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func isValidDate(value string) bool {
	if value == "" {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func isValidTime(value string) bool {
	if value == "" {
		return false
	}
	_, err := time.Parse("15:04", value)
	return err == nil
}

func renderWidth(width int) int {
	width = safeWidth(width)
	if width <= 1 {
		return width
	}
	return width - 1
}

func safeWidth(width int) int {
	if width <= 0 {
		return 80
	}
	return width
}

func promptHeight(height int) int {
	if height <= 0 {
		return 6
	}
	available := height - (len(asciiArtLines) + 1) - 6
	if available < 3 {
		return 3
	}
	if available > 12 {
		return 12
	}
	return available
}

func normalizeTimeValue(value string) string {
	if len(value) == 5 && value[2] == ':' && isTimeDigits(value) {
		return value
	}

	digits := make([]rune, 0, 4)
	for _, r := range value {
		if r >= '0' && r <= '9' {
			digits = append(digits, r)
		}
		if len(digits) == 4 {
			break
		}
	}
	for len(digits) < 4 {
		digits = append(digits, '0')
	}
	return fmt.Sprintf("%c%c:%c%c", digits[0], digits[1], digits[2], digits[3])
}

func applyTimeMask(value string, pos int, key tea.KeyMsg) (string, int, bool) {
	value = normalizeTimeValue(value)
	pos = clamp(pos, 0, len(value))
	if pos >= len(value) {
		pos = len(value) - 1
	}
	if pos == 2 {
		pos = 3
	}

	switch key.Type {
	case tea.KeyRunes:
		changed := false
		for _, r := range key.Runes {
			if r < '0' || r > '9' {
				continue
			}
			value = setTimeDigit(value, pos, r)
			pos = timeNextPos(pos)
			changed = true
		}
		return value, pos, changed
	case tea.KeyLeft:
		return value, timePrevPos(pos), true
	case tea.KeyRight:
		return value, timeNextPos(pos), true
	case tea.KeyHome:
		return value, 0, true
	case tea.KeyEnd:
		return value, 4, true
	case tea.KeyBackspace:
		pos = timePrevPos(pos)
		value = setTimeDigit(value, pos, '0')
		return value, pos, true
	case tea.KeyDelete:
		value = setTimeDigit(value, pos, '0')
		return value, pos, true
	case tea.KeyCtrlU:
		return "00:00", 0, true
	}

	return value, pos, false
}

func timePrevPos(pos int) int {
	switch pos {
	case 4:
		return 3
	case 3:
		return 1
	case 1:
		return 0
	case 0:
		return 0
	default:
		if pos <= 1 {
			return 0
		}
		if pos <= 3 {
			return 1
		}
		return 3
	}
}

func timeNextPos(pos int) int {
	switch pos {
	case 0:
		return 1
	case 1:
		return 3
	case 3:
		return 4
	case 4:
		return 4
	default:
		if pos <= 0 {
			return 0
		}
		if pos <= 1 {
			return 1
		}
		if pos <= 3 {
			return 3
		}
		return 4
	}
}

func setTimeDigit(value string, pos int, digit rune) string {
	runes := []rune(normalizeTimeValue(value))
	if pos < 0 || pos >= len(runes) || pos == 2 {
		return string(runes)
	}
	runes[pos] = digit
	return string(runes)
}

func isTimeDigits(value string) bool {
	if len(value) != 5 {
		return false
	}
	for i, r := range value {
		if i == 2 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
