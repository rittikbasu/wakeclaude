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
	"wakeclaude/internal/scheduler"
)

type Input struct {
	Projects    []app.Project
	ProjectsErr error
	Schedules   []scheduler.ScheduleEntry
	Logs        []scheduler.LogEntry
	Models      []app.ModelOption
}

type ActionKind int

const (
	ActionNone ActionKind = iota
	ActionSchedule
	ActionEdit
	ActionDelete
	ActionQuit
)

type Action struct {
	Kind       ActionKind
	Draft      *Draft
	ScheduleID string
}

type Draft struct {
	ProjectPath string
	SessionID   string
	SessionPath string
	NewSession  bool
	Model       string
	Permission  string
	Prompt      string
	Schedule    Schedule
}

type Schedule struct {
	Type     string
	Date     string
	Time     string
	Weekday  string
	Timezone string
}

type stage int

const (
	stageMain stage = iota
	stageProjects
	stageSessions
	stageModels
	stagePermissionMode
	stagePrompt
	stageScheduleType
	stageScheduleDate
	stageScheduleWeekday
	stageScheduleTime
	stageScheduleList
	stageLogs
	stageConfirmDelete
)

var ErrUserQuit = errors.New("user quit")

func Run(input Input) (Action, error) {
	m := newModel(input)
	program := tea.NewProgram(m, tea.WithAltScreen())
	final, err := program.Run()
	if err != nil {
		return Action{}, err
	}

	var state model
	switch typed := final.(type) {
	case model:
		state = typed
	case *model:
		state = *typed
	default:
		return Action{}, fmt.Errorf("unexpected model type")
	}
	if state.err != nil {
		return Action{}, state.err
	}

	return state.action, nil
}

type itemKind int

const (
	itemMain itemKind = iota
	itemProject
	itemSession
	itemNewSession
	itemModel
	itemPermissionMode
	itemScheduleType
	itemWeekday
	itemSchedule
	itemLog
	itemConfirm
)

type listItem struct {
	title  string
	meta   string
	detail string
	extra  string
	filter string
	kind   itemKind
	index  int
	pinned bool
}

type model struct {
	stage         stage
	projects      []app.Project
	projectsErr   error
	schedules     []scheduler.ScheduleEntry
	logs          []scheduler.LogEntry
	project       app.Project
	sessions      []app.Session
	selectedSess  *app.Session
	selectedNew   bool
	selectedModel app.ModelOption
	selectedPerm  string
	models        []app.ModelOption

	promptText string
	schedule   Schedule
	inputError string
	editID     string
	pendingDel *scheduler.ScheduleEntry

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

	action Action
	err    error
}

func newModel(input Input) model {
	models := input.Models
	if len(models) == 0 {
		models = []app.ModelOption{{Label: "Default (auto)", Value: "auto"}}
	}

	search := textinput.New()
	search.Prompt = ""
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
		stage:        stageMain,
		projects:     input.Projects,
		projectsErr:  input.ProjectsErr,
		schedules:    input.Schedules,
		logs:         input.Logs,
		models:       models,
		selectedPerm: "acceptEdits",
		searchInput:  search,
		promptInput:  prompt,
		dateInput:    dateInput,
		timeInput:    timeInput,
	}

	m.setMainItems()
	m.applyInputSizing()
	m.searchInput.Blur()
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
	case stageProjects, stageSessions, stageModels, stagePermissionMode, stageScheduleType, stageScheduleWeekday, stageMain, stageScheduleList, stageLogs, stageConfirmDelete:
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
	b.WriteString(renderLine("Schedule prompts to run at specific times", lineWidth))
	b.WriteString("\n")
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
	b.WriteString(clearLine)
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
	b.WriteString(clearLine)
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
	b.WriteString(clearLine)
	b.WriteString("\n")
	if m.inputError != "" {
		b.WriteString(renderLine(fmt.Sprintf("Error: %s", m.inputError), width))
		b.WriteString("\n")
	}
	b.WriteString("enter confirm | esc back | q quit\n")
}

