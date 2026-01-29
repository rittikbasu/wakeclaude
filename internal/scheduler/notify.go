package scheduler

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func NotifyRun(entry ScheduleEntry, logEntry LogEntry) {
	script := buildNotificationScript(logEntry)
	if script == "" {
		return
	}

	if os.Geteuid() == 0 && entry.UID > 0 {
		cmd := exec.Command("/bin/launchctl", "asuser", strconv.Itoa(entry.UID), "/usr/bin/osascript", "-e", script)
		cmd.Env = append(os.Environ(), []string{
			"HOME=" + entry.HomeDir,
			"USER=" + entry.User,
			"LOGNAME=" + entry.User,
		}...)
		_ = cmd.Run()
		return
	}

	cmd := exec.Command("/usr/bin/osascript", "-e", script)
	_ = cmd.Run()
}

func buildNotificationScript(logEntry LogEntry) string {
	title := "WakeClaude"
	subtitle := "Run complete"
	status := "OK"
	message := logEntry.PromptPreview
	if strings.TrimSpace(message) == "" {
		message = "Run finished."
	}

	if logEntry.Status != "success" {
		subtitle = "Run failed"
		status = "ERROR"
		if logEntry.Error != "" {
			message = logEntry.Error
		}
	}

	message = truncateNotification(message, 140)
	full := fmt.Sprintf("%s · %s", status, message)

	return fmt.Sprintf(
		`display notification "%s" with title "%s" subtitle "%s"`,
		escapeAppleScript(full),
		escapeAppleScript(title),
		escapeAppleScript(subtitle),
	)
}

func truncateNotification(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= max {
		return string(runes)
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func escapeAppleScript(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "\"", "\\\"")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", " ")
	return text
}
