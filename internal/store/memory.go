package store

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

type Memory struct {
	mu        sync.RWMutex
	path      string
	runs      map[string]domain.TestRun
	packages  map[string][]domain.DataPackage
	anomalies map[string][]domain.Anomaly
	evidence  map[string][]domain.DispositionEvidence
	reviews   map[string][]domain.ReviewSnapshot
	decisions map[string]domain.ValidityDecision
	audit     []domain.AuditEvent
	idem      map[string]domain.IdempotencyRecord
}

func Open(path string) (*Memory, error) {
	s := &Memory{path: path, runs: map[string]domain.TestRun{}, packages: map[string][]domain.DataPackage{}, anomalies: map[string][]domain.Anomaly{}, evidence: map[string][]domain.DispositionEvidence{}, reviews: map[string][]domain.ReviewSnapshot{}, decisions: map[string]domain.ValidityDecision{}, idem: map[string]domain.IdempotencyRecord{}}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *Memory) Close() error                  { return nil }
func (s *Memory) Migrate(context.Context) error { return nil }
func (s *Memory) VerifyAudit(context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var prev string
	revisions := map[string]int64{}
	for i, e := range s.audit {
		expected := domain.Digest(fmt.Sprintf("%d|%s|%s|%s|%s|%s|%d|%d|%s|%s", e.Sequence, e.TestRunID, e.RequestID, e.Actor, e.Action, e.FromStatus, e.BeforeRevision, e.AfterRevision, e.PayloadDigest, e.PreviousHash))
		if e.Sequence != int64(i+1) || e.PreviousHash != prev || e.EventHash != expected || e.AfterRevision < e.BeforeRevision || e.BeforeRevision != revisions[e.TestRunID] {
			return domain.Integrity("审计哈希链不连续")
		}
		revisions[e.TestRunID] = e.AfterRevision
		prev = e.EventHash
	}
	return nil
}
func (s *Memory) WithinTx(ctx context.Context, fn func(domain.Transaction) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	backup := s.snapshotLocked()
	if err := fn(&tx{s: s}); err != nil {
		s.restoreLocked(backup)
		return err
	}
	if err := s.saveLocked(); err != nil {
		s.restoreLocked(backup)
		return err
	}
	return nil
}
func (s *Memory) GetRun(_ context.Context, id string) (domain.TestRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[id]
	if !ok {
		return r, domain.NotFound("试车任务不存在")
	}
	return cloneRun(r), nil
}
func (s *Memory) ListRuns(_ context.Context) ([]domain.TestRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o := make([]domain.TestRun, 0, len(s.runs))
	for _, r := range s.runs {
		o = append(o, cloneRun(r))
	}
	sort.Slice(o, func(i, j int) bool { return o[i].CreatedAt.After(o[j].CreatedAt) })
	return o, nil
}
func (s *Memory) GetPackages(_ context.Context, id string) ([]domain.DataPackage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clonePackages(s.packages[id]), nil
}
func (s *Memory) GetAnomalies(_ context.Context, id string) ([]domain.Anomaly, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAnomalies(s.anomalies[id]), nil
}
func (s *Memory) GetEvidence(_ context.Context, id string) ([]domain.DispositionEvidence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.DispositionEvidence{}
	for _, a := range s.anomalies[id] {
		out = append(out, cloneEvidenceSlice(s.evidence[a.ID])...)
	}
	return out, nil
}
func (s *Memory) GetReviews(_ context.Context, id string) ([]domain.ReviewSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneReviews(s.reviews[id]), nil
}
func (s *Memory) GetDecision(_ context.Context, id string) (domain.ValidityDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.decisions[id]
	if !ok {
		return d, domain.NotFound("裁定不存在")
	}
	return cloneDecision(d), nil
}
func (s *Memory) GetAudit(_ context.Context, id string) ([]domain.AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o := []domain.AuditEvent{}
	for _, e := range s.audit {
		if e.TestRunID == id {
			o = append(o, e)
		}
	}
	return append([]domain.AuditEvent{}, o...), nil
}

type tx struct{ s *Memory }

