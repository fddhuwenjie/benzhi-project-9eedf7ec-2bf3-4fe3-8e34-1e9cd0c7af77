package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"encoding/json"
	"sort"
	"strings"
)

func (s *Service) Triage(ctx context.Context, c TriageCommand) (Result, error) {
	if e := s.check(c.RequestID, c.Actor); e != nil {
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
		if r.Status != domain.StatusDataChecked && r.Status != domain.StatusTriaged {
			return domain.Transition("当前状态不能分诊")
		}
		as, e := tx.GetAnomalies(ctx, c.RunID)
		if e != nil {
			return e
		}
		var target *domain.Anomaly
		for i := range as {
			if as[i].ID == c.AnomalyID {
				target = &as[i]
				break
			}
		}
		if target == nil {
			return domain.NotFound("异常不存在")
		}
		from := r.Status
		target.Kind = c.Kind
		target.Severity = c.Severity
		target.AffectedChannels = c.Channels
		target.TimeRange = c.TimeRange
		target.ImpactStatement = c.Impact
		target.Disposition = c.Disposition
		baseline := map[string]bool{}
		for _, ch := range r.ExpectedChannels {
			baseline[ch] = true
		}
		for _, ch := range c.Channels {
			if !baseline[ch] {
				return domain.Invalid("受影响测点必须来自冻结基线")
			}
		}
		if e = domain.ValidateTriage(*target, c.Channels); e != nil {
			return e
		}
		target.Status = "TRIAGED"
		target.Owner = c.Actor
		target.UpdatedAt = s.now().UTC()
		if e = tx.UpdateAnomaly(ctx, *target); e != nil {
			return e
		}
		allTriaged := true
		for _, x := range as {
			if x.ID == target.ID {
				x = *target
			}
			if x.Status == "OPEN" {
				allTriaged = false
			}
		}
		if allTriaged {
			r.Status = domain.StatusTriaged
		}
		r.Revision++
		if e = tx.UpdateRun(ctx, r, c.ExpectedRevision); e != nil {
			return e
		}
		remaining := []string{}
		for _, x := range as {
			if x.ID != target.ID && x.Status == "OPEN" {
				remaining = append(remaining, x.ID)
			}
		}
		sort.Strings(remaining)
		out = Result{Run: r, Message: "异常已分诊", RemainingAnomalyIDs: remaining}
		b, _ := json.Marshal(out)
		if e = tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: c.RequestID, Fingerprint: fp, StatusCode: 200, Response: b}); e != nil {
			return e
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: c.RunID, RequestID: c.RequestID, Actor: c.Actor, Action: "TRIAGE_ANOMALY", FromStatus: from, ToStatus: r.Status, BeforeRevision: c.ExpectedRevision, AfterRevision: r.Revision, PayloadDigest: domain.Digest(target), OccurredAt: s.now()})
	})
	return out, err
}

