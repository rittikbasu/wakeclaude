package scheduler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	scheduleVersion = 1
	MaxRunLogs      = 50
	MaxDaemonLogs   = 50
)

type scheduleFile struct {
	Version   int             `json:"version"`
	Schedules []ScheduleEntry `json:"schedules"`
}

func (s *Store) LoadSchedules() ([]ScheduleEntry, error) {
	if err := s.Ensure(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(s.Schedules)
	if err != nil {
		if os.IsNotExist(err) {
			return []ScheduleEntry{}, nil
		}
		return nil, fmt.Errorf("read schedules: %w", err)
	}

	var file scheduleFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse schedules: %w", err)
	}

	if file.Version == 0 {
		file.Version = scheduleVersion
	}

	return file.Schedules, nil
}

func (s *Store) SaveSchedules(entries []ScheduleEntry) error {
	if err := s.Ensure(); err != nil {
		return err
	}

	file := scheduleFile{
		Version:   scheduleVersion,
		Schedules: entries,
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("encode schedules: %w", err)
	}

	tmp := s.Schedules + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write schedules: %w", err)
	}
	return os.Rename(tmp, s.Schedules)
}

func (s *Store) AddSchedule(entry ScheduleEntry) (ScheduleEntry, error) {
	entries, err := s.LoadSchedules()
	if err != nil {
		return ScheduleEntry{}, err
	}
	entries = append(entries, entry)
	if err := s.SaveSchedules(entries); err != nil {
		return ScheduleEntry{}, err
	}
	return entry, nil
}

func (s *Store) UpdateSchedule(entry ScheduleEntry) error {
	entries, err := s.LoadSchedules()
	if err != nil {
		return err
	}
	found := false
	for i := range entries {
		if entries[i].ID == entry.ID {
			entries[i] = entry
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("schedule not found: %s", entry.ID)
	}
	return s.SaveSchedules(entries)
}

func (s *Store) DeleteSchedule(id string) (ScheduleEntry, error) {
	entries, err := s.LoadSchedules()
	if err != nil {
		return ScheduleEntry{}, err
	}

	var deleted ScheduleEntry
	kept := make([]ScheduleEntry, 0, len(entries))
	found := false
	for _, entry := range entries {
		if entry.ID == id {
			deleted = entry
			found = true
			continue
		}
		kept = append(kept, entry)
	}
	if !found {
		return ScheduleEntry{}, fmt.Errorf("schedule not found: %s", id)
	}

	if err := s.SaveSchedules(kept); err != nil {
		return ScheduleEntry{}, err
	}
	return deleted, nil
}

func (s *Store) LoadLogs(limit int) ([]LogEntry, error) {
	if err := s.Ensure(); err != nil {
		return nil, err
	}

	file, err := os.Open(s.Logs)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogEntry{}, nil
		}
		return nil, fmt.Errorf("read logs: %w", err)
	}
	defer file.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			entries = append(entries, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read logs: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].RanAt.After(entries[j].RanAt)
	})

	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func (s *Store) AppendLog(entry LogEntry) error {
	return s.AppendLogWithOwnership(entry, -1, -1)
}

func (s *Store) AppendLogWithOwnership(entry LogEntry, uid, gid int) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	if entry.ID == "" {
		entry.ID = NewID()
	}

	file, err := os.OpenFile(s.Logs, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("write log: %w", err)
	}
	defer file.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode log: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write log: %w", err)
	}

	if uid >= 0 && gid >= 0 {
		_ = os.Chown(s.Logs, uid, gid)
	}
	return nil
}

func (s *Store) LogFilePath(entry LogEntry) string {
	name := fmt.Sprintf("run-%s-%s.log", entry.ScheduleID, entry.RanAt.Format("20060102-150405"))
	return filepath.Join(s.LogsDir, name)
}

func (s *Store) PruneLogs(runMax, daemonMax int, uid, gid int) error {
	if runMax <= 0 && daemonMax <= 0 {
		return nil
	}
	if err := s.Ensure(); err != nil {
		return err
	}

	entries, err := s.LoadLogs(0)
	if err != nil {
		return err
	}
	if runMax > 0 && len(entries) > runMax {
		entries = entries[:runMax]
	}

	keepPaths := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		path := entry.OutputPath
		if path == "" {
			path = s.LogFilePath(entry)
		}
		if path == "" {
			continue
		}
		keepPaths[filepath.Clean(path)] = struct{}{}
	}

	if err := s.writeLogIndex(entries, uid, gid); err != nil {
		return err
	}

	if err := s.pruneRunLogs(runMax, keepPaths); err != nil {
		return err
	}

	if err := s.pruneDaemonLogs(daemonMax); err != nil {
		return err
	}

	return nil
}

func (s *Store) writeLogIndex(entries []LogEntry, uid, gid int) error {
	if len(entries) == 0 {
		if _, err := os.Stat(s.Logs); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
	}

	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	tmp := s.Logs + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("write log: %w", err)
	}
	enc := json.NewEncoder(file)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			_ = file.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("encode log: %w", err)
		}
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write log: %w", err)
	}
	if err := os.Rename(tmp, s.Logs); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write log: %w", err)
	}
	if uid >= 0 && gid >= 0 {
		_ = os.Chown(s.Logs, uid, gid)
	}
	return nil
}

type logFile struct {
	path    string
	modTime time.Time
}

func (s *Store) pruneRunLogs(max int, keep map[string]struct{}) error {
	if max <= 0 {
		return nil
	}
	files, err := s.listLogFiles("run-", ".log")
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	if len(keep) > 0 {
		for _, file := range files {
			if _, ok := keep[filepath.Clean(file.path)]; ok {
				continue
			}
			_ = os.Remove(file.path)
		}
		return nil
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	if len(files) <= max {
		return nil
	}
	for _, file := range files[max:] {
		_ = os.Remove(file.path)
	}
	return nil
}

func (s *Store) pruneDaemonLogs(max int) error {
	if max <= 0 {
		return nil
	}
	files, err := s.listLogFiles("daemon-", ".log")
	if err != nil {
		return err
	}
	if len(files) <= max {
		return nil
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	for _, file := range files[max:] {
		_ = os.Remove(file.path)
	}
	return nil
}

func (s *Store) listLogFiles(prefix, suffix string) ([]logFile, error) {
	entries, err := os.ReadDir(s.LogsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]logFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, logFile{
			path:    filepath.Join(s.LogsDir, name),
			modTime: info.ModTime(),
		})
	}
	return files, nil
}

func Preview(text string, max int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
