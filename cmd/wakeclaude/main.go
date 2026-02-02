package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"wakeclaude/internal/app"
	"wakeclaude/internal/scheduler"
	"wakeclaude/internal/tui"
)

func main() {
	fs := flag.NewFlagSet("wakeclaude", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var projectsRoot string
	var runID string
	var showHelp bool
	fs.StringVar(&projectsRoot, "projects-root", "", "Root directory for Claude projects (default: ~/.claude/projects)")
	fs.StringVar(&runID, "run", "", "Run a scheduled job by id (internal)")
	fs.BoolVar(&showHelp, "help", false, "Show help")
	fs.BoolVar(&showHelp, "h", false, "Show help")

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if showHelp {
		printUsage()
		return
	}
	if fs.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "wakeclaude does not accept positional arguments.")
		printUsage()
		os.Exit(2)
	}

	store, err := scheduler.DefaultStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if runID != "" {
		if err := scheduler.RunSchedule(store, runID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	projects, projectsErr := app.DiscoverProjects(projectsRoot)

	schedules, err := store.LoadSchedules()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sort.Slice(schedules, func(i, j int) bool {
		if schedules[i].NextRun.Equal(schedules[j].NextRun) {
			return schedules[i].CreatedAt.Before(schedules[j].CreatedAt)
		}
		if schedules[i].NextRun.IsZero() {
			return false
		}
		if schedules[j].NextRun.IsZero() {
			return true
		}
		return schedules[i].NextRun.Before(schedules[j].NextRun)
	})

	logs, err := store.LoadLogs(scheduler.MaxRunLogs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	claudeReady := app.ClaudeAvailable()
	tokenReady := false
	tokenErr := ""
	if claudeReady {
		if token, err := app.LoadOAuthToken(); err == nil && token != "" {
			tokenReady = true
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			tokenErr = err.Error()
		}
	}

	models := []app.ModelOption{
		{Label: "Default (auto)", Value: "auto"},
		{Label: "Opus", Value: "opus"},
		{Label: "Sonnet", Value: "sonnet"},
		{Label: "Haiku", Value: "haiku"},
	}

	action, err := tui.Run(tui.Input{
		Projects:    projects,
		ProjectsErr: projectsErr,
		Schedules:   schedules,
		Logs:        logs,
		Models:      models,
		ClaudeReady: claudeReady,
		InstallCmd:  app.ClaudeInstallCmd,
		TokenReady:  tokenReady,
		TokenErr:    tokenErr,
		SetupCmd:    app.ClaudeSetupTokenCmd,
	})
	if err != nil {
		if errors.Is(err, tui.ErrUserQuit) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	switch action.Kind {
	case tui.ActionSchedule:
		entry, err := buildEntry(action.Draft, nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := scheduler.EnsureSudo(); err != nil {
			fmt.Fprintln(os.Stderr, "sudo required to schedule wakeclaude")
			os.Exit(1)
		}
		if _, err := store.AddSchedule(entry); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := scheduler.EnsureLaunchd(entry); err != nil {
			_, _ = store.DeleteSchedule(entry.ID)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := scheduler.ScheduleWake(entry, entry.WakeTime); err != nil {
			_, _ = store.DeleteSchedule(entry.ID)
			_ = scheduler.RemoveLaunchd(entry)
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printScheduled(entry)
	case tui.ActionEdit:
		if action.ScheduleID == "" {
			fmt.Fprintln(os.Stderr, "missing schedule id")
			os.Exit(1)
		}
		current, ok := findSchedule(schedules, action.ScheduleID)
		if !ok {
			fmt.Fprintln(os.Stderr, "schedule not found")
			os.Exit(1)
		}
		entry, err := buildEntry(action.Draft, &current)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := scheduler.EnsureSudo(); err != nil {
			fmt.Fprintln(os.Stderr, "sudo required to update wakeclaude")
			os.Exit(1)
		}
		_ = scheduler.RemoveLaunchd(current)
		if err := scheduler.CancelWake(current); err != nil {
			fmt.Fprintln(os.Stderr, "warning: failed to cancel previous wake schedule:", err)
		}
		if err := store.UpdateSchedule(entry); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := scheduler.EnsureLaunchd(entry); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := scheduler.ScheduleWake(entry, entry.WakeTime); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printUpdated(entry)
	case tui.ActionDelete:
		if action.ScheduleID == "" {
			fmt.Fprintln(os.Stderr, "missing schedule id")
			os.Exit(1)
		}
		current, ok := findSchedule(schedules, action.ScheduleID)
		if !ok {
			fmt.Fprintln(os.Stderr, "schedule not found")
			os.Exit(1)
		}
		if err := scheduler.EnsureSudo(); err != nil {
			fmt.Fprintln(os.Stderr, "sudo required to delete wakeclaude schedule")
			os.Exit(1)
		}
		_ = scheduler.RemoveLaunchd(current)
		if err := scheduler.CancelWake(current); err != nil {
			fmt.Fprintln(os.Stderr, "warning: failed to cancel wake schedule:", err)
		}
		if _, err := store.DeleteSchedule(current.ID); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		printDeleted(current)
	default:
		return
	}
}

func buildEntry(draft *tui.Draft, existing *scheduler.ScheduleEntry) (scheduler.ScheduleEntry, error) {
	if draft == nil {
		return scheduler.ScheduleEntry{}, fmt.Errorf("missing schedule details")
	}

	now := time.Now()
	id := ""
	created := now
	if existing != nil {
		id = existing.ID
		if !existing.CreatedAt.IsZero() {
			created = existing.CreatedAt
		}
	}
	if id == "" {
		id = scheduler.NewID()
	}

	exe, err := os.Executable()
	if err != nil {
		return scheduler.ScheduleEntry{}, fmt.Errorf("resolve wakeclaude path: %w", err)
	}
	exe, _ = filepath.Abs(exe)

	usr, _ := user.Current()
	username := os.Getenv("USER")
	if usr != nil && usr.Username != "" {
		username = usr.Username
	}
	uid := os.Getuid()
	gid := os.Getgid()
	if usr != nil {
		if parsed, err := strconv.Atoi(usr.Uid); err == nil {
			uid = parsed
		}
		if parsed, err := strconv.Atoi(usr.Gid); err == nil {
			gid = parsed
		}
	}

	home, _ := os.UserHomeDir()
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" && existing != nil {
		pathEnv = existing.PathEnv
	}
	if pathEnv == "" {
		pathEnv = "/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}

	model := strings.TrimSpace(draft.Model)
	if model == "" {
		model = "auto"
	}
	perm := strings.TrimSpace(draft.Permission)
	if perm == "" {
		perm = "acceptEdits"
	}

	entry := scheduler.ScheduleEntry{
		ID:             id,
		ProjectPath:    draft.ProjectPath,
		SessionID:      draft.SessionID,
		SessionPath:    draft.SessionPath,
		NewSession:     draft.NewSession,
		Model:          model,
		PermissionMode: perm,
		Prompt:         strings.TrimSpace(draft.Prompt),
		Schedule: scheduler.Schedule{
			Type:    draft.Schedule.Type,
			Date:    draft.Schedule.Date,
			Time:    draft.Schedule.Time,
			Weekday: draft.Schedule.Weekday,
		},
		Timezone:   draft.Schedule.Timezone,
		CreatedAt:  created,
		UpdatedAt:  now,
		BinaryPath: exe,
		User:       username,
		UID:        uid,
		GID:        gid,
		HomeDir:    home,
		PathEnv:    pathEnv,
	}

	if existing != nil {
		if entry.Timezone == "" {
			entry.Timezone = existing.Timezone
		}
		if entry.PermissionMode == "" {
			entry.PermissionMode = existing.PermissionMode
		}
		if entry.User == "" {
			entry.User = existing.User
		}
		if entry.HomeDir == "" {
			entry.HomeDir = existing.HomeDir
		}
		if entry.PathEnv == "" {
			entry.PathEnv = existing.PathEnv
		}
	}

	nextRun, err := scheduler.NextRun(entry, now)
	if err != nil {
		return scheduler.ScheduleEntry{}, err
	}
	entry.NextRun = nextRun
	entry.WakeTime = scheduler.FormatPMSet(nextRun)

	return entry, nil
}

func findSchedule(list []scheduler.ScheduleEntry, id string) (scheduler.ScheduleEntry, bool) {
	for _, entry := range list {
		if entry.ID == id {
			return entry, true
		}
	}
	return scheduler.ScheduleEntry{}, false
}

func printScheduled(entry scheduler.ScheduleEntry) {
	fmt.Println("Scheduled.")
	fmt.Printf("ID: %s\n", entry.ID)
	fmt.Printf("Next run: %s (%s)\n", entry.NextRun.Format(time.RFC1123), scheduler.RelativeLabel(entry.NextRun, time.Now()))
	fmt.Printf("Project: %s\n", app.HumanizePath(entry.ProjectPath))
}

func printUpdated(entry scheduler.ScheduleEntry) {
	fmt.Println("Schedule updated.")
	fmt.Printf("ID: %s\n", entry.ID)
	fmt.Printf("Next run: %s (%s)\n", entry.NextRun.Format(time.RFC1123), scheduler.RelativeLabel(entry.NextRun, time.Now()))
}

func printDeleted(entry scheduler.ScheduleEntry) {
	fmt.Println("Schedule deleted.")
	fmt.Printf("ID: %s\n", entry.ID)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "wakeclaude - schedule Claude prompts from local sessions")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  wakeclaude [--projects-root <path>]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --projects-root   Root directory for Claude projects (default: ~/.claude/projects)")
	fmt.Fprintln(os.Stderr, "  --run             Internal: run a scheduled task by id")
	fmt.Fprintln(os.Stderr, "  --help, -h        Show help")
}
