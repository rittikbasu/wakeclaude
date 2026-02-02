package scheduler

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"wakeclaude/internal/app"
)

func RunSchedule(store *Store, id string) error {
	schedules, err := store.LoadSchedules()
	if err != nil {
		return err
	}

	var entry *ScheduleEntry
	for i := range schedules {
		if schedules[i].ID == id {
			entry = &schedules[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("schedule not found: %s", id)
	}
	defer func() {
		_ = store.PruneLogs(MaxRunLogs, MaxDaemonLogs, entry.UID, entry.GID)
	}()

	logEntry := LogEntry{
		ID:            NewID(),
		ScheduleID:    entry.ID,
		RanAt:         time.Now(),
		Status:        "error",
		PromptPreview: Preview(entry.Prompt, 120),
		Model:         entry.Model,
		SessionID:     entry.SessionID,
		NewSession:    entry.NewSession,
		ProjectPath:   entry.ProjectPath,
	}

	if err := store.Ensure(); err != nil {
		logEntry.Error = err.Error()
		_ = store.AppendLogWithOwnership(logEntry, entry.UID, entry.GID)
		return err
	}

	outputPath := store.LogFilePath(logEntry)
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		logEntry.Error = err.Error()
		_ = store.AppendLogWithOwnership(logEntry, entry.UID, entry.GID)
		return err
	}

	outputFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		logEntry.Error = err.Error()
		_ = store.AppendLogWithOwnership(logEntry, entry.UID, entry.GID)
		return err
	}
	defer outputFile.Close()
	_ = os.Chown(outputPath, entry.UID, entry.GID)

	cmd, err := buildClaudeCommand(*entry)
	if err != nil {
		logEntry.Error = err.Error()
		_ = store.AppendLogWithOwnership(logEntry, entry.UID, entry.GID)
		return err
	}

	cmd.Stdout = outputFile
	cmd.Stderr = outputFile

	exitCode := 0
	if err := runWithCaffeinate(cmd, outputFile); err != nil {
		exitCode = exitStatus(err)
		logEntry.Error = err.Error()
	} else {
		logEntry.Status = "success"
	}

	if logEntry.SessionID == "" && entry.NewSession && logEntry.Status == "success" {
		if sessionID := findNewSessionID(*entry, logEntry.RanAt); sessionID != "" {
			logEntry.SessionID = sessionID
		}
	}

	logEntry.ExitCode = exitCode
	logEntry.OutputPath = outputPath
	_ = store.AppendLogWithOwnership(logEntry, entry.UID, entry.GID)
	NotifyRun(*entry, logEntry)

	if entry.Schedule.Type == "once" {
		RemoveLaunchdIfRoot(*entry)
		_, _ = store.DeleteSchedule(entry.ID)
		_ = os.Chown(store.Schedules, entry.UID, entry.GID)
		return nil
	}

	now := time.Now()
	nextRun, err := NextRun(*entry, now)
	if err == nil {
		entry.NextRun = nextRun
		entry.UpdatedAt = now
		entry.WakeTime = FormatPMSet(nextRun)
		_ = store.UpdateSchedule(*entry)
		_ = os.Chown(store.Schedules, entry.UID, entry.GID)
		if os.Geteuid() == 0 {
			_ = ScheduleWake(*entry, entry.WakeTime)
		}
	}

	return nil
}

func buildClaudeCommand(entry ScheduleEntry) (*exec.Cmd, error) {
	path, err := findInPath(entry.PathEnv, "claude")
	if err != nil {
		return nil, fmt.Errorf("claude not found in PATH; install: %s", app.ClaudeInstallCmd)
	}
	token, err := loadOAuthToken(entry)
	if err != nil {
		return nil, err
	}

	workDir := resolveWorkDir(entry)
	if workDir == "" {
		workDir = entry.HomeDir
	}

	args := []string{"-p"}
	if entry.Model != "" && entry.Model != "auto" {
		args = append(args, "--model", entry.Model)
	}
	if entry.PermissionMode != "" && entry.PermissionMode != "default" {
		args = append(args, "--permission-mode", entry.PermissionMode)
	}
	if !entry.NewSession && entry.SessionID != "" {
		args = append(args, "--resume", entry.SessionID)
	}
	args = append(args, entry.Prompt)

	if os.Geteuid() == 0 && entry.UID > 0 {
		cmd := exec.Command("/bin/launchctl", append([]string{
			"asuser", strconv.Itoa(entry.UID),
			"/usr/bin/sudo", "-u", entry.User, "-H", "--",
			"/usr/bin/env",
			"CLAUDE_CODE_OAUTH_TOKEN=" + token,
			"ANTHROPIC_API_KEY=",
			"ANTHROPIC_AUTH_TOKEN=",
			path,
		}, args...)...)
		cmd.Dir = workDir
		cmd.Env = append(os.Environ(), []string{
			"HOME=" + entry.HomeDir,
			"USER=" + entry.User,
			"LOGNAME=" + entry.User,
			"PATH=" + entry.PathEnv,
		}...)
		return cmd, nil
	}

	cmd := exec.Command(path, args...)
	cmd.Dir = workDir

	cmd.Env = append(os.Environ(), []string{
		"HOME=" + entry.HomeDir,
		"USER=" + entry.User,
		"LOGNAME=" + entry.User,
		"PATH=" + entry.PathEnv,
		"CLAUDE_CODE_OAUTH_TOKEN=" + token,
		"ANTHROPIC_API_KEY=",
		"ANTHROPIC_AUTH_TOKEN=",
	}...)

	return cmd, nil
}

