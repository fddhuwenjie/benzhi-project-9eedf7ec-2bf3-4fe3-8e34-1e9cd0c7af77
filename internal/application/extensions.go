package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

type ReviseBaselineCommand struct {
	RequestID, Actor, RunID, RigID, EngineRef, Objective string
	ScheduledStart, ScheduledEnd                         time.Time
	ExpectedChannels                                     []string
	ExpectedRevision                                     int64
}
type BaselinePrecheck struct {
	Run                domain.TestRun `json:"run"`
	NormalizedChannels []string       `json:"normalized_channels"`
	CandidateHash      string         `json:"candidate_baseline_hash"`
	Differences        map[string]any `json:"differences"`
	Revision           int64          `json:"revision"`
}

func (s *Service) FreezeBaselineChecked(ctx context.Context, id, actor, request string, expected int64, candidate string) (Result, error) {
	if e := s.check(request, actor); e != nil {
		return Result{}, e
	}
	fp := domain.Digest([]any{"freeze_checked", id, expected, candidate})
	var out Result
	if ok, e := s.replay(ctx, request, fp, &out); ok || e != nil {
		return out, e
	}
	e := s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		r, e := tx.GetRun(ctx, id)
		if e != nil {
			return e
		}
		if r.Status != domain.StatusDraft {
			return domain.Transition("只有草稿任务可以冻结基线")
		}
		if r.Revision != expected {
			return domain.RevisionError(expected, r.Revision)
		}
		ch, e := domain.NormalizeChannels(r.ExpectedChannels)
		if e != nil {
			return e
		}
		h := domain.BaselineHash(r.RigID, r.EngineRef, r.Objective, ch, r.ScheduledStart.Format(time.RFC3339Nano), r.ScheduledEnd.Format(time.RFC3339Nano))
		if candidate == "" || candidate != h {
			return domain.Conflict("候选 baseline_hash 已过期")
		}
		r.ExpectedChannels = ch
		r.BaselineHash = h
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
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: id, RequestID: request, Actor: actor, Action: "FREEZE_BASELINE", FromStatus: domain.StatusDraft, ToStatus: r.Status, BeforeRevision: expected, AfterRevision: r.Revision, PayloadDigest: domain.Digest(r), OccurredAt: s.now().UTC()})
	})
	return out, e
}
func (s *Service) ReviseBaseline(ctx context.Context, c ReviseBaselineCommand) (Result, error) {
	if e := s.check(c.RequestID, c.Actor); e != nil {
		return Result{}, e
	}
	ch, e := domain.NormalizeChannels(c.ExpectedChannels)
	if e != nil {
		return Result{}, e
	}
	r, e := s.repo.GetRun(ctx, c.RunID)
	if e != nil {
		return Result{}, e
	}
	if r.Status != domain.StatusDraft {
		return Result{}, domain.Transition("只有草稿任务可以修订基线")
	}
	prev := &domain.BaselineDraft{RigID: r.RigID, EngineRef: r.EngineRef, Objective: r.Objective, ScheduledStart: r.ScheduledStart, ScheduledEnd: r.ScheduledEnd, ExpectedChannels: append([]string{}, r.ExpectedChannels...), Revision: r.Revision}
	n := domain.TestRun{ID: r.ID, RigID: strings.TrimSpace(c.RigID), EngineRef: strings.TrimSpace(c.EngineRef), Objective: strings.TrimSpace(c.Objective), ScheduledStart: c.ScheduledStart, ScheduledEnd: c.ScheduledEnd, ExpectedChannels: ch, Status: r.Status, Revision: r.Revision, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, PreviousDraft: prev}
	if e = domain.ValidateBaseline(n); e != nil {
		return Result{}, e
	}
	fp := domain.Digest(c)
	var out Result
	if ok, e := s.replay(ctx, c.RequestID, fp, &out); ok || e != nil {
		return out, e
	}
	e = s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		cur, e := tx.GetRun(ctx, c.RunID)
		if e != nil {
			return e
		}
		if cur.Status != domain.StatusDraft {
			return domain.Transition("只有草稿任务可以修订基线")
		}
		n.Revision = cur.Revision + 1
		if e = tx.UpdateRun(ctx, n, c.ExpectedRevision); e != nil {
			return e
		}
		out = Result{Run: n, Message: "基线草稿已修订"}
		b, _ := json.Marshal(out)
		if e = tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: c.RequestID, Fingerprint: fp, StatusCode: 200, Response: b}); e != nil {
			return e
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: c.RunID, RequestID: c.RequestID, Actor: c.Actor, Action: "REVISE_BASELINE", FromStatus: cur.Status, ToStatus: n.Status, BeforeRevision: cur.Revision, AfterRevision: n.Revision, PayloadDigest: domain.Digest(n), OccurredAt: s.now().UTC()})
	})
	return out, e
}
func (s *Service) PrecheckBaseline(ctx context.Context, id string) (BaselinePrecheck, error) {
	r, e := s.repo.GetRun(ctx, id)
	if e != nil {
		return BaselinePrecheck{}, e
	}
	ch, e := domain.NormalizeChannels(r.ExpectedChannels)
	if e != nil {
		return BaselinePrecheck{}, e
	}
	h := domain.BaselineHash(r.RigID, r.EngineRef, r.Objective, ch, r.ScheduledStart.Format(time.RFC3339Nano), r.ScheduledEnd.Format(time.RFC3339Nano))
	d := map[string]any{}
	if p := r.PreviousDraft; p != nil {
		if p.RigID != r.RigID {
			d["rig_id"] = map[string]any{"before": p.RigID, "after": r.RigID}
		}
		if p.EngineRef != r.EngineRef {
			d["engine_ref"] = map[string]any{"before": p.EngineRef, "after": r.EngineRef}
		}
		if p.Objective != r.Objective {
			d["objective"] = map[string]any{"before": p.Objective, "after": r.Objective}
		}
		if !p.ScheduledStart.Equal(r.ScheduledStart) || !p.ScheduledEnd.Equal(r.ScheduledEnd) {
			d["sampling_window"] = map[string]any{"before": []time.Time{p.ScheduledStart, p.ScheduledEnd}, "after": []time.Time{r.ScheduledStart, r.ScheduledEnd}}
		}
		if domain.Digest(p.ExpectedChannels) != domain.Digest(ch) {
			d["expected_channels"] = map[string]any{"before": p.ExpectedChannels, "after": ch}
		}
	}
	return BaselinePrecheck{Run: r, NormalizedChannels: ch, CandidateHash: h, Differences: d, Revision: r.Revision}, nil
}