func (s *Service) SubmitEvidence(ctx context.Context, c EvidenceCommand) (Result, error) {
	if e := s.check(c.RequestID, c.Actor); e != nil {
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
		aList, e := tx.GetAnomalies(ctx, c.RunID)
		if e != nil {
			return e
		}
		var a domain.Anomaly
		for _, x := range aList {
			if x.ID == c.AnomalyID {
				a = x
			}
		}
		if a.ID == "" {
			return domain.NotFound("异常不存在")
		}
		if r.Status != domain.StatusTriaged && r.Status != domain.StatusDataChecked {
			return domain.Transition("当前状态不能提交处置证据")
		}
		if e = domain.ValidateEvidence(a, c.Evidence); e != nil {
			return e
		}
		ev := c.Evidence
		ev.ID = newID()
		ev.AnomalyID = a.ID
		ev.SubmittedBy = c.Actor
		ev.SubmittedAt = s.now().UTC()
		if a.Status == "NEEDS_REVISION" {
			if ev.SupersedesEvidenceID == "" {
				return domain.Invalid("修订证据必须引用上一版 evidence_id")
			}
			old, er := tx.GetEvidence(ctx, a.ID)
			if er != nil {
				return er
			}
			found := false
			for i := range old {
				if old[i].ID == ev.SupersedesEvidenceID && old[i].ReplacedBy == "" {
					found = true
					old[i].ReplacedBy = ev.ID
					if er = tx.UpdateEvidence(ctx, old[i]); er != nil {
						return er
					}
				}
			}
			if !found {
				return domain.Invalid("supersedes_evidence_id 无效")
			}
		}
		if e = tx.InsertEvidence(ctx, ev); e != nil {
			return e
		}
		a.Status = "CLOSED"
		a.Disposition = ev.Rationale
		a.Owner = c.Actor
		a.UpdatedAt = s.now().UTC()
		if e = tx.UpdateAnomaly(ctx, a); e != nil {
			return e
		}
		allClosed := true
		for _, x := range aList {
			if x.ID == a.ID {
				x = a
			}
			if x.Status != "CLOSED" {
				allClosed = false
			}
		}
		from := r.Status
		if allClosed && (r.Status == domain.StatusTriaged || r.Status == domain.StatusDataChecked) {
			r.Status = domain.StatusReviewPending
		}
		r.Revision++
		if e = tx.UpdateRun(ctx, r, c.ExpectedRevision); e != nil {
			return e
		}
		out = Result{Run: r, Message: "处置证据已提交"}
		b, _ := json.Marshal(out)
		if e = tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: c.RequestID, Fingerprint: fp, StatusCode: 200, Response: b}); e != nil {
			return e
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: c.RunID, RequestID: c.RequestID, Actor: c.Actor, Action: "SUBMIT_EVIDENCE", FromStatus: from, ToStatus: r.Status, BeforeRevision: c.ExpectedRevision, AfterRevision: r.Revision, PayloadDigest: domain.Digest(ev), OccurredAt: s.now()})
	})
	return out, err
}

