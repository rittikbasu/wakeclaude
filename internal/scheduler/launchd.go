package scheduler

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const launchdDomain = "system"

func LaunchdPath(id string) string {
	return filepath.Join("/Library/LaunchDaemons", fmt.Sprintf("com.wakeclaude.%s.plist", id))
}

func EnsureLaunchd(entry ScheduleEntry) error {
	interval, err := calendarInterval(entry)
	if err != nil {
		return err
	}

	plist := buildPlist(entry, interval)
	tmp, err := writeTempPlist(entry.ID, plist)
	if err != nil {
		return err
	}
	defer os.Remove(tmp)

	dest := LaunchdPath(entry.ID)
	if err := runSudo("install", "-m", "644", tmp, dest); err != nil {
		return fmt.Errorf("install launchd plist: %w", err)
	}

	_ = runSudoQuiet("launchctl", "bootout", launchdDomain, dest)
	if err := runSudo("launchctl", "bootstrap", launchdDomain, dest); err != nil {
		return fmt.Errorf("load launchd job: %w", err)
	}
	return nil
}

func RemoveLaunchd(entry ScheduleEntry) error {
	dest := LaunchdPath(entry.ID)
	_ = runSudoQuiet("launchctl", "bootout", launchdDomain, dest)
	_ = runSudo("rm", "-f", dest)
	return nil
}

func RemoveLaunchdIfRoot(entry ScheduleEntry) {
	if os.Geteuid() != 0 {
		return
	}
	dest := LaunchdPath(entry.ID)
	_ = runSudoQuiet("launchctl", "bootout", launchdDomain, dest)
	_ = runSudo("rm", "-f", dest)
}

func calendarInterval(entry ScheduleEntry) (map[string]int, error) {
	switch entry.Schedule.Type {
	case "once":
		next, err := NextRun(entry, time.Now())
		if err != nil {
			return nil, err
		}
		return map[string]int{
			"Year":   next.Year(),
			"Month":  int(next.Month()),
			"Day":    next.Day(),
			"Hour":   next.Hour(),
			"Minute": next.Minute(),
		}, nil
	case "daily":
		hour, minute := parseClock(entry.Schedule.Time)
		return map[string]int{
			"Hour":   hour,
			"Minute": minute,
		}, nil
	case "weekly":
		hour, minute := parseClock(entry.Schedule.Time)
		weekday, ok := WeekdayNumber(entry.Schedule.Weekday)
		if !ok {
			return nil, fmt.Errorf("invalid weekday: %s", entry.Schedule.Weekday)
		}
		return map[string]int{
			"Weekday": weekday,
			"Hour":    hour,
			"Minute":  minute,
		}, nil
	default:
		return nil, fmt.Errorf("unknown schedule type: %s", entry.Schedule.Type)
	}
}

func writeTempPlist(id string, data []byte) (string, error) {
	name := fmt.Sprintf("wakeclaude-%s.plist", id)
	path := filepath.Join(os.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func buildPlist(entry ScheduleEntry, interval map[string]int) []byte {
	arguments := []string{entry.BinaryPath, "--run", entry.ID}
	env := map[string]string{
		"PATH":    entry.PathEnv,
		"HOME":    entry.HomeDir,
		"USER":    entry.User,
		"LOGNAME": entry.User,
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	b.WriteString("<dict>\n")
	writeKey(&b, "Label")
	writeString(&b, fmt.Sprintf("com.wakeclaude.%s", entry.ID))
	writeKey(&b, "ProgramArguments")
	writeArray(&b, arguments)
	writeKey(&b, "StartCalendarInterval")
	writeDict(&b, interval)
	writeKey(&b, "StandardOutPath")
	writeString(&b, filepath.Join(entry.HomeDir, "Library", "Application Support", appName, "logs", fmt.Sprintf("daemon-%s.out.log", entry.ID)))
	writeKey(&b, "StandardErrorPath")
	writeString(&b, filepath.Join(entry.HomeDir, "Library", "Application Support", appName, "logs", fmt.Sprintf("daemon-%s.err.log", entry.ID)))
	writeKey(&b, "EnvironmentVariables")
	writeStringDict(&b, env)
	writeKey(&b, "RunAtLoad")
	writeBool(&b, false)
	b.WriteString("</dict>\n</plist>\n")
	return []byte(b.String())
}

func writeKey(b *strings.Builder, key string) {
	fmt.Fprintf(b, "<key>%s</key>\n", xmlEscape(key))
}

func writeString(b *strings.Builder, value string) {
	fmt.Fprintf(b, "<string>%s</string>\n", xmlEscape(value))
}

func writeBool(b *strings.Builder, value bool) {
	if value {
		b.WriteString("<true/>\n")
	} else {
		b.WriteString("<false/>\n")
	}
}

func writeArray(b *strings.Builder, values []string) {
	b.WriteString("<array>\n")
	for _, value := range values {
		writeString(b, value)
	}
	b.WriteString("</array>\n")
}

func writeDict(b *strings.Builder, values map[string]int) {
	b.WriteString("<dict>\n")
	for key, value := range values {
		writeKey(b, key)
		fmt.Fprintf(b, "<integer>%d</integer>\n", value)
	}
	b.WriteString("</dict>\n")
}

func writeStringDict(b *strings.Builder, values map[string]string) {
	b.WriteString("<dict>\n")
	for key, value := range values {
		writeKey(b, key)
		writeString(b, value)
	}
	b.WriteString("</dict>\n")
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
		`'`, "&apos;",
	)
	return replacer.Replace(value)
}

func runSudoQuiet(args ...string) error {
	if os.Geteuid() == 0 {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdout = io.Discard
		cmd.Stderr = io.Discard
		return cmd.Run()
	}
	cmd := exec.Command("sudo", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}
