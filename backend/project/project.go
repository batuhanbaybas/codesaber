package project

import "time"

const (
	EventAdded   = "project.added"
	EventRemoved = "project.removed"
)

type Project struct {
	ID       string    `json:"id"` // uuid
	Name     string    `json:"name"`
	Root     string    `json:"root"`
	Branch   string    `json:"branch"`
	LastUsed time.Time `json:"lastUsed"`
	EngineOK bool      `json:"engineOk"`
}
