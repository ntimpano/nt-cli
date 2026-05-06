package app

import "time"

// Project is the application-level view of a stored project context.
// The store layer (internal/store) returns/accepts these.
type Project struct {
	ID          int64
	Name        string
	RootPath    string
	Fingerprint string
	CreatedAt   time.Time
}

// ProjectInput is the create-time payload for a new project. CreatedAt is
// stamped by the store at insert time so callers don't need to set it.
type ProjectInput struct {
	Name        string
	RootPath    string
	Fingerprint string
}
