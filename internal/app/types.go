package app

import "time"

type Project struct {
	Path         string    `json:"path"`
	DisplayName  string    `json:"display_name"`
	CWD          string    `json:"cwd,omitempty"`
	SessionCount int       `json:"session_count"`
	LastModified time.Time `json:"last_modified"`
	LastActive   string    `json:"last_active"`
}

type Session struct {
	ID      string    `json:"id"`
	Path    string    `json:"path"`
	ModTime time.Time `json:"mod_time"`
	RelTime string    `json:"rel_time"`
	Preview string    `json:"preview"`
}

type ModelOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}
