package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const wakeClaudeAppName = "WakeClaude"

func DefaultProjectsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, ".claude", "projects"), nil
}

func WakeClaudeSupportDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, "Library", "Application Support", wakeClaudeAppName), nil
}

func WakeClaudeVerifyDir() (string, error) {
	base, err := WakeClaudeSupportDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "verify"), nil
}

func ClaudeProjectDirName(path string) (string, error) {
	abs, err := NormalizePath(path)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(abs, string(os.PathSeparator), "-"), nil
}

func WakeClaudeVerifyProjectDirName() (string, error) {
	verifyDir, err := WakeClaudeVerifyDir()
	if err != nil {
		return "", err
	}
	return ClaudeProjectDirName(verifyDir)
}

func IsWakeClaudeInternalPath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	base, err := WakeClaudeSupportDir()
	if err != nil {
		return false
	}
	base, err = NormalizePath(base)
	if err != nil {
		return false
	}
	candidate, err := NormalizePath(path)
	if err != nil {
		return false
	}
	if candidate == base {
		return true
	}
	return strings.HasPrefix(candidate, base+string(os.PathSeparator))
}

func ExpandHome(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}

		if path == "~" {
			return home, nil
		}

		return filepath.Join(home, path[2:]), nil
	}

	return path, nil
}

func NormalizePath(path string) (string, error) {
	expanded, err := ExpandHome(path)
	if err != nil {
		return "", err
	}

	if expanded == "" {
		return "", fmt.Errorf("path is required")
	}

	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("resolve absolute path: %w", err)
	}

	return filepath.Clean(abs), nil
}

func HumanizePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}

	clean := filepath.Clean(path)
	home = filepath.Clean(home)
	if clean == home {
		return "~"
	}

	if strings.HasPrefix(clean, home+string(os.PathSeparator)) {
		rel, err := filepath.Rel(home, clean)
		if err != nil {
			return clean
		}
		return filepath.Join("~", rel)
	}

	return clean
}
