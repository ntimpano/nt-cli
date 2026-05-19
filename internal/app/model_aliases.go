package app

import "flint/internal/model"

type MemoryItem = model.MemoryItem
type SaveRequest = model.SaveRequest
type RecallOptions = model.RecallOptions
type ContextOptions = model.ContextOptions
type ListOptions = model.ListOptions
type ImportRecord = model.ImportRecord
type ImportResult = model.ImportResult
type DoctorReport = model.DoctorReport
type SessionEvent = model.SessionEvent
type BehavioralObservation = model.BehavioralObservation
type RelationDirection = model.RelationDirection
type MemoryRelation = model.MemoryRelation
type RuntimeType = model.RuntimeType
type ModelTiers = model.ModelTiers
type RuntimeConfig = model.RuntimeConfig
type ProfileConfig = model.ProfileConfig
type ProbeResult = model.ProbeResult
type Project = model.Project
type ProjectInput = model.ProjectInput

const (
	RelationDirectionOutbound = model.RelationDirectionOutbound
	RelationDirectionInbound  = model.RelationDirectionInbound
	RuntimeOpenCode           = model.RuntimeOpenCode
	RuntimeUnknown            = model.RuntimeUnknown
)