func (m model) renderList(b *strings.Builder, width int) {
	switch m.stage {
	case stageMain:
		b.WriteString(renderLine("What would you like to do?", width))
		b.WriteString("\n")
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
	case stagePermissionMode:
		m.renderContextHeader(b, width)
		b.WriteString(renderLine("Select a permission mode.", width))
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
	case stageScheduleList:
		b.WriteString(renderLine("Scheduled prompts.", width))
		b.WriteString("\n")
	case stageLogs:
		b.WriteString(renderLine("Run logs.", width))
		b.WriteString("\n")
	case stageConfirmDelete:
		if m.pendingDel != nil {
			b.WriteString(renderLine("Delete scheduled prompt?", width))
			b.WriteString("\n")
			b.WriteString(renderLine(fmt.Sprintf("%s", scheduler.Preview(m.pendingDel.Prompt, 80)), width))
			b.WriteString("\n")
		}
	}

	if m.projectsErr != nil && m.stage == stageMain {
		b.WriteString(renderLine(fmt.Sprintf("Notice: %s", m.projectsErr.Error()), width))
		b.WriteString("\n")
	}
	if m.inputError != "" && m.stage == stageMain {
		b.WriteString(renderLine(fmt.Sprintf("Error: %s", m.inputError), width))
		b.WriteString("\n")
	}

	if m.usesSearch() {
		b.WriteString(searchLabel)
		b.WriteString(m.searchInput.View())
		b.WriteString(clearLine)
		b.WriteString("\n")
	}
	b.WriteString(strings.Repeat("-", max(10, min(width, 60))))
	b.WriteString("\n")

	if len(m.items) == 0 {
		empty := "No matches."
		if m.stage == stageScheduleList {
			empty = "No active schedules."
		} else if m.stage == stageLogs {
			empty = "No logs yet."
		}
		b.WriteString(renderLine(empty, width))
		b.WriteString("\n")
	} else {
		start, end := m.visibleRange()
		for i := start; i < end; i++ {
			selected := i == m.cursor
			switch m.stage {
			case stageScheduleList:
				renderMultilineItem(b, m.items[i], selected, width, 2)
			case stagePermissionMode:
				metaWidth := maxMetaWidth(m.items, 18)
				b.WriteString(renderItemWithMetaWidth(m.items[i], selected, width, metaWidth))
				b.WriteString("\n")
			default:
				b.WriteString(renderItem(m.items[i], selected, width))
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	if m.stage == stageLogs {
		m.renderLogCommands(b, width)
	}
	if m.stage == stagePermissionMode {
		m.renderPermissionHelp(b, width)
	}
	b.WriteString(m.footerHint())
	b.WriteString("\n")
}

func (m model) footerHint() string {
	switch m.stage {
	case stageMain:
		return "enter select | q quit"
	case stageScheduleList:
		return "enter edit | d delete | esc back | q quit"
	case stageLogs:
		return "esc back | q quit"
	case stageConfirmDelete:
		return "enter confirm | esc back | q quit"
	default:
		return "up/down move | enter select | esc back | q quit"
	}
}

func (m model) renderLogCommands(b *strings.Builder, width int) {
	if len(m.items) == 0 {
		return
	}
	item := m.items[m.cursor]
	if item.kind != itemLog || item.index < 0 || item.index >= len(m.logs) {
		return
	}
	entry := m.logs[item.index]
	if entry.Status != "success" && entry.OutputPath != "" {
		b.WriteString(renderWrappedPath("Output: cat ", entry.OutputPath, width))
		b.WriteString("\n")
	}
	if entry.SessionID != "" {
		line := fmt.Sprintf("Resume: claude --resume %s", entry.SessionID)
		b.WriteString(renderWrappedLines(line, width, len("Resume: ")))
		b.WriteString("\n")
	}
	project := m.logProjectPath(entry)
	if project == "" {
		project = "(unknown)"
	} else {
		project = app.HumanizePath(project)
	}
	b.WriteString(renderWrappedPath("Project: ", project, width))
	b.WriteString("\n")
}

func (m model) renderPermissionHelp(b *strings.Builder, width int) {
	if len(m.items) == 0 {
		return
	}
	item := m.items[m.cursor]
	if item.kind != itemPermissionMode || item.index < 0 || item.index >= len(permissionModeOptions) {
		return
	}
	desc := permissionModeOptions[item.index].Desc
	if desc == "" {
		return
	}
	b.WriteString(renderLine(fmt.Sprintf("Mode: %s", desc), width))
	b.WriteString("\n")
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
	if m.project.Path != "" {
		return app.HumanizePath(m.project.Path)
	}
	return ""
}

func (m model) sessionLabel() string {
	if m.selectedSess != nil {
		if m.selectedSess.Preview != "" {
			return m.selectedSess.Preview
		}
		return m.selectedSess.ID
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

func (m *model) setMainItems() {
	m.inputError = ""
	m.searchInput.SetValue("")
	m.searchInput.Blur()
	m.editID = ""
	m.pendingDel = nil

	options := mainOptions
	items := make([]listItem, 0, len(options))
	for i, option := range options {
		items = append(items, listItem{
			title:  option.Label,
			meta:   option.Meta,
			filter: strings.ToLower(option.Label + " " + option.Meta),
			kind:   itemMain,
			index:  i,
		})
	}
	m.all = items
	m.applyFilter()
}

func (m *model) setProjectItems() {
	m.selectedSess = nil
	m.selectedNew = false
	m.selectedModel = app.ModelOption{}
	m.selectedPerm = "acceptEdits"
	m.promptText = ""
	m.inputError = ""
	m.schedule = Schedule{}
	m.pendingDel = nil
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	m.promptInput.SetValue("")
	m.dateInput.SetValue("")
	m.timeInput.SetValue("")
	items := make([]listItem, 0, len(m.projects))
	for i, project := range m.projects {
		display := project.DisplayName
		if display == "" {
			display = app.HumanizePath(project.Path)
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
	m.selectModelCursor()
}

func (m *model) setPermissionModeItems() {
	m.inputError = ""
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	items := make([]listItem, 0, len(permissionModeOptions))
	for i, option := range permissionModeOptions {
		items = append(items, listItem{
			title:  option.Label,
			meta:   option.Value,
			filter: strings.ToLower(option.Label + " " + option.Value),
			kind:   itemPermissionMode,
			index:  i,
		})
	}
	m.all = items
	m.applyFilter()
	m.selectPermissionCursor()
}

func (m *model) setScheduleTypeItems() {
	m.inputError = ""
	m.searchInput.SetValue("")
	m.searchInput.Focus()
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
	m.selectScheduleTypeCursor()
}

func (m *model) setWeekdayItems() {
	m.inputError = ""
	m.searchInput.SetValue("")
	m.searchInput.Focus()
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
	m.selectWeekdayCursor()
}

func (m *model) setScheduleItems() {
	m.inputError = ""
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	items := make([]listItem, 0, len(m.schedules))
	now := time.Now()
	for i, entry := range m.schedules {
		if _, ok := nextRunForList(entry, now); !ok {
			continue
		}
		preview := scheduler.Preview(entry.Prompt, 200)
		if preview == "" {
			preview = "(no prompt)"
		}
		scheduleLabel := formatScheduleLabel(entry)
		addedLabel := formatAdded(entry.CreatedAt, now)
		project := app.HumanizePath(entry.ProjectPath)
		if project == "" {
			project = "(no path)"
		}
		title := fmt.Sprintf("%s · %s", addedLabel, scheduleLabel)
		if project != "" {
			title = fmt.Sprintf("%s · %s", title, project)
		}
		filter := strings.ToLower(strings.Join([]string{preview, scheduleLabel, project, entry.ID}, " "))
		items = append(items, listItem{
			title:  title,
			detail: preview,
			filter: filter,
			kind:   itemSchedule,
			index:  i,
		})
	}
	m.all = items
	m.applyFilter()
}

func (m *model) setLogItems() {
	m.inputError = ""
	m.searchInput.SetValue("")
	m.searchInput.Focus()
	items := make([]listItem, 0, len(m.logs))
	now := time.Now()
	for i, entry := range m.logs {
		preview := entry.PromptPreview
		if preview == "" {
			preview = "(no prompt)"
		}
		runMsg := formatRunMessage(entry)
		when := scheduler.RelativeLabel(entry.RanAt, now)
		project := entry.ProjectPath
		if project == "" {
			project = m.logProjectPath(entry)
		}
		title := preview
		if runMsg != "" {
			title = fmt.Sprintf("%s · %s", runMsg, preview)
		}
		filter := strings.ToLower(strings.Join([]string{preview, entry.Status, entry.Model, entry.ScheduleID, entry.SessionID, project}, " "))
		items = append(items, listItem{
			meta:   when,
			title:  title,
			filter: filter,
			kind:   itemLog,
			index:  i,
		})
	}
	m.all = items
	m.applyFilter()
}

func (m *model) setConfirmDeleteItems() {
	items := []listItem{
		{title: "Delete this schedule", meta: "delete", filter: "delete", kind: itemConfirm, index: 0},
		{title: "Cancel", meta: "cancel", filter: "cancel", kind: itemConfirm, index: 1},
	}
	m.all = items
	m.applyFilter()
}

func (m *model) selectScheduleTypeCursor() {
	if m.schedule.Type == "" {
		return
	}
	for i, item := range m.items {
		if item.kind != itemScheduleType {
			continue
		}
		if item.index >= 0 && item.index < len(scheduleTypeOptions) {
			if scheduleTypeOptions[item.index].Value == m.schedule.Type {
				m.cursor = i
				m.ensureCursorVisible()
				return
			}
		}
	}
}

func (m *model) selectWeekdayCursor() {
	if m.schedule.Weekday == "" {
		return
	}
	for i, item := range m.items {
		if item.kind != itemWeekday {
			continue
		}
		if item.index >= 0 && item.index < len(weekdayOptions) {
			if weekdayOptions[item.index].Label == m.schedule.Weekday {
				m.cursor = i
				m.ensureCursorVisible()
				return
			}
		}
	}
}

func (m *model) selectModelCursor() {
	if m.selectedModel.Value == "" {
		return
	}
	for i, item := range m.items {
		if item.kind != itemModel {
			continue
		}
		if item.index >= 0 && item.index < len(m.models) {
			if m.models[item.index].Value == m.selectedModel.Value {
				m.cursor = i
				m.ensureCursorVisible()
				return
			}
		}
	}
}

func (m *model) selectPermissionCursor() {
	if m.selectedPerm == "" {
		return
	}
	for i, item := range m.items {
		if item.kind != itemPermissionMode {
			continue
		}
		if item.index >= 0 && item.index < len(permissionModeOptions) {
			if permissionModeOptions[item.index].Value == m.selectedPerm {
				m.cursor = i
				m.ensureCursorVisible()
				return
			}
		}
	}
}

func (m *model) handleBack() (tea.Model, tea.Cmd) {
	if m.editID != "" {
		switch m.stage {
		case stagePrompt, stageModels, stagePermissionMode, stageSessions, stageProjects:
			m.editID = ""
			m.stage = stageScheduleList
			m.pendingDel = nil
			m.setScheduleItems()
			return m, nil
		}
	}
	switch m.stage {
	case stageProjects:
		m.startMainStage()
		return m, nil
	case stageSessions:
		m.stage = stageProjects
		m.project = app.Project{}
		m.sessions = nil
		m.selectedSess = nil
		m.selectedNew = false
		m.selectedModel = app.ModelOption{}
		m.setProjectItems()
		return m, nil
	case stageModels:
		m.stage = stagePrompt
		m.promptInput.SetValue(m.promptText)
		m.promptInput.Focus()
		return m, nil
	case stagePermissionMode:
		m.stage = stageModels
		m.searchInput.Focus()
		m.setModelItems()
		return m, nil
	case stagePrompt:
		m.stage = stageSessions
		m.promptText = strings.TrimSpace(m.promptInput.Value())
		m.promptInput.Blur()
		m.setSessionItems()
		return m, nil
	case stageScheduleType:
		m.startPermissionModeStage()
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
	case stageScheduleList, stageLogs:
		m.startMainStage()
		return m, nil
	case stageConfirmDelete:
		m.stage = stageScheduleList
		m.pendingDel = nil
		m.setScheduleItems()
		return m, nil
	case stageMain:
		m.err = ErrUserQuit
		return m, tea.Quit
	default:
		m.err = ErrUserQuit
		return m, tea.Quit
	}
}

func (m *model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		allowJK := !(m.usesSearch() && m.searchInput.Focused())
		switch msg.String() {
		case "enter":
			return m, m.selectCurrent()
		case "up":
			m.moveCursor(-1)
			return m, nil
		case "down":
			m.moveCursor(1)
			return m, nil
		case "k":
			if allowJK {
				m.moveCursor(-1)
				return m, nil
			}
		case "j":
			if allowJK {
				m.moveCursor(1)
				return m, nil
			}
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
		case "d":
			if m.stage == stageScheduleList {
				return m, m.beginDelete()
			}
		}
	}

	if !m.usesSearch() {
		return m, nil
	}
	prev := m.searchInput.Value()
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	if m.searchInput.Value() != prev {
		m.applyFilter()
	}
	return m, cmd
}

func (m *model) beginDelete() tea.Cmd {
	if len(m.items) == 0 {
		return nil
	}
	item := m.items[m.cursor]
	if item.kind != itemSchedule {
		return nil
	}
	if item.index < 0 || item.index >= len(m.schedules) {
		return nil
	}
	entry := m.schedules[item.index]
	m.pendingDel = &entry
	m.stage = stageConfirmDelete
	m.searchInput.SetValue("")
	m.searchInput.Blur()
	m.setConfirmDeleteItems()
	return nil
}

func (m *model) applyInputSizing() {
	width := renderWidth(m.width)
	if width <= 0 {
		width = 80
	}
	m.searchInput.Width = max(10, width-len(searchLabel))
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
		m.startModelStage()
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

func (m *model) startMainStage() {
	m.stage = stageMain
	m.inputError = ""
	m.searchInput.Blur()
	m.promptInput.Blur()
	m.dateInput.Blur()
	m.timeInput.Blur()
	m.setMainItems()
}

func (m *model) startProjectStage() {
	m.stage = stageProjects
	m.inputError = ""
	m.editID = ""
	m.pendingDel = nil
	m.searchInput.Focus()
	m.promptInput.Blur()
	m.dateInput.Blur()
	m.timeInput.Blur()
	m.setProjectItems()
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

func (m *model) startModelStage() {
	m.stage = stageModels
	m.inputError = ""
	m.promptInput.Blur()
	m.searchInput.Focus()
	m.setModelItems()
}

func (m *model) startPermissionModeStage() {
	m.stage = stagePermissionMode
	m.inputError = ""
	m.promptInput.Blur()
	m.searchInput.Focus()
	m.setPermissionModeItems()
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

func (m *model) startScheduleListStage() {
	m.stage = stageScheduleList
	m.inputError = ""
	m.searchInput.Focus()
	m.setScheduleItems()
}

func (m *model) startLogsStage() {
	m.stage = stageLogs
	m.inputError = ""
	m.searchInput.Focus()
	m.setLogItems()
}

func (m *model) startEditFlow(entry scheduler.ScheduleEntry) {
	m.editID = entry.ID
	m.project = m.findProject(entry.ProjectPath)
	if m.project.Path == "" {
		m.project = app.Project{Path: entry.ProjectPath, DisplayName: app.HumanizePath(entry.ProjectPath)}
	}

	sessions, err := app.ListSessions(entry.ProjectPath)
	if err == nil {
		m.sessions = sessions
	}

	m.selectedNew = entry.NewSession
	m.selectedSess = nil
	if !entry.NewSession && entry.SessionID != "" {
		for i := range m.sessions {
			if m.sessions[i].ID == entry.SessionID {
				m.selectedSess = &m.sessions[i]
				break
			}
		}
		if m.selectedSess == nil {
			m.selectedSess = &app.Session{ID: entry.SessionID, Path: entry.SessionPath, Preview: entry.SessionID}
		}
	}

	m.selectedModel = m.findModel(entry.Model)
	if entry.PermissionMode != "" && entry.PermissionMode != "default" {
		m.selectedPerm = entry.PermissionMode
	} else {
		m.selectedPerm = "acceptEdits"
	}
	m.promptText = entry.Prompt
	m.schedule = Schedule{
		Type:     entry.Schedule.Type,
		Date:     entry.Schedule.Date,
		Time:     entry.Schedule.Time,
		Weekday:  entry.Schedule.Weekday,
		Timezone: entry.Timezone,
	}

	m.promptInput.SetValue(entry.Prompt)
	m.startPromptStage()
}

func (m *model) finishResult() {
	projectPath := m.project.CWD
	if projectPath == "" {
		projectPath = m.project.Path
	}
	draft := &Draft{
		ProjectPath: projectPath,
		Model:       m.selectedModel.Value,
		Permission:  m.selectedPerm,
		Prompt:      m.promptText,
		Schedule:    m.schedule,
	}
	if m.selectedNew {
		draft.NewSession = true
	} else if m.selectedSess != nil {
		draft.SessionID = m.selectedSess.ID
		draft.SessionPath = m.selectedSess.Path
	}

	kind := ActionSchedule
	if m.editID != "" {
		kind = ActionEdit
	}
	m.action = Action{
		Kind:       kind,
		Draft:      draft,
		ScheduleID: m.editID,
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
	case itemMain:
		switch item.index {
		case 0:
			if m.projectsErr != nil || len(m.projects) == 0 {
				m.inputError = "No Claude projects found. Run Claude once to create them."
				return nil
			}
			m.startProjectStage()
			return nil
		case 1:
			m.startScheduleListStage()
			return nil
		case 2:
			m.startLogsStage()
			return nil
		case 3:
			m.err = ErrUserQuit
			return tea.Quit
		}
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
		m.selectedSess = nil
		m.selectedNew = true
		m.startPromptStage()
		return nil
	case itemSession:
		session := m.sessions[item.index]
		m.selectedSess = &session
		m.selectedNew = false
		m.startPromptStage()
		return nil
	case itemModel:
		if item.index < 0 || item.index >= len(m.models) {
			return nil
		}
		option := m.models[item.index]
		m.selectedModel = option
		m.startPermissionModeStage()
		return nil
	case itemPermissionMode:
		if item.index < 0 || item.index >= len(permissionModeOptions) {
			return nil
		}
		option := permissionModeOptions[item.index]
		m.selectedPerm = option.Value
		m.startScheduleTypeStage()
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
	case itemSchedule:
		if item.index < 0 || item.index >= len(m.schedules) {
			return nil
		}
		entry := m.schedules[item.index]
		m.startEditFlow(entry)
		return nil
	case itemConfirm:
		if item.index == 0 && m.pendingDel != nil {
			m.action = Action{
				Kind:       ActionDelete,
				ScheduleID: m.pendingDel.ID,
			}
			return tea.Quit
		}
		m.stage = stageScheduleList
		m.pendingDel = nil
		m.setScheduleItems()
		return nil
	default:
		return nil
	}

	return nil
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
	available := m.height - m.headerLines()
	if available < 3 {
		return 3
	}
	lines := m.itemLines()
	if lines <= 0 {
		lines = 1
	}
	return max(1, available/lines)
}

func (m model) headerLines() int {
	lines := len(asciiArtLines) + 2
	switch m.stage {
	case stageMain:
		lines += 1
	case stageProjects:
		lines += 1
	case stageSessions:
		lines += 2
	case stageModels:
		lines += 3
	case stagePermissionMode:
		lines += 5
	case stageScheduleType:
		lines += 5
	case stageScheduleWeekday:
		lines += 6
	case stageScheduleList:
		lines += 1
	case stageLogs:
		lines += 2
	case stageConfirmDelete:
		lines += 2
	default:
		lines += 1
	}
	if m.projectsErr != nil && m.stage == stageMain {
		lines += 1
	}
	if m.usesSearch() {
		lines += 2
	} else {
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

func (m model) itemLines() int {
	switch m.stage {
	case stageScheduleList:
		return 2
	default:
		return 1
	}
}

func (m model) usesSearch() bool {
	switch m.stage {
	case stageProjects, stageSessions, stageScheduleList, stageLogs:
		return true
	case stageMain, stageConfirmDelete:
		return false
	case stagePrompt, stageScheduleDate, stageScheduleTime:
		return false
	default:
		return false
	}
}

func renderItem(item listItem, selected bool, width int) string {
	prefix := "  "
	if selected {
		prefix = "> "
	}

	line := fmt.Sprintf("%s%-10s %s", prefix, item.meta, item.title)
	return renderLine(line, width)
}

func renderItemWithMetaWidth(item listItem, selected bool, width int, metaWidth int) string {
	if metaWidth <= 0 {
		return renderItem(item, selected, width)
	}
	prefix := "  "
	if selected {
		prefix = "> "
	}
	format := fmt.Sprintf("%%s%%-%ds %%s", metaWidth)
	line := fmt.Sprintf(format, prefix, item.meta, item.title)
	return renderLine(line, width)
}

func maxMetaWidth(items []listItem, maxWidth int) int {
	width := 0
	for _, item := range items {
		if item.meta == "" {
			continue
		}
		l := len([]rune(item.meta))
		if l > width {
			width = l
		}
	}
	if width < 6 {
		width = 6
	}
	if maxWidth > 0 && width > maxWidth {
		return maxWidth
	}
	return width
}

func renderMultilineItem(b *strings.Builder, item listItem, selected bool, width int, lines int) {
	content := []string{item.title}
	if lines >= 2 {
		content = append(content, item.detail)
	}
	if lines >= 3 {
		content = append(content, item.extra)
	}
	for i, line := range content {
		prefix := "  "
		if i == 0 && selected {
			prefix = "> "
		}
		b.WriteString(renderLine(prefix+line, width))
		b.WriteString("\n")
	}
}

func renderLine(text string, width int) string {
	return truncateToWidth(text, width) + clearLine
}

func renderWrappedLines(text string, width int, indent int) string {
	lines := wrapWithIndent(text, width, indent)
	if len(lines) == 0 {
		return renderLine("", width)
	}
	var b strings.Builder
	for i, line := range lines {
		prefix := ""
		if i > 0 && indent > 0 {
			prefix = strings.Repeat(" ", indent)
		}
		b.WriteString(renderLine(prefix+line, width))
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderWrappedPath(prefix, path string, width int) string {
	if width <= 0 {
		width = 80
	}
	if path == "" {
		return renderLine(prefix, width)
	}
	indent := len([]rune(prefix))
	if indent >= width {
		return renderLine(prefix+path, width)
	}

	tokens := splitPathTokens(path)
	if len(tokens) == 0 {
		return renderLine(prefix+path, width)
	}

	lines := make([]string, 0, 3)
	current := prefix
	curLen := len([]rune(current))

	for _, tok := range tokens {
		tokRunes := []rune(tok)
		tokLen := len(tokRunes)
		if curLen+tokLen <= width {
			current += tok
			curLen += tokLen
			continue
		}
		if strings.TrimSpace(current) != "" {
			lines = append(lines, current)
			current = strings.Repeat(" ", indent)
			curLen = indent
		}
		for tokLen > 0 {
			remaining := width - curLen
			if remaining <= 0 {
				lines = append(lines, current)
				current = strings.Repeat(" ", indent)
				curLen = indent
				remaining = width - curLen
				if remaining <= 0 {
					remaining = width
				}
			}
			if tokLen <= remaining {
				current += string(tokRunes)
				curLen += tokLen
				tokLen = 0
				break
			}
			current += string(tokRunes[:remaining])
			curLen += remaining
			tokRunes = tokRunes[remaining:]
			tokLen = len(tokRunes)
			lines = append(lines, current)
			current = strings.Repeat(" ", indent)
			curLen = indent
		}
	}
	if strings.TrimSpace(current) != "" {
		lines = append(lines, current)
	}

	var b strings.Builder
	for i, line := range lines {
		b.WriteString(renderLine(line, width))
		if i < len(lines)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func splitPathTokens(path string) []string {
	if path == "" {
		return nil
	}
	if path == "/" {
		return []string{"/"}
	}
	abs := strings.HasPrefix(path, "/")
	parts := strings.Split(path, "/")
	tokens := make([]string, 0, len(parts))
	first := true
	for _, part := range parts {
		if part == "" {
			continue
		}
		if first && !abs {
			tokens = append(tokens, part)
		} else {
			tokens = append(tokens, "/"+part)
		}
		first = false
	}
	return tokens
}

func wrapWithIndent(text string, width int, indent int) []string {
	if width <= 0 {
		width = 80
	}
	if indent < 0 {
		indent = 0
	}
	if indent >= width {
		indent = 0
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	firstLimit := width
	nextLimit := width - indent
	if nextLimit < 10 {
		nextLimit = width
	}

	limit := firstLimit
	usingFirst := true
	lines := make([]string, 0, 4)
	line := ""
	lineLen := 0

	flush := func() {
		if lineLen > 0 {
			lines = append(lines, line)
			line = ""
			lineLen = 0
			usingFirst = false
			limit = nextLimit
		}
	}

	for _, word := range words {
		wordRunes := []rune(word)
		for len(wordRunes) > limit {
			if lineLen > 0 {
				flush()
			}
			lines = append(lines, string(wordRunes[:limit]))
			wordRunes = wordRunes[limit:]
			usingFirst = false
			limit = nextLimit
		}
		w := string(wordRunes)
		wLen := len([]rune(w))
		if lineLen == 0 {
			line = w
			lineLen = wLen
			continue
		}
		if lineLen+1+wLen <= limit {
			line = line + " " + w
			lineLen += 1 + wLen
			continue
		}
		flush()
		if usingFirst {
			limit = firstLimit
		}
		line = w
		lineLen = wLen
	}
	if lineLen > 0 {
		lines = append(lines, line)
	}
	return lines
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

type permissionModeOption struct {
	Value string
	Label string
	Desc  string
}

type mainOption struct {
	Label string
	Meta  string
}

var mainOptions = []mainOption{
	{Label: "Schedule a prompt", Meta: "new"},
	{Label: "Manage scheduled prompts", Meta: "list"},
	{Label: "View run logs", Meta: "logs"},
	{Label: "Quit", Meta: "exit"},
}

var scheduleTypeOptions = []scheduleOption{
	{Value: "once", Label: "One-time (pick date and time)", Meta: "once"},
	{Value: "daily", Label: "Daily (pick time)", Meta: "daily"},
	{Value: "weekly", Label: "Weekly (pick day and time)", Meta: "weekly"},
}

var permissionModeOptions = []permissionModeOption{
	{
		Value: "acceptEdits",
		Label: "Accept edits",
		Desc:  "Auto-accept file edits and filesystem access.",
	},
	{
		Value: "plan",
		Label: "Plan only",
		Desc:  "Read-only; no commands or file changes.",
	},
	{
		Value: "bypassPermissions",
		Label: "Bypass permissions",
		Desc:  "Skip all permission prompts (advanced).",
	},
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

func formatScheduleLabel(entry scheduler.ScheduleEntry) string {
	switch entry.Schedule.Type {
	case "daily":
		if entry.Schedule.Time != "" {
			return fmt.Sprintf("Daily %s", entry.Schedule.Time)
		}
		return "Daily"
	case "weekly":
		if entry.Schedule.Time != "" && entry.Schedule.Weekday != "" {
			return fmt.Sprintf("Weekly %s %s", entry.Schedule.Weekday, entry.Schedule.Time)
		}
		if entry.Schedule.Weekday != "" {
			return fmt.Sprintf("Weekly %s", entry.Schedule.Weekday)
		}
		return "Weekly"
	case "once":
		if entry.Schedule.Date != "" && entry.Schedule.Time != "" {
			return fmt.Sprintf("Once %s %s", entry.Schedule.Date, entry.Schedule.Time)
		}
		return "Once"
	default:
		return "Schedule"
	}
}

func formatAdded(t time.Time, now time.Time) string {
	if t.IsZero() {
		return "Added"
	}
	layout := "Jan 02 15:04"
	if t.Year() != now.Year() {
		layout = "Jan 02 2006"
	}
	return fmt.Sprintf("Added %s", t.Format(layout))
}

func nextRunForList(entry scheduler.ScheduleEntry, now time.Time) (time.Time, bool) {
	if !entry.NextRun.IsZero() && entry.NextRun.After(now) {
		return entry.NextRun, true
	}
	next, err := scheduler.NextRun(entry, now)
	if err != nil {
		return time.Time{}, false
	}
	return next, next.After(now)
}

func formatRunMessage(entry scheduler.LogEntry) string {
	status := strings.ToUpper(entry.Status)
	if status == "" {
		status = "UNKNOWN"
	}
	if entry.Status == "success" {
		return "OK"
	}
	if entry.Error != "" {
		return fmt.Sprintf("ERROR: %s", truncateString(entry.Error, 60))
	}
	return status
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

func (m *model) findProject(path string) app.Project {
	for _, project := range m.projects {
		if project.Path == path {
			return project
		}
	}
	return app.Project{}
}

func (m *model) logProjectPath(entry scheduler.LogEntry) string {
	if entry.ProjectPath != "" {
		return entry.ProjectPath
	}
	for _, schedule := range m.schedules {
		if schedule.ID == entry.ScheduleID {
			return schedule.ProjectPath
		}
	}
	return ""
}

func (m *model) findModel(value string) app.ModelOption {
	for _, option := range m.models {
		if option.Value == value {
			return option
		}
	}
	if len(m.models) > 0 {
		return m.models[0]
	}
	return app.ModelOption{Value: "auto", Label: "Default (auto)"}
}

var asciiArtLines = []string{
	"              _          _              _     ",
	" __ __ ____ _| |_____ __| |__ _ _  _ __| |___ ",
	" \\ V  V / _` | / / -_/ _| / _` | || / _` / -_)",
	"  \\_/\\_/\\__,_|_\\_\\___\\__|_\\__,_|\\_,_\\__,_\\___|",
	"                                              ",
	"",
}

const (
	clearLine   = "\x1b[0K"
	searchLabel = "Search: "
)

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
