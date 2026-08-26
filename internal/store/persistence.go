package store

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"encoding/json"
)

type diskState struct {
	Runs        map[string]domain.TestRun               `json:"runs"`
	Packages    map[string][]domain.DataPackage         `json:"packages"`
	Anomalies   map[string][]domain.Anomaly             `json:"anomalies"`
	Evidence    map[string][]domain.DispositionEvidence `json:"evidence"`
	Reviews     map[string][]domain.ReviewSnapshot      `json:"reviews"`
	Decisions   map[string]domain.ValidityDecision      `json:"decisions"`
	Audit       []domain.AuditEvent                     `json:"audit"`
	Idempotency map[string]domain.IdempotencyRecord     `json:"idempotency"`
}

func (s *Memory) snapshotLocked() diskState {
	b, _ := json.Marshal(diskState{Runs: s.runs, Packages: s.packages, Anomalies: s.anomalies, Evidence: s.evidence, Reviews: s.reviews, Decisions: s.decisions, Audit: s.audit, Idempotency: s.idem})
	var d diskState
	_ = json.Unmarshal(b, &d)
	return d
}
func (s *Memory) restoreLocked(d diskState) {
	s.runs = d.Runs
	s.packages = d.Packages
	s.anomalies = d.Anomalies
	s.evidence = d.Evidence
	s.reviews = d.Reviews
	s.decisions = d.Decisions
	s.audit = d.Audit
	s.idem = d.Idempotency
}
func (s *Memory) load() error {
	if s.path == "" || s.path == ":memory:" {
		return nil
	}
	if err := sqliteInit(s.path); err != nil {
		return err
	}
	b, err := sqliteLoad(s.path)
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var d diskState
	if err = json.Unmarshal(b, &d); err != nil {
		return err
	}
	s.restoreLocked(d)
	return nil
}
func (s *Memory) saveLocked() error {
	if s.path == "" || s.path == ":memory:" {
		return nil
	}
	b, err := json.Marshal(s.snapshotLocked())
	if err != nil {
		return err
	}
	return sqliteSave(s.path, b)
}
