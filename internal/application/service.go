package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo domain.Repository
	now  func() time.Time
}
type Archive = domain.Archive

func New(repo domain.Repository) *Service { return &Service{repo: repo, now: time.Now} }
func newID() string                       { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }

type CreateRunCommand struct {
	RequestID, Actor, RigID, EngineRef, Objective string
	ScheduledStart, ScheduledEnd                  time.Time
	ExpectedChannels                              []string
	ExpectedRevision                              int64
}
type RegisterPackageCommand struct {
	RequestID, Actor, RunID string
	Package                 domain.DataPackage
	ExpectedRevision        int64
	CandidatePackageHash    string
}
type TriageCommand struct {
	RequestID, Actor, RunID, AnomalyID, Severity, Kind, Impact, Disposition, TimeRange string
	Channels                                                                           []string
	ExpectedRevision                                                                   int64
}
type EvidenceCommand struct {
	RequestID, Actor, RunID, AnomalyID string
	Evidence                           domain.DispositionEvidence
	ExpectedRevision                   int64
}
type ReviewCommand struct {
	RequestID, Actor, RunID, Outcome, Notes, Reason string
	ExpectedRevision                                int64
	Checklist                                       []domain.ReviewChecklistItem
	Targets                                         []domain.ReviewReturnTarget
	ComparisonReviewID                              string
}
type DecisionCommand struct {
	RequestID, Actor, RunID, Verdict string
	Objectives, Limitations          []string
	ExpectedRevision                 int64
}
type Result struct {
	Run                 domain.TestRun `json:"run"`
	Message             string         `json:"message"`
	PassedGates         int            `json:"passed_gates,omitempty"`
	FailedGates         int            `json:"failed_gates,omitempty"`
	BlockingAnomalies   int            `json:"blocking_anomalies,omitempty"`
	RemainingAnomalyIDs []string       `json:"remaining_anomaly_ids,omitempty"`
}

type EvidenceBatchItem struct {
	AnomalyID string                     `json:"anomaly_id"`
	Evidence  domain.DispositionEvidence `json:"evidence"`
}

type EvidenceBatchCommand struct {
	RequestID, Actor, RunID string
	ExpectedRevision        int64
	Items                   []EvidenceBatchItem
}

