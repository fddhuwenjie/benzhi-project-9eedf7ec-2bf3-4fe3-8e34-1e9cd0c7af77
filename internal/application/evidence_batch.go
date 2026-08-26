package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"encoding/json"
	"sort"
	"strings"
)

func (s *Service) EvidencePrecheck(ctx context.Context, runID string) (domain.EvidencePrecheck, error) {
	r, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return domain.EvidencePrecheck{}, err
	}
	anomalies, err := s.repo.GetAnomalies(ctx, runID)
	if err != nil {
		return domain.EvidencePrecheck{}, err
	}
	evidence, err := s.repo.GetEvidence(ctx, runID)
	if err != nil {
		return domain.EvidencePrecheck{}, err
	}
	out := domain.EvidencePrecheck{Revision: r.Revision, ExpectedStatus: domain.StatusReviewPending, Items: []domain.EvidencePrecheckItem{}, RemainingIDs: []string{}}
	for _, a := range anomalies {
		item := domain.EvidencePrecheckItem{AnomalyID: a.ID, Status: a.Status, MissingFields: []string{}, BlockingReasons: []string{}}
		if ev, ok := domain.CurrentEvidence(evidence, a.ID); ok {
			item.EffectiveEvidence = &ev
			if a.Status == "NEEDS_REVISION" {
				item.SupersedesEvidenceID = ev.ID
			}
		}
		if a.Status != "CLOSED" {
			item.MissingFields = []string{"method", "rationale", "before_digest", "after_digest", "evidence_refs"}
			item.BlockingReasons = append(item.BlockingReasons, "异常尚未形成有效闭环证据")
			out.RemainingIDs = append(out.RemainingIDs, a.ID)
		}
		if a.Status == "NEEDS_REVISION" {
			item.BlockingReasons = append(item.BlockingReasons, "退回证据必须由新版本精确取代")
		}
		out.Items = append(out.Items, item)
	}
	if len(out.RemainingIDs) > 0 {
		out.ExpectedStatus = r.Status
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].AnomalyID < out.Items[j].AnomalyID })
	sort.Strings(out.RemainingIDs)
	return out, nil
}

