package model

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

// ProbeResult is the read-only proposal returned by Probe. It never mutates
// state — callers invoke Confirm or Switch explicitly to change the active
// project.
type ProbeResult struct {
	Status     string    // "known" | "new" | "ambiguous" | "none"
	Candidate  string    // project name (existing or proposed)
	Candidates []Project // non-nil only when Status=="ambiguous"
	Confidence string    // "high" | "low"
	Reason     string
}