func (s *Service) check(cmdID, actor string) error {
	if err := domain.ValidateRequestID(strings.TrimSpace(cmdID)); err != nil {
		return err
	}
	return domain.ValidateActor(actor)
}
func (s *Service) replay(ctx context.Context, id, fp string, out any) (bool, error) {
	found := false
	err := s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		r, e := tx.FindIdempotency(ctx, id)
		if e != nil || r == nil {
			return e
		}
		if r.Fingerprint != fp {
			return domain.Wrap(domain.ErrIdempotency, "request_id 已用于其他命令", nil)
		}
		found = true
		return json.Unmarshal(r.Response, out)
	})
	return found, err
}
func (s *Service) idem(ctx context.Context, id, fp string) (*domain.IdempotencyRecord, error) {
	var out *domain.IdempotencyRecord
	err := s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		r, e := tx.FindIdempotency(ctx, id)
		if e != nil {
			return e
		}
		if r != nil {
			if r.Fingerprint != fp {
				return domain.Wrap(domain.ErrIdempotency, "request_id 已用于其他命令", nil)
			}
			out = r
		}
		return nil
	})
	return out, err
}
func (s *Service) commit(ctx context.Context, id, fp string, status int, payload []byte) error {
	return s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		return tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: id, Fingerprint: fp, StatusCode: status, Response: payload})
	})
}
func (s *Service) CreateRun(ctx context.Context, c CreateRunCommand) (Result, error) {
	if e := s.check(c.RequestID, c.Actor); e != nil {
		return Result{}, e
	}
	r := domain.TestRun{ID: newID(), RigID: c.RigID, EngineRef: c.EngineRef, Objective: c.Objective, ScheduledStart: c.ScheduledStart, ScheduledEnd: c.ScheduledEnd, ExpectedChannels: c.ExpectedChannels, Status: domain.StatusDraft, Revision: 1, CreatedBy: c.Actor, CreatedAt: s.now().UTC()}
	if e := domain.ValidateBaseline(r); e != nil {
		return Result{}, e
	}
	fp := domain.Digest(c)
	var out Result
	if ok, e := s.replay(ctx, c.RequestID, fp, &out); ok || e != nil {
		return out, e
	}
	err := s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		if e := tx.InsertRun(ctx, r); e != nil {
			return e
		}
		out = Result{Run: r, Message: "试车任务已建立"}
		b, _ := json.Marshal(out)
		if e := tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: c.RequestID, Fingerprint: fp, StatusCode: 201, Response: b}); e != nil {
			return e
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: r.ID, RequestID: c.RequestID, Actor: c.Actor, Action: "CREATE_RUN", FromStatus: "", ToStatus: r.Status, BeforeRevision: 0, AfterRevision: r.Revision, PayloadDigest: domain.Digest(c), OccurredAt: s.now()})
	})
	return out, err
}
func (s *Service) FreezeBaseline(ctx context.Context, id, actor, request string, expected int64) (Result, error) {
	if e := s.check(request, actor); e != nil {
		return Result{}, e
	}
	fp := domain.Digest([]any{"freeze", id, expected})
	var out Result
	if ok, e := s.replay(ctx, request, fp, &out); ok || e != nil {
		return out, e
	}
	err := s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		r, e := tx.GetRun(ctx, id)
		if e != nil {
			return e
		}
		if r.Status != domain.StatusDraft {
			return domain.Transition("只有草稿任务可以冻结基线")
		}
		r.BaselineHash = domain.BaselineHash(r.RigID, r.EngineRef, r.Objective, r.ExpectedChannels, r.ScheduledStart.Format(time.RFC3339Nano), r.ScheduledEnd.Format(time.RFC3339Nano))
		r.Status = domain.StatusBaselined
		r.Revision++
		if e = tx.UpdateRun(ctx, r, expected); e != nil {
			return e
		}
		out = Result{Run: r, Message: "测点基线已冻结"}
		b, _ := json.Marshal(out)
		if e = tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: request, Fingerprint: fp, StatusCode: 200, Response: b}); e != nil {
			return e
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: id, RequestID: request, Actor: actor, Action: "FREEZE_BASELINE", FromStatus: domain.StatusDraft, ToStatus: r.Status, BeforeRevision: expected, AfterRevision: r.Revision, PayloadDigest: domain.Digest(r), OccurredAt: s.now()})
	})
	return out, err
}
func (s *Service) RegisterPackage(ctx context.Context, c RegisterPackageCommand) (Result, error) {
	if e := s.check(c.RequestID, c.Actor); e != nil {
		return Result{}, e
	}
	if e := domain.ValidatePackageCandidate(c.Package); e != nil {
		return Result{}, e
	}
	fp := domain.Digest(c)
	var out Result
	if ok, e := s.replay(ctx, c.RequestID, fp, &out); ok || e != nil {
		return out, e
	}
	err := s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		r, e := tx.GetRun(ctx, c.RunID)
		if e != nil {
			return e
		}
		if r.Status != domain.StatusBaselined {
			return domain.Transition("请先冻结基线")
		}
		p := c.Package
		preview, e := packagePreview(r, p)
		if e != nil {
			return e
		}
		if c.CandidatePackageHash == "" || c.CandidatePackageHash != preview.CandidatePackageHash {
			return domain.Wrap(domain.ErrConflict, "数据包预演摘要已过期", map[string]any{"candidate_package_hash": c.CandidatePackageHash, "current_candidate_package_hash": preview.CandidatePackageHash, "actual_revision": r.Revision})
		}
		p.ID = newID()
		p.TestRunID = r.ID
		p.RegisteredBy = c.Actor
		p.RegisteredAt = s.now().UTC()
		p.ManifestHash = preview.ManifestHash
		gates, generated := preview.GateResults, preview.ExpectedAnomalies
		p.GateResults = gates
		if e = tx.InsertPackage(ctx, p); e != nil {
			return e
		}
		for i := range generated {
			generated[i].ID = newID()
			generated[i].TestRunID = r.ID
			generated[i].UpdatedAt = s.now().UTC()
			generated[i].Owner = c.Actor
		}
		if e = tx.InsertAnomalies(ctx, generated); e != nil {
			return e
		}
		r.Status = domain.StatusDataChecked
		r.Revision++
		if e = tx.UpdateRun(ctx, r, c.ExpectedRevision); e != nil {
			return e
		}
		passed := 0
		for _, g := range gates {
			if g.Passed {
				passed++
			}
		}
		out = Result{Run: r, Message: fmt.Sprintf("数据包已登记，生成 %d 项异常", len(generated)), PassedGates: passed, FailedGates: len(gates) - passed, BlockingAnomalies: len(generated)}
		b, _ := json.Marshal(out)
		if e = tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: c.RequestID, Fingerprint: fp, StatusCode: 200, Response: b}); e != nil {
			return e
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: r.ID, RequestID: c.RequestID, Actor: c.Actor, Action: "REGISTER_PACKAGE", FromStatus: domain.StatusBaselined, ToStatus: r.Status, BeforeRevision: c.ExpectedRevision, AfterRevision: r.Revision, PayloadDigest: domain.Digest(p), OccurredAt: s.now()})
	})
	return out, err
}
