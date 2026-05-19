package store

import (
	"time"

	"flint/internal/model"
)

type BehavioralStore interface {
	RecordObservation(category, field, value string, confidence int, now time.Time) (int64, error)
	ListObservations(includeStatuses []string) ([]model.BehavioralObservation, error)
	GetObservation(id int64) (*model.BehavioralObservation, error)
	DismissObservation(id int64) error
	Candidates() ([]model.BehavioralObservation, error)
}
