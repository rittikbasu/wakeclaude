package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var uuidRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ListSessions(projectPath string) ([]Session, error) {
	sessions, err := CollectSessions(projectPath)
	if err != nil {
		return nil, err
	}

	fillSessionPreviews(sessions)

	return sessions, nil
}

func IsUUID(value string) bool {
	return uuidRe.MatchString(value)
}

func CollectSessions(projectPath string) ([]Session, error) {
	projectPath, err := NormalizePath(projectPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(projectPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("project path not found: %s", projectPath)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied reading %s", projectPath)
		}
		return nil, fmt.Errorf("stat project path %s: %w", projectPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project path is not a directory: %s", projectPath)
	}

	entries, err := os.ReadDir(projectPath)
	if err != nil {
		return nil, fmt.Errorf("read project directory %s: %w", projectPath, err)
	}

	sessions := make([]Session, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".jsonl") {
			continue
		}

		id := strings.TrimSuffix(name, ".jsonl")
		if !IsUUID(id) {
			continue
		}

		entryInfo, err := entry.Info()
		if err != nil {
			continue
		}

		sessions = append(sessions, Session{
			ID:      id,
			Path:    filepath.Join(projectPath, name),
			ModTime: entryInfo.ModTime(),
		})
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].ModTime.Equal(sessions[j].ModTime) {
			return sessions[i].ID < sessions[j].ID
		}
		return sessions[i].ModTime.After(sessions[j].ModTime)
	})

	for i := range sessions {
		sessions[i].RelTime = RelativeTime(sessions[i].ModTime)
	}

	return sessions, nil
}

type previewResult struct {
	index   int
	preview string
}

func fillSessionPreviews(sessions []Session) {
	if len(sessions) == 0 {
		return
	}

	workers := runtime.GOMAXPROCS(0)
	if workers < 2 {
		workers = 2
	}
	if workers > 4 {
		workers = 4
	}

	jobs := make(chan int)
	results := make(chan previewResult, len(sessions))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				preview, err := ExtractPreview(sessions[idx].Path)
				if err != nil {
					preview = ""
				}
				results <- previewResult{index: idx, preview: preview}
			}
		}()
	}

	for i := range sessions {
		jobs <- i
	}
	close(jobs)

	wg.Wait()
	close(results)

	for res := range results {
		sessions[res.index].Preview = res.preview
	}
}
