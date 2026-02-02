package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func DiscoverProjects(root string) ([]Project, error) {
	var err error
	if root == "" {
		root, err = DefaultProjectsRoot()
		if err != nil {
			return nil, err
		}
	}

	root, err = NormalizePath(root)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Claude projects root not found at %s; run Claude once or pass --projects-root", root)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied reading %s; check permissions or pass --projects-root", root)
		}
		return nil, fmt.Errorf("read projects root %s: %w", root, err)
	}

	projects := make([]Project, 0, len(entries))
	verifyProjectName, _ := WakeClaudeVerifyProjectDirName()
	var warnings int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		if verifyProjectName != "" && entry.Name() == verifyProjectName {
			continue
		}

		path := filepath.Join(root, entry.Name())
		sessions, err := CollectSessions(path)
		if err != nil {
			warnings++
			continue
		}

		if len(sessions) == 0 {
			continue
		}

		displayName, cwd := resolveProjectDisplay(path, sessions)
		if IsWakeClaudeInternalPath(cwd) {
			continue
		}
		project := Project{
			Path:         path,
			DisplayName:  displayName,
			CWD:          cwd,
			SessionCount: len(sessions),
		}
		if len(sessions) > 0 {
			project.LastModified = sessions[0].ModTime
			project.LastActive = RelativeTime(project.LastModified)
		}

		projects = append(projects, project)
	}

	sort.Slice(projects, func(i, j int) bool {
		if projects[i].LastModified.Equal(projects[j].LastModified) {
			return projects[i].DisplayName < projects[j].DisplayName
		}
		return projects[i].LastModified.After(projects[j].LastModified)
	})

	if len(projects) == 0 && warnings > 0 {
		return nil, fmt.Errorf("no readable project directories found under %s", root)
	}

	return projects, nil
}

func resolveProjectDisplay(projectPath string, sessions []Session) (string, string) {
	for _, session := range sessions {
		cwd, err := ExtractCWD(session.Path)
		if err == nil && cwd != "" {
			return HumanizePath(cwd), cwd
		}
	}

	return filepath.Base(projectPath), ""
}
