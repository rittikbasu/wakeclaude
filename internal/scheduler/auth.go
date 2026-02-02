package scheduler

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"wakeclaude/internal/app"
)

func loadOAuthToken(entry ScheduleEntry) (string, error) {
	if os.Geteuid() == 0 && entry.UID > 0 {
		return loadOAuthTokenAsUser(entry)
	}
	token, err := app.LoadOAuthToken()
	if err != nil {
		return "", fmt.Errorf("missing setup token; run %s", app.ClaudeSetupTokenCmd)
	}
	if strings.TrimSpace(token) == "" {
		return "", fmt.Errorf("missing setup token; run %s", app.ClaudeSetupTokenCmd)
	}
	return token, nil
}

func loadOAuthTokenAsUser(entry ScheduleEntry) (string, error) {
	args := []string{
		"asuser", strconv.Itoa(entry.UID),
		"/usr/bin/security", "find-generic-password",
		"-s", app.ClaudeOAuthService,
		"-w",
	}
	if entry.User != "" {
		args = append(args, "-a", entry.User)
	}

	cmd := exec.Command("/bin/launchctl", args...)
	cmd.Env = append(os.Environ(), []string{
		"HOME=" + entry.HomeDir,
		"USER=" + entry.User,
		"LOGNAME=" + entry.User,
		"LANG=C",
	}...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("missing setup token; run %s", app.ClaudeSetupTokenCmd)
	}
	token := strings.TrimSpace(string(output))
	if token == "" {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return "", fmt.Errorf("missing setup token; run %s", app.ClaudeSetupTokenCmd)
		}
		return "", fmt.Errorf("missing setup token; run %s", app.ClaudeSetupTokenCmd)
	}
	return token, nil
}
