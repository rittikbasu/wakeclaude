package scheduler

import (
	"fmt"
	"os"
	"os/exec"
)

func ScheduleWake(entry ScheduleEntry, when string) error {
	if when == "" {
		return nil
	}
	owner := wakeOwner(entry.ID)
	return runSudo("pmset", "schedule", "wakeorpoweron", when, owner)
}

func CancelWake(entry ScheduleEntry) error {
	if entry.WakeTime == "" {
		return nil
	}
	owner := wakeOwner(entry.ID)
	return runSudo("pmset", "schedule", "cancel", "wakeorpoweron", entry.WakeTime, owner)
}

func wakeOwner(id string) string {
	return fmt.Sprintf("com.wakeclaude.%s", id)
}

func runSudo(args ...string) error {
	if os.Geteuid() == 0 {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	cmd := exec.Command("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
