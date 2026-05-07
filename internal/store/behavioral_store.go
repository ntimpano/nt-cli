package store

import (
	"time"

	"nt-cli/internal/app"
)

type BehavioralObservation = app.BehavioralObservation

type BehavioralStore interface {
	RecordObservation(category, field, value string, confidence int, now time.Time) (int64, error)
	ListObservations(includeStatuses []string) ([]BehavioralObservation, error)
	GetObservation(id int64) (*BehavioralObservation, error)
	DismissObservation(id int64) error
	Candidates() ([]BehavioralObservation, error)
}
