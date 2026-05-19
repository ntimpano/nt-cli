package model

import "time"

// BehavioralObservation is a persisted candidate extracted from
// `[BEHAVIORAL_OBSERVATION: ...]` markers.
type BehavioralObservation struct {
	ID              int64
	Category        string
	Field           string
	Value           string
	Confidence      int
	OccurrenceCount int
	Status          string
	LastSeen        time.Time
	CreatedAt       time.Time
}