func (s *Service) Review(ctx context.Context, c ReviewCommand) (Result, error) {
	if e := s.check(c.RequestID, c.Actor); e != nil {
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
		if r.Status != domain.StatusReviewPending {
			return domain.Transition("当前状态不在待复核")
		}
		if e = domain.ValidateReview(c.Actor, r.CreatedBy); e != nil {
			return e
		}
		anoms, e := tx.GetAnomalies(ctx, c.RunID)
		if e != nil {
			return e
		}
		packages, e := tx.GetPackages(ctx, c.RunID)
		if e != nil {
			return e
		}
		allEvidence := []domain.DispositionEvidence{}
		for _, a := range anoms {
			evs, er := tx.GetEvidence(ctx, a.ID)
			if er != nil {
				return er
			}
			allEvidence = append(allEvidence, evs...)
			for _, ev := range evs {
				if ev.ReplacedBy == "" && ev.SubmittedBy == c.Actor {
					return domain.Forbidden("复核员不得复核本人提交的处置证据")
				}
			}
		}
		outcome := strings.ToUpper(c.Outcome)
		if outcome != "APPROVED" && outcome != "RETURNED" {
			return domain.Invalid("复核结果只能是 APPROVED 或 RETURNED")
		}
		reviews, e := tx.GetReviews(ctx, c.RunID)
		if e != nil {
			return e
		}
		latestReturnedID := ""
		for i := len(reviews) - 1; i >= 0; i-- {
			if reviews[i].Outcome == "RETURNED" {
				latestReturnedID = reviews[i].ID
				break
			}
		}
		if latestReturnedID != "" {
			if c.ComparisonReviewID != latestReturnedID {
				return domain.Wrap(domain.ErrConflict, "再审比较基准已过期", map[string]any{"comparison_review_id": latestReturnedID})
			}
			diff, er := s.reviewDifferenceData(r, reviews, anoms, packages, allEvidence)
			if er != nil {
				return er
			}
			if outcome == "APPROVED" {
				covered := map[string]bool{}
				for _, it := range c.Checklist {
					covered[it.AnomalyID] = true
				}
				missing := []string{}
				for _, it := range diff.ReturnedTargets {
					if len(it.BlockingReasons) > 0 {
						missing = append(missing, it.AnomalyID)
					}
				}
				for _, it := range diff.DriftedUntargeted {
					if !covered[it.AnomalyID] {
						missing = append(missing, it.AnomalyID)
					}
				}
				if len(missing) > 0 {
					return domain.Wrap(domain.ErrConflict, "再审差异存在阻断项", map[string]any{"blocking_anomaly_ids": missing})
				}
			}
		}
		byID := map[string]domain.Anomaly{}
		for _, a := range anoms {
			byID[a.ID] = a
		}
		seen := map[string]bool{}
		if len(c.Checklist) == 0 {
			for _, a := range anoms {
				c.Checklist = append(c.Checklist, domain.ReviewChecklistItem{AnomalyID: a.ID, DataPackageConfirmed: true, GateConfirmed: true, ImpactConfirmed: true, EvidenceConfirmed: true, BeforeAfterConfirmed: true, Passed: outcome == "APPROVED", Reason: c.Reason})
			}
		}
		if len(c.Checklist) != len(anoms) {
			return domain.Invalid("复核清单必须覆盖每项异常")
		}
		for i, it := range c.Checklist {
			if seen[it.AnomalyID] || byID[it.AnomalyID].ID == "" {
				return domain.Invalid("checklist anomaly_id 重复或不属于当前试车")
			}
			seen[it.AnomalyID] = true
			if outcome == "APPROVED" && (!it.Passed || !it.DataPackageConfirmed || !it.GateConfirmed || !it.ImpactConfirmed || !it.EvidenceConfirmed || !it.BeforeAfterConfirmed) {
				return domain.Invalid("通过复核必须完成每项确认")
			}
			if outcome == "RETURNED" && !it.Passed && strings.TrimSpace(it.Reason) == "" {
				return domain.Invalid("不通过清单项必须填写原因")
			}
			_ = i
		}
		if outcome == "RETURNED" {
			if len(c.Targets) == 0 && len(anoms) > 0 && strings.TrimSpace(c.Reason) != "" {
				c.Targets = []domain.ReviewReturnTarget{{AnomalyID: anoms[0].ID, ReasonCategory: "OTHER", Requirement: c.Reason}}
			}
			if len(c.Targets) == 0 {
				return domain.Invalid("退回必须指定 targets")
			}
			bad := false
			for _, it := range c.Checklist {
				if !it.Passed {
					bad = true
				}
			}
			if !bad {
				return domain.Invalid("退回至少需要一项不通过")
			}
			for _, t := range c.Targets {
				a, ok := byID[t.AnomalyID]
				if !ok || strings.TrimSpace(t.ReasonCategory) == "" || strings.TrimSpace(t.Requirement) == "" {
					return domain.Invalid("退回目标无效")
				}
				a.Status = "NEEDS_REVISION"
				a.UpdatedAt = s.now().UTC()
				if e = tx.UpdateAnomaly(ctx, a); e != nil {
					return e
				}
			}
		}
		basis := make([]domain.ReviewBasis, 0, len(anoms))
		for _, a := range anoms {
			basis = append(basis, domain.ReviewBasisFor(packages, a, allEvidence))
		}
		snap := domain.ReviewSnapshot{ID: newID(), TestRunID: r.ID, Reviewer: c.Actor, Outcome: outcome, Notes: c.Notes, ReturnedReason: c.Reason, LockedAt: s.now().UTC(), Checklist: c.Checklist, Targets: c.Targets, Basis: basis, ComparisonReviewID: c.ComparisonReviewID}
		snap.DifferenceDigest = domain.Digest([]any{c.ComparisonReviewID, basis, c.Checklist, c.Targets})
		if outcome == "APPROVED" {
			currentEvidence := []domain.DispositionEvidence{}
			for _, a := range anoms {
				es, er := tx.GetEvidence(ctx, a.ID)
				if er != nil {
					return er
				}
				for _, ev := range es {
					if ev.ReplacedBy == "" {
						currentEvidence = append(currentEvidence, ev)
					}
				}
			}
			snap.SnapshotHash = domain.Digest([]any{packages, anoms, currentEvidence, c.Checklist, snap.DifferenceDigest})
		}
		if e = tx.InsertReview(ctx, snap); e != nil {
			return e
		}
		from := r.Status
		if outcome == "APPROVED" {
			r.Status = domain.StatusReviewed
		} else {
			r.Status = domain.StatusTriaged
		}
		r.Revision++
		if e = tx.UpdateRun(ctx, r, c.ExpectedRevision); e != nil {
			return e
		}
		out = Result{Run: r, Message: "独立复核已" + map[string]string{"APPROVED": "通过", "RETURNED": "退回"}[outcome]}
		b, _ := json.Marshal(out)
		if e = tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: c.RequestID, Fingerprint: fp, StatusCode: 200, Response: b}); e != nil {
			return e
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: c.RunID, RequestID: c.RequestID, Actor: c.Actor, Action: "REVIEW_" + outcome, FromStatus: from, ToStatus: r.Status, BeforeRevision: c.ExpectedRevision, AfterRevision: r.Revision, PayloadDigest: domain.Digest(snap), OccurredAt: s.now()})
	})
	return out, err
}