func resolveWorkDir(entry ScheduleEntry) string {
	path := strings.TrimSpace(entry.ProjectPath)
	if path != "" && isValidWorkDir(path) {
		return path
	}
	if entry.SessionPath != "" {
		if cwd, err := app.ExtractCWD(entry.SessionPath); err == nil && isValidWorkDir(cwd) {
			return cwd
		}
	}
	return ""
}

func isValidWorkDir(path string) bool {
	if path == "" {
		return false
	}
	if strings.Contains(path, string(filepath.Separator)+".claude"+string(filepath.Separator)+"projects"+string(filepath.Separator)) {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

func findNewSessionID(entry ScheduleEntry, since time.Time) string {
	projectDir := findClaudeProjectDir(entry)
	if projectDir == "" {
		return ""
	}
	sessions, err := app.CollectSessions(projectDir)
	if err != nil {
		return ""
	}
	cutoff := since.Add(-30 * time.Second)
	for _, session := range sessions {
		if session.ModTime.Before(cutoff) {
			break
		}
		if session.ModTime.After(cutoff) {
			if !matchesPrompt(entry.Prompt, session.Path) {
				continue
			}
			if os.Geteuid() == 0 && entry.UID > 0 {
				_ = os.Chown(session.Path, entry.UID, entry.GID)
			}
			return session.ID
		}
	}
	return ""
}

func matchesPrompt(prompt, sessionPath string) bool {
	if strings.TrimSpace(prompt) == "" {
		return false
	}
	text, err := app.ExtractFirstUserText(sessionPath)
	if err != nil {
		return false
	}
	return promptMatchesText(prompt, text)
}

func promptMatchesText(prompt, text string) bool {
	p := normalizePromptText(prompt)
	t := normalizePromptText(text)
	if p == "" || t == "" {
		return false
	}
	if strings.HasPrefix(p, t) || strings.HasPrefix(t, p) {
		return true
	}
	return false
}

func normalizePromptText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) > 200 {
		return string(runes[:200])
	}
	return text
}

func findClaudeProjectDir(entry ScheduleEntry) string {
	if entry.ProjectPath == "" {
		return ""
	}

	root := ""
	if entry.HomeDir != "" {
		root = filepath.Join(entry.HomeDir, ".claude", "projects")
	} else if resolved, err := app.DefaultProjectsRoot(); err == nil {
		root = resolved
	}
	if root == "" {
		return ""
	}

	wanted, err := app.NormalizePath(entry.ProjectPath)
	if err != nil {
		return ""
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		sessions, err := app.CollectSessions(dir)
		if err != nil || len(sessions) == 0 {
			continue
		}
		for i, session := range sessions {
			if i >= 5 {
				break
			}
			cwd, err := app.ExtractCWD(session.Path)
			if err != nil || cwd == "" {
				continue
			}
			if samePath(cwd, wanted) {
				return dir
			}
		}
	}
	return ""
}

func samePath(path, wanted string) bool {
	normalized, err := app.NormalizePath(path)
	if err != nil {
		return false
	}
	return normalized == wanted
}

func findInPath(pathEnv, name string) (string, error) {
	if pathEnv == "" {
		return exec.LookPath(name)
	}
	for _, dir := range strings.Split(pathEnv, string(os.PathListSeparator)) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", exec.ErrNotFound
}

func runWithCaffeinate(cmd *exec.Cmd, outputFile *os.File) error {
	if err := cmd.Start(); err != nil {
		return err
	}

	var caf *exec.Cmd
	if path, err := exec.LookPath("caffeinate"); err == nil {
		pid := cmd.Process.Pid
		caf = exec.Command(path, "-d", "-i", "-s", "-w", strconv.Itoa(pid))
		caf.Stdout = outputFile
		caf.Stderr = outputFile
		_ = caf.Start()
	}

	err := cmd.Wait()
	if caf != nil {
		_ = caf.Wait()
	}
	return err
}

func exitStatus(err error) int {
	var exitErr *exec.ExitError
	if err == nil {
		return 0
	}
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
	}
	return 1
}
