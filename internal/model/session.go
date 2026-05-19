package model

import "time"

// SessionEvent is a single row in the session-lifecycle log. Kind is one
// of "start", "summary", "end". Summary is non-empty only for "summary"
// rows. CreatedAt is UTC.
type SessionEvent struct {
	SessionID string
	Kind      string
	Summary   string
	CreatedAt time.Time
}