func (s *Service) Decide(ctx context.Context, c DecisionCommand) (Result, error) {
	if e := s.check(c.RequestID, c.Actor); e != nil {
		return Result{}, e
	}
	reviews, _ := s.repo.GetReviews(ctx, c.RunID)
	fp := domain.Digest(c)
	var out Result
	if ok, e := s.replay(ctx, c.RequestID, fp, &out); ok || e != nil {
		return out, e
	}
	ready, e := s.DecisionReadiness(ctx, c.RunID)
	if e != nil {
		return Result{}, e
	}
	if ready["ready"] != true {
		return Result{}, domain.Wrap(domain.ErrConflict, "裁定尚未就绪", map[string]any{"blocking_reasons": ready["blocking_reasons"]})
	}
	err := s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		r, e := tx.GetRun(ctx, c.RunID)
		if e != nil {
			return e
		}
		if r.Status != domain.StatusReviewed {
			return domain.Transition("必须先完成独立复核")
		}
		if e = domain.ValidateVerdict(c.Verdict, c.Objectives, c.Actor); e != nil {
			return e
		}
		reviewer := ""
		notes := ""
		if n := len(reviews); n > 0 {
			reviewer = reviews[n-1].Reviewer
			notes = reviews[n-1].Notes
		}
		if reviewer == c.Actor {
			return domain.Forbidden("签发人不得与最新独立复核员相同")
		}
		uniq := map[string]bool{}
		normalized := []string{}
		for _, o := range c.Objectives {
			o = strings.TrimSpace(o)
			if o != "" && !uniq[o] {
				uniq[o] = true
				normalized = append(normalized, o)
			}
		}
		if len(normalized) != 1 || normalized[0] != strings.TrimSpace(r.Objective) {
			return domain.Invalid("适用试验目标必须与冻结基线匹配")
		}
		c.Objectives = normalized
		anoms, er := tx.GetAnomalies(ctx, c.RunID)
		if er != nil {
			return er
		}
		needLimit := false
		for _, a := range anoms {
			if (a.Severity == "MAJOR" || a.Severity == "BLOCKING") && strings.Contains(strings.ToUpper(a.Disposition), "ACCEPT") {
				needLimit = true
			}
		}
		if c.Verdict == "VALID" && needLimit && len(c.Limitations) == 0 {
			return domain.Invalid("接受重大或阻断异常时必须提供限制条件")
		}
		if c.Verdict == "INVALID" && len(c.Limitations) == 0 {
			return domain.Invalid("INVALID 裁定必须提供失效理由")
		}
		d := domain.ValidityDecision{ID: newID(), TestRunID: r.ID, Reviewer: reviewer, ReviewOutcome: "APPROVED", ReviewNotes: notes, Verdict: c.Verdict, ApplicableObjectives: c.Objectives, Limitations: c.Limitations, SignedBy: c.Actor, SignedAt: s.now().UTC()}
		if e = tx.InsertDecision(ctx, d); e != nil {
			return e
		}
		r.Status = domain.StatusDecided
		r.Revision++
		if e = tx.UpdateRun(ctx, r, c.ExpectedRevision); e != nil {
			return e
		}
		out = Result{Run: r, Message: "有效性裁定已签发"}
		b, _ := json.Marshal(out)
		if e = tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: c.RequestID, Fingerprint: fp, StatusCode: 200, Response: b}); e != nil {
			return e
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: c.RunID, RequestID: c.RequestID, Actor: c.Actor, Action: "SIGN_DECISION", FromStatus: domain.StatusReviewed, ToStatus: r.Status, BeforeRevision: c.ExpectedRevision, AfterRevision: r.Revision, PayloadDigest: domain.Digest(d), OccurredAt: s.now()})
	})
	return out, err
}