type TriageItem struct {
	AnomalyID, Kind, Severity, Impact, Disposition, TimeRange string
	Channels                                                  []string
}

func (s *Service) TriageBatch(ctx context.Context, runID, actor, request string, expected int64, items []TriageItem) (Result, error) {
	if e := s.check(request, actor); e != nil {
		return Result{}, e
	}
	if len(items) == 0 {
		return Result{}, domain.Invalid("items 不能为空")
	}
	fp := domain.Digest([]any{runID, expected, items})
	var out Result
	if ok, e := s.replay(ctx, request, fp, &out); ok || e != nil {
		return out, e
	}
	e := s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		r, e := tx.GetRun(ctx, runID)
		if e != nil {
			return e
		}
		if r.Status != domain.StatusDataChecked && r.Status != domain.StatusTriaged {
			return domain.Transition("当前状态不能分诊")
		}
		seen := map[string]bool{}
		as, e := tx.GetAnomalies(ctx, runID)
		if e != nil {
			return e
		}
		for _, it := range items {
			if seen[it.AnomalyID] {
				return domain.Invalid("anomaly_id 不得重复")
			}
			seen[it.AnomalyID] = true
			found := false
			for _, a := range as {
				if a.ID == it.AnomalyID {
					found = true
					a.Kind = it.Kind
					a.Severity = it.Severity
					a.AffectedChannels = it.Channels
					a.TimeRange = it.TimeRange
					a.ImpactStatement = it.Impact
					a.Disposition = it.Disposition
					baseline := map[string]bool{}
					for _, ch := range r.ExpectedChannels {
						baseline[ch] = true
					}
					for _, ch := range it.Channels {
						if !baseline[ch] {
							return domain.Invalid("受影响测点必须来自冻结基线")
						}
					}
					if e = domain.ValidateTriage(a, it.Channels); e != nil {
						return e
					}
					a.Status = "TRIAGED"
					a.Owner = actor
					a.UpdatedAt = s.now().UTC()
					if e = tx.UpdateAnomaly(ctx, a); e != nil {
						return e
					}
				}
			}
			if !found {
				return domain.NotFound("异常不存在")
			}
		}
		from := r.Status
		allTriaged := true
		for _, a := range as {
			if a.Status == "OPEN" && !seen[a.ID] {
				allTriaged = false
			}
		}
		if allTriaged {
			r.Status = domain.StatusTriaged
		}
		r.Revision++
		if e = tx.UpdateRun(ctx, r, expected); e != nil {
			return e
		}
		remaining := []string{}
		for _, a := range as {
			if a.Status == "OPEN" && !seen[a.ID] {
				remaining = append(remaining, a.ID)
			}
		}
		sort.Strings(remaining)
		out = Result{Run: r, Message: "异常已批量分诊", RemainingAnomalyIDs: remaining}
		b, _ := json.Marshal(out)
		if e = tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: request, Fingerprint: fp, StatusCode: 200, Response: b}); e != nil {
			return e
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: runID, RequestID: request, Actor: actor, Action: "TRIAGE_BATCH", FromStatus: from, ToStatus: r.Status, BeforeRevision: expected, AfterRevision: r.Revision, PayloadDigest: fp, OccurredAt: s.now().UTC()})
	})
	return out, e
}