func (t *tx) GetRun(_ context.Context, id string) (domain.TestRun, error) {
	r, ok := t.s.runs[id]
	if !ok {
		return r, domain.NotFound("试车任务不存在")
	}
	return cloneRun(r), nil
}
func (t *tx) GetPackages(_ context.Context, id string) ([]domain.DataPackage, error) {
	return clonePackages(t.s.packages[id]), nil
}
func (t *tx) InsertRun(_ context.Context, r domain.TestRun) error {
	if _, ok := t.s.runs[r.ID]; ok {
		return domain.Conflict("试车 ID 已存在")
	}
	t.s.runs[r.ID] = cloneRun(r)
	return nil
}
func (t *tx) UpdateRun(_ context.Context, r domain.TestRun, expected int64) error {
	cur, ok := t.s.runs[r.ID]
	if !ok {
		return domain.NotFound("试车任务不存在")
	}
	if cur.Revision != expected {
		return domain.RevisionError(expected, cur.Revision)
	}
	t.s.runs[r.ID] = cloneRun(r)
	return nil
}
func (t *tx) InsertPackage(_ context.Context, p domain.DataPackage) error {
	t.s.packages[p.TestRunID] = append(t.s.packages[p.TestRunID], clonePackages([]domain.DataPackage{p})[0])
	return nil
}

