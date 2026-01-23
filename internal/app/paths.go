package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func DefaultProjectsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, ".claude", "projects"), nil
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