func (s *Service) DecisionReadiness(ctx context.Context, id string) (map[string]any, error) {
	r, e := s.repo.GetRun(ctx, id)
	if e != nil {
		return nil, e
	}
	// 读取复核、异常、数据包与证据等全部依据；任一读取失败都原样返回存储错误，
	// 避免把存储故障误报为缺少有效快照等业务阻断。只有在全部依据读取成功后才生成阻断原因。
	rs, e := s.repo.GetReviews(ctx, id)
	if e != nil {
		return nil, e
	}
	as, e := s.repo.GetAnomalies(ctx, id)
	if e != nil {
		return nil, e
	}
	pk, e := s.repo.GetPackages(ctx, id)
	if e != nil {
		return nil, e
	}
	ev, e := s.repo.GetEvidence(ctx, id)
	if e != nil {
		return nil, e
	}
	current := []domain.DispositionEvidence{}
	for _, x := range ev {
		if x.ReplacedBy == "" {
			current = append(current, x)
		}
	}
	reasons := []string{}
	if r.Status != domain.StatusReviewed {
		reasons = append(reasons, "任务尚未完成独立复核")
	}
	latest, ok := domain.LatestReview(rs)
	if !ok || latest.Outcome != "APPROVED" || latest.SnapshotHash == "" {
		reasons = append(reasons, "缺少锁定通过复核快照")
	} else if domain.Digest([]any{pk, as, current, latest.Checklist, latest.DifferenceDigest}) != latest.SnapshotHash {
		reasons = append(reasons, "复核快照与当前依据不一致")
	}
	for _, a := range as {
		if a.Status == "NEEDS_REVISION" {
			reasons = append(reasons, "存在待修订异常:"+a.ID)
		}
	}
	return map[string]any{"ready": len(reasons) == 0, "blocking_reasons": reasons, "revision": r.Revision, "review": latest}, nil
}

func (s *Service) IntegrityReport(ctx context.Context, id string) (map[string]any, error) {
	a, e := s.Detail(ctx, id)
	if e != nil {
		return nil, e
	}
	if a.TestRun.Status != domain.StatusArchived {
		return nil, domain.Transition("档案尚未封存")
	}
	if e = s.repo.VerifyAudit(ctx); e != nil {
		return nil, e
	}
	expected := domain.ArchiveDigest(a)
	ok := expected == a.ArchiveHash
	checks := []map[string]any{{"name": "baseline_hash", "expected": domain.BaselineHash(a.TestRun.RigID, a.TestRun.EngineRef, a.TestRun.Objective, a.TestRun.ExpectedChannels, a.TestRun.ScheduledStart.Format(time.RFC3339Nano), a.TestRun.ScheduledEnd.Format(time.RFC3339Nano)), "actual": a.TestRun.BaselineHash}, {"name": "audit_chain", "expected": "continuous", "actual": "continuous"}, {"name": "archive_hash", "expected": expected, "actual": a.ArchiveHash}}
	for _, p := range a.DataPackages {
		checks = append(checks, map[string]any{"name": "manifest_hash:" + p.ID, "expected": domain.ManifestHash(p.Files), "actual": p.ManifestHash})
	}
	for _, c := range checks {
		if c["expected"] != c["actual"] {
			ok = false
			c["passed"] = false
		} else {
			c["passed"] = true
		}
	}
	return map[string]any{"ok": ok, "archive_hash": a.ArchiveHash, "expected_archive_hash": expected, "checked_at": s.now().UTC(), "checks": checks}, nil
}

func sortAnomalies(as []domain.Anomaly) {
	sort.SliceStable(as, func(i, j int) bool {
		si := as[i].Severity
		sj := as[j].Severity
		rank := map[string]int{"BLOCKING": 0, "MAJOR": 1, "MINOR": 2}
		if rank[si] != rank[sj] {
			return rank[si] < rank[sj]
		}
		if as[i].UpdatedAt.Equal(as[j].UpdatedAt) {
			return as[i].ID < as[j].ID
		}
		return as[i].UpdatedAt.Before(as[j].UpdatedAt)
	})
}
