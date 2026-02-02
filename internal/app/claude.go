package app

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
)

const ClaudeInstallCmd = "curl -fsSL https://claude.ai/install.sh | bash"
const ClaudeSetupTokenCmd = "claude setup-token"
const ClaudeOAuthService = "wakeclaude-claude-oauth"

func ClaudeAvailable() bool {
	_, err := exec.LookPath("claude")
	return err == nil
}

func LoadOAuthToken() (string, error) {
	account := currentUsername()
	if account != "" {
		cmd := exec.Command("/usr/bin/security", "find-generic-password", "-s", ClaudeOAuthService, "-a", account, "-w")
		cmd.Env = append(os.Environ(), "LANG=C")
		if output, err := cmd.Output(); err == nil {
			token := strings.TrimSpace(string(output))
			if token != "" {
				return token, nil
			}
			return "", os.ErrNotExist
		} else if !isTokenNotFound(err) {
			// fall through to try without account, but remember the error
		}
	}

	cmd := exec.Command("/usr/bin/security", "find-generic-password", "-s", ClaudeOAuthService, "-w")
	cmd.Env = append(os.Environ(), "LANG=C")
	output, err := cmd.Output()
	if err != nil {
		if isTokenNotFound(err) {
			return "", os.ErrNotExist
		}
		return "", err
	}
	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", os.ErrNotExist
	}
	return token, nil
}

func SaveOAuthToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("token is empty")
	}
	args := []string{"add-generic-password", "-s", ClaudeOAuthService, "-w", token, "-U"}
	if account := currentUsername(); account != "" {
		args = append(args, "-a", account)
	}
	cmd := exec.Command("/usr/bin/security", args...)
	cmd.Env = append(os.Environ(), "LANG=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("keychain: %s", msg)
		}
		return err
	}
	return nil
}

func currentUsername() string {
	if usr, err := user.Current(); err == nil {
		if usr.Username != "" {
			return usr.Username
		}
	}
	if value := os.Getenv("USER"); value != "" {
		return value
	}
	return ""
}

func isTokenNotFound(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return false
	}
	return status.ExitStatus() == 44
}

func VerifyOAuthToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("token is empty")
	}
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude not found in PATH")
	}

	verifyDir, err := WakeClaudeVerifyDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(verifyDir, 0o755); err != nil {
		return fmt.Errorf("create verify directory: %w", err)
	}

	cmd := exec.Command("claude", "-p", "ping", "--permission-mode", "plan", "--model", "haiku")
	cmd.Dir = verifyDir
	cmd.Env = append(os.Environ(),
		"CLAUDE_CODE_OAUTH_TOKEN="+token,
		"ANTHROPIC_API_KEY=",
		"ANTHROPIC_AUTH_TOKEN=",
	)
	output, cmdErr := cmd.CombinedOutput()
	cleanupVerifyProject(verifyDir)
	if cmdErr != nil {
		msg := strings.TrimSpace(string(output))
		if msg != "" {
			return fmt.Errorf(friendlyTokenError(msg))
		}
		return fmt.Errorf("token verification failed")
	}
	return nil
}

func friendlyTokenError(msg string) string {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "failed to authenticate") || strings.Contains(lower, "authentication") || strings.Contains(lower, "unauthorized") {
		return "invalid token. run `claude setup-token` again"
	}
	if strings.Contains(lower, "api error: 401") || strings.Contains(lower, "401") {
		return "invalid token. run `claude setup-token` again"
	}
	return msg
}

func cleanupVerifyProject(verifyDir string) {
	name, err := ClaudeProjectDirName(verifyDir)
	if err != nil || name == "" {
		return
	}
	if !strings.Contains(name, wakeClaudeAppName) {
		return
	}
	root, err := DefaultProjectsRoot()
	if err != nil || root == "" {
		return
	}
	projectPath := filepath.Join(root, name)
	_ = os.RemoveAll(projectPath)
}