func (s *Service) Archive(ctx context.Context, id, actor, request string, expected int64) (Archive, error) {
	if e := s.check(request, actor); e != nil {
		return Archive{}, e
	}
	fp := domain.Digest([]any{"archive", id, expected})
	var replayed Archive
	if ok, e := s.replay(ctx, request, fp, &replayed); ok || e != nil {
		return replayed, e
	}
	d, e := s.repo.GetDecision(ctx, id)
	if e != nil {
		return Archive{}, e
	}
	pk, _ := s.repo.GetPackages(ctx, id)
	as, _ := s.repo.GetAnomalies(ctx, id)
	ev, _ := s.repo.GetEvidence(ctx, id)
	rv, _ := s.repo.GetReviews(ctx, id)
	au, _ := s.repo.GetAudit(ctx, id)
	var out Archive
	err := s.repo.WithinTx(ctx, func(tx domain.Transaction) error {
		r, e := tx.GetRun(ctx, id)
		if e != nil {
			return e
		}
		if r.Status != domain.StatusDecided {
			return domain.Transition("只有已裁定任务可以封存")
		}
		out = Archive{TestRun: r, DataPackages: pk, Anomalies: as, Evidence: ev, Decision: d, Reviews: rv, Audit: au}
		sort.Slice(out.Audit, func(i, j int) bool { return out.Audit[i].Sequence < out.Audit[j].Sequence })
		out.ArchiveHash = domain.ArchiveDigest(out)
		if e = tx.UpdateDecisionArchive(ctx, d.ID, out.ArchiveHash); e != nil {
			return e
		}
		r.Status = domain.StatusArchived
		r.Revision++
		if e = tx.UpdateRun(ctx, r, expected); e != nil {
			return e
		}
		out.TestRun = r
		b, _ := json.Marshal(out)
		if e = tx.SaveIdempotency(ctx, domain.IdempotencyRecord{RequestID: request, Fingerprint: fp, StatusCode: 200, Response: b}); e != nil {
			return e
		}
		return tx.AppendAudit(ctx, domain.AuditEvent{TestRunID: id, RequestID: request, Actor: actor, Action: "ARCHIVE", FromStatus: domain.StatusDecided, ToStatus: r.Status, BeforeRevision: expected, AfterRevision: r.Revision, PayloadDigest: out.ArchiveHash, OccurredAt: s.now()})
	})
	return out, err
}

func (s *Service) Detail(ctx context.Context, id string) (Archive, error) {
	r, e := s.repo.GetRun(ctx, id)
	if e != nil {
		return Archive{}, e
	}
	out := Archive{TestRun: r}
	out.DataPackages, _ = s.repo.GetPackages(ctx, id)
	out.Anomalies, _ = s.repo.GetAnomalies(ctx, id)
	out.Evidence, _ = s.repo.GetEvidence(ctx, id)
	out.Reviews, _ = s.repo.GetReviews(ctx, id)
	out.Decision, _ = s.repo.GetDecision(ctx, id)
	out.Audit, _ = s.repo.GetAudit(ctx, id)
	out.ArchiveHash = out.Decision.ArchiveHash
	return out, nil
}
func (s *Service) List(ctx context.Context) ([]domain.TestRun, error) { return s.repo.ListRuns(ctx) }
func (s *Service) Timeline(ctx context.Context, id string) ([]domain.AuditEvent, error) {
	return s.repo.GetAudit(ctx, id)
}
