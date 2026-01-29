package scheduler

import "time"

type ScheduleEntry struct {
	ID             string    `json:"id"`
	ProjectPath    string    `json:"projectPath"`
	SessionID      string    `json:"sessionId,omitempty"`
	SessionPath    string    `json:"sessionPath,omitempty"`
	NewSession     bool      `json:"newSession"`
	Model          string    `json:"model"`
	PermissionMode string    `json:"permissionMode,omitempty"`
	Prompt         string    `json:"prompt"`
	Schedule       Schedule  `json:"schedule"`
	Timezone       string    `json:"timezone"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	NextRun        time.Time `json:"nextRun"`
	WakeTime       string    `json:"wakeTime"`
	BinaryPath     string    `json:"binaryPath"`
	User           string    `json:"user"`
	UID            int       `json:"uid"`
	GID            int       `json:"gid"`
	HomeDir        string    `json:"homeDir"`
	PathEnv        string    `json:"pathEnv"`
}

type Schedule struct {
	Type    string `json:"type"`
	Date    string `json:"date,omitempty"`
	Time    string `json:"time,omitempty"`
	Weekday string `json:"weekday,omitempty"`
}

type LogEntry struct {
	ID            string    `json:"id"`
	ScheduleID    string    `json:"scheduleId"`
	RanAt         time.Time `json:"ranAt"`
	Status        string    `json:"status"`
	ExitCode      int       `json:"exitCode"`
	Error         string    `json:"error,omitempty"`
	PromptPreview string    `json:"promptPreview"`
	Model         string    `json:"model"`
	SessionID     string    `json:"sessionId,omitempty"`
	NewSession    bool      `json:"newSession"`
	OutputPath    string    `json:"outputPath,omitempty"`
	ProjectPath   string    `json:"projectPath,omitempty"`
}