func (s *Service) SubmitEvidenceBatch(ctx context.Context, c EvidenceBatchCommand) (Result, error) {
	if err := s.check(c.RequestID, c.Actor); err != nil {
		return Result{}, err
	}
	if len(c.Items) == 0 {
		return Result{}, domain.FieldInvalid("items", "items 不能为空")
	}
	fp := domain.Digest(c)
	var out Result
	if ok, err := s.replay(ctx, c.RequestID, fp, &out); ok || err != nil {
		return out, err
	}
	err := s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		r, err := tx.GetRun(ctx, c.RunID)
		if err != nil {
			return err
		}
		if r.Status != domain.StatusTriaged && r.Status != domain.StatusDataChecked {
			return domain.Transition("当前状态不能提交处置证据")
		}
		if r.Revision != c.ExpectedRevision {
			return domain.RevisionError(c.ExpectedRevision, r.Revision)
		}
		anomalies, err := tx.GetAnomalies(ctx, c.RunID)
		if err != nil {
			return err
		}
		byID := map[string]domain.Anomaly{}
		allEvidence := []domain.DispositionEvidence{}
		for _, a := range anomalies {
			byID[a.ID] = a
			es, e := tx.GetEvidence(ctx, a.ID)
			if e != nil {
				return e
			}
			allEvidence = append(allEvidence, es...)
		}
		evidenceOwner := map[string]string{}
		for _, e := range allEvidence {
			evidenceOwner[e.ID] = e.AnomalyID
		}
		seen := map[string]bool{}
		rowErrors := []map[string]any{}
		for i, item := range c.Items {
			fields := []string{}
			messages := []string{}
			if seen[item.AnomalyID] {
				fields = append(fields, "anomaly_id")
				messages = append(messages, "anomaly_id 在请求内不得重复")
			}
			seen[item.AnomalyID] = true
			a, ok := byID[item.AnomalyID]
			if !ok {
				fields = append(fields, "anomaly_id")
				messages = append(messages, "异常不属于当前试车")
			} else {
				if a.Status != "TRIAGED" && a.Status != "NEEDS_REVISION" {
					fields = append(fields, "anomaly_id")
					messages = append(messages, "异常必须先完成分诊")
				}
				method := strings.ToUpper(strings.TrimSpace(item.Evidence.Method))
				if method != "EXCLUDE" && method != "CORRECT" && method != "ACCEPT" {
					fields = append(fields, "method")
					messages = append(messages, "method 只能是 EXCLUDE、CORRECT 或 ACCEPT")
				}
				item.Evidence.Method = method
				c.Items[i].Evidence.Method = method
				if e := domain.ValidateEvidence(a, item.Evidence); e != nil {
					messages = append(messages, e.Error())
				}
				if owner := evidenceOwner[item.Evidence.SupersedesEvidenceID]; item.Evidence.SupersedesEvidenceID != "" && owner != "" && owner != a.ID {
					fields = append(fields, "supersedes_evidence_id")
					messages = append(messages, "不得引用其他异常的证据")
				}
				if a.Status != "NEEDS_REVISION" && item.Evidence.SupersedesEvidenceID != "" {
					fields = append(fields, "supersedes_evidence_id")
					messages = append(messages, "仅待修订异常可以取代历史证据")
				}
				if a.Status == "NEEDS_REVISION" {
					cur, found := domain.CurrentEvidence(allEvidence, a.ID)
					if !found || item.Evidence.SupersedesEvidenceID != cur.ID {
						fields = append(fields, "supersedes_evidence_id")
						messages = append(messages, "必须精确引用当前未被取代的 evidence_id")
					}
				}
			}
			if len(messages) > 0 {
				rowErrors = append(rowErrors, map[string]any{"index": i, "anomaly_id": item.AnomalyID, "fields": fields, "messages": messages})
			}
		}
		if len(rowErrors) > 0 {
			return domain.Wrap(domain.ErrInvalidInput, "批量证据校验失败", map[string]any{"items": rowErrors})
		}
		now := s.now().UTC()
		newEvidence := make([]domain.DispositionEvidence, 0, len(c.Items))
		for _, item := range c.Items {
			a := byID[item.AnomalyID]
			ev := item.Evidence
			ev.ID = newID()
			ev.AnomalyID = a.ID
			ev.SubmittedBy = c.Actor
			ev.SubmittedAt = now
			if ev.SupersedesEvidenceID != "" {
				for _, old := range allEvidence {
					if old.ID == ev.SupersedesEvidenceID {
						old.ReplacedBy = ev.ID
						if err = tx.UpdateEvidence(ctx, old); err != nil {
							return err
						}
						break
					}
				}
			}
			if err = tx.InsertEvidence(ctx, ev); err != nil {
				return err
			}
			newEvidence = append(newEvidence, ev)
			a.Status = "CLOSED"
			a.Disposition = ev.Method + ": " + ev.Rationale
			a.Owner = c.Actor
			a.UpdatedAt = now
			if err = tx.UpdateAnomaly(ctx, a); err != nil {
				return err
			}
			byID[a.ID] = a
		}
		remaining := []string{}
		for _, a := range anomalies {
			if byID[a.ID].Status != "CLOSED" {
				remaining = append(remaining, a.ID)
			}
		}
		sort.Strings(remaining)
		from := r.Status
		if len(remaining) == 0 {
			r.Status = domain.StatusReviewPending
		}
		r.Revision++
		if err = tx.UpdateRun(ctx, r, c.ExpectedRevision); err != nil {
			return err
		}
		out = Result{Run: r, Message: "处置证据已批量提交", RemainingAnomalyIDs: remaining}
		b, _ := json.Marshal(out)
		if err = tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: c.RequestID, Fingerprint: fp, StatusCode: 200, Response: b}); err != nil {
			return err
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: c.RunID, RequestID: c.RequestID, Actor: c.Actor, Action: "SUBMIT_EVIDENCE_BATCH", FromStatus: from, ToStatus: r.Status, BeforeRevision: c.ExpectedRevision, AfterRevision: r.Revision, PayloadDigest: domain.Digest(newEvidence), OccurredAt: now})
	})
	return out, err
}
