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
func (s *Memory) WithinTx(ctx context.Context, fn func(domain.Transaction) error) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	backup := s.snapshotLocked()
	committed := false
	// A panic inside fn (or saveLocked) unwinds without restoring the
	// in-memory state, leaving half-written mutations visible to callers that
	// recover upstream. Restore the pre-transaction snapshot on the way out
	// and re-panic with the original value so error and commit semantics stay
	// unchanged.
	defer func() {
		if !committed {
			s.restoreLocked(backup)
		}
		if r := recover(); r != nil {
			panic(r)
		}
	}()
	if e := fn(&tx{s: s}); e != nil {
		return e
	}
	if e := s.saveLocked(); e != nil {
		return e
	}
	committed = true
	return nil
}
func (s *Memory) GetRun(_ context.Context, id string) (domain.TestRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.runs[id]
	if !ok {
		return r, domain.NotFound("试车任务不存在")
	}
	return r, nil
}
func (s *Memory) ListRuns(_ context.Context) ([]domain.TestRun, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o := make([]domain.TestRun, 0, len(s.runs))
	for _, r := range s.runs {
		o = append(o, r)
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
	return append([]domain.Anomaly{}, s.anomalies[id]...), nil
}
func (s *Memory) GetEvidence(_ context.Context, id string) ([]domain.DispositionEvidence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.DispositionEvidence{}
	for _, a := range s.anomalies[id] {
		out = append(out, s.evidence[a.ID]...)
	}
	return out, nil
}
func (s *Memory) GetReviews(_ context.Context, id string) ([]domain.ReviewSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.ReviewSnapshot{}, s.reviews[id]...), nil
}
func (s *Memory) GetDecision(_ context.Context, id string) (domain.ValidityDecision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.decisions[id]
	if !ok {
		return d, domain.NotFound("裁定不存在")
	}
	return d, nil
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
	return o, nil
}

type tx struct{ s *Memory }

func (t *tx) GetRun(_ context.Context, id string) (domain.TestRun, error) {
	r, ok := t.s.runs[id]
	if !ok {
		return r, domain.NotFound("试车任务不存在")
	}
	return r, nil
}
func (t *tx) GetPackages(_ context.Context, id string) ([]domain.DataPackage, error) {
	return clonePackages(t.s.packages[id]), nil
}
func (t *tx) InsertRun(_ context.Context, r domain.TestRun) error {
	if _, ok := t.s.runs[r.ID]; ok {
		return domain.Conflict("试车 ID 已存在")
	}
	t.s.runs[r.ID] = r
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
	t.s.runs[r.ID] = r
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
func (t *tx) InsertAnomalies(_ context.Context, a []domain.Anomaly) error {
	for _, x := range a {
		t.s.anomalies[x.TestRunID] = append(t.s.anomalies[x.TestRunID], x)
	}
	return nil
}
func (t *tx) GetAnomalies(_ context.Context, id string) ([]domain.Anomaly, error) {
	return append([]domain.Anomaly{}, t.s.anomalies[id]...), nil
}
func (t *tx) UpdateAnomaly(_ context.Context, a domain.Anomaly) error {
	xs := t.s.anomalies[a.TestRunID]
	for i := range xs {
		if xs[i].ID == a.ID {
			xs[i] = a
			t.s.anomalies[a.TestRunID] = xs
			return nil
		}
	}
	return domain.NotFound("异常不存在")
}
func (t *tx) InsertEvidence(_ context.Context, e domain.DispositionEvidence) error {
	t.s.evidence[e.AnomalyID] = append(t.s.evidence[e.AnomalyID], e)
	return nil
}
func (t *tx) UpdateEvidence(_ context.Context, e domain.DispositionEvidence) error {
	xs := t.s.evidence[e.AnomalyID]
	for i := range xs {
		if xs[i].ID == e.ID {
			xs[i] = e
			t.s.evidence[e.AnomalyID] = xs
			return nil
		}
	}
	return domain.NotFound("处置证据不存在")
}
func (t *tx) GetEvidence(_ context.Context, id string) ([]domain.DispositionEvidence, error) {
	return append([]domain.DispositionEvidence{}, t.s.evidence[id]...), nil
}
func (t *tx) GetReviews(_ context.Context, id string) ([]domain.ReviewSnapshot, error) {
	return append([]domain.ReviewSnapshot{}, t.s.reviews[id]...), nil
}
func (t *tx) InsertReview(_ context.Context, r domain.ReviewSnapshot) error {
	t.s.reviews[r.TestRunID] = append(t.s.reviews[r.TestRunID], r)
	return nil
}
func (t *tx) InsertDecision(_ context.Context, d domain.ValidityDecision) error {
	if _, ok := t.s.decisions[d.TestRunID]; ok {
		return domain.Conflict("裁定已存在")
	}
	t.s.decisions[d.TestRunID] = d
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
	return &r, nil
}
func (t *tx) SaveIdempotency(_ context.Context, r domain.IdempotencyRecord) error {
	if old, ok := t.s.idem[r.RequestID]; ok {
		if old.Fingerprint != r.Fingerprint {
			return domain.Wrap(domain.ErrIdempotency, "request_id 冲突", nil)
		}
		return nil
	}
	t.s.idem[r.RequestID] = r
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