func clonePackages(in []domain.DataPackage) []domain.DataPackage {
	out := append([]domain.DataPackage{}, in...)
	for i := range out {
		out[i].Files = append([]domain.FileEntry{}, out[i].Files...)
		out[i].ChannelSummaries = append([]domain.ChannelSummary{}, out[i].ChannelSummaries...)
		out[i].DuplicateSegments = append([]string{}, out[i].DuplicateSegments...)
		out[i].GateResults = append([]domain.GateResult{}, out[i].GateResults...)
		for j := range out[i].GateResults {
			out[i].GateResults[j].AffectedChannels = append([]string{}, out[i].GateResults[j].AffectedChannels...)
		}
	}
	return out
}
func cloneRun(r domain.TestRun) domain.TestRun {
	r.ExpectedChannels = append([]string{}, r.ExpectedChannels...)
	if r.PreviousDraft != nil {
		d := *r.PreviousDraft
		d.ExpectedChannels = append([]string{}, d.ExpectedChannels...)
		r.PreviousDraft = &d
	}
	return r
}
func cloneAnomaly(a domain.Anomaly) domain.Anomaly {
	a.AffectedChannels = append([]string{}, a.AffectedChannels...)
	return a
}
func cloneAnomalies(in []domain.Anomaly) []domain.Anomaly {
	out := append([]domain.Anomaly{}, in...)
	for i := range out {
		out[i] = cloneAnomaly(out[i])
	}
	return out
}
func cloneEvidence(e domain.DispositionEvidence) domain.DispositionEvidence {
	e.EvidenceRefs = append([]string{}, e.EvidenceRefs...)
	return e
}
func cloneEvidenceSlice(in []domain.DispositionEvidence) []domain.DispositionEvidence {
	out := append([]domain.DispositionEvidence{}, in...)
	for i := range out {
		out[i] = cloneEvidence(out[i])
	}
	return out
}
func cloneReview(r domain.ReviewSnapshot) domain.ReviewSnapshot {
	r.Checklist = append([]domain.ReviewChecklistItem{}, r.Checklist...)
	r.Targets = append([]domain.ReviewReturnTarget{}, r.Targets...)
	r.Basis = append([]domain.ReviewBasis{}, r.Basis...)
	return r
}
func cloneReviews(in []domain.ReviewSnapshot) []domain.ReviewSnapshot {
	out := append([]domain.ReviewSnapshot{}, in...)
	for i := range out {
		out[i] = cloneReview(out[i])
	}
	return out
}
func cloneDecision(d domain.ValidityDecision) domain.ValidityDecision {
	d.ApplicableObjectives = append([]string{}, d.ApplicableObjectives...)
	d.Limitations = append([]string{}, d.Limitations...)
	return d
}
func cloneIdempotency(r domain.IdempotencyRecord) domain.IdempotencyRecord {
	r.Response = append([]byte{}, r.Response...)
	return r
}
func (t *tx) InsertAnomalies(_ context.Context, a []domain.Anomaly) error {
	cloned := cloneAnomalies(a)
	for _, x := range cloned {
		t.s.anomalies[x.TestRunID] = append(t.s.anomalies[x.TestRunID], x)
	}
	return nil
}
func (t *tx) GetAnomalies(_ context.Context, id string) ([]domain.Anomaly, error) {
	return cloneAnomalies(t.s.anomalies[id]), nil
}
func (t *tx) UpdateAnomaly(_ context.Context, a domain.Anomaly) error {
	xs := t.s.anomalies[a.TestRunID]
	for i := range xs {
		if xs[i].ID == a.ID {
			xs[i] = cloneAnomaly(a)
			t.s.anomalies[a.TestRunID] = xs
			return nil
		}
	}
	return domain.NotFound("异常不存在")
}
func (t *tx) InsertEvidence(_ context.Context, e domain.DispositionEvidence) error {
	t.s.evidence[e.AnomalyID] = append(t.s.evidence[e.AnomalyID], cloneEvidence(e))
	return nil
}
func (t *tx) UpdateEvidence(_ context.Context, e domain.DispositionEvidence) error {
	xs := t.s.evidence[e.AnomalyID]
	for i := range xs {
		if xs[i].ID == e.ID {
			xs[i] = cloneEvidence(e)
			t.s.evidence[e.AnomalyID] = xs
			return nil
		}
	}
	return domain.NotFound("处置证据不存在")
}
func (t *tx) GetEvidence(_ context.Context, id string) ([]domain.DispositionEvidence, error) {
	return cloneEvidenceSlice(t.s.evidence[id]), nil
}
func (t *tx) GetReviews(_ context.Context, id string) ([]domain.ReviewSnapshot, error) {
	return cloneReviews(t.s.reviews[id]), nil
}
func (t *tx) InsertReview(_ context.Context, r domain.ReviewSnapshot) error {
	t.s.reviews[r.TestRunID] = append(t.s.reviews[r.TestRunID], cloneReview(r))
	return nil
}
func (t *tx) InsertDecision(_ context.Context, d domain.ValidityDecision) error {
	if _, ok := t.s.decisions[d.TestRunID]; ok {
		return domain.Conflict("裁定已存在")
	}
	t.s.decisions[d.TestRunID] = cloneDecision(d)
	return nil
}
func (t *tx) UpdateDecisionArchive(_ context.Context, id, hash string) error {
	for run, d := range t.s.decisions {
		if d.ID == id {
			d.ArchiveHash = hash
			t.s.decisions[run] = d
			return nil
		}
	}
	return domain.NotFound("裁定不存在")
}
func (t *tx) FindIdempotency(_ context.Context, id string) (*domain.IdempotencyRecord, error) {
	r, ok := t.s.idem[id]
	if !ok {
		return nil, nil
	}
	c := cloneIdempotency(r)
	return &c, nil
}
func (t *tx) SaveIdempotency(_ context.Context, r domain.IdempotencyRecord) error {
	if old, ok := t.s.idem[r.RequestID]; ok {
		if old.Fingerprint != r.Fingerprint {
			return domain.Wrap(domain.ErrIdempotency, "request_id 冲突", nil)
		}
		return nil
	}
	t.s.idem[r.RequestID] = cloneIdempotency(r)
	return nil
}
func (t *tx) AppendAudit(_ context.Context, e domain.AuditEvent) error {
	e.Sequence = int64(len(t.s.audit) + 1)
	if len(t.s.audit) > 0 {
		e.PreviousHash = t.s.audit[len(t.s.audit)-1].EventHash
	}
	e.EventHash = domain.Digest(fmt.Sprintf("%d|%s|%s|%s|%s|%s|%d|%d|%s|%s", e.Sequence, e.TestRunID, e.RequestID, e.Actor, e.Action, e.FromStatus, e.BeforeRevision, e.AfterRevision, e.PayloadDigest, e.PreviousHash))
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	t.s.audit = append(t.s.audit, e)
	return nil
}

var _ = json.Valid
