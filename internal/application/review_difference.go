package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"sort"
)

func (s *Service) ReviewDifference(ctx context.Context, runID string) (domain.ReviewDifference, error) {
	r, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return domain.ReviewDifference{}, err
	}
	reviews, err := s.repo.GetReviews(ctx, runID)
	if err != nil {
		return domain.ReviewDifference{}, err
	}
	anomalies, err := s.repo.GetAnomalies(ctx, runID)
	if err != nil {
		return domain.ReviewDifference{}, err
	}
	packages, err := s.repo.GetPackages(ctx, runID)
	if err != nil {
		return domain.ReviewDifference{}, err
	}
	evidence, err := s.repo.GetEvidence(ctx, runID)
	if err != nil {
		return domain.ReviewDifference{}, err
	}
	return s.reviewDifferenceData(r, reviews, anomalies, packages, evidence)
}

func (s *Service) reviewDifferenceData(r domain.TestRun, reviews []domain.ReviewSnapshot, anomalies []domain.Anomaly, packages []domain.DataPackage, evidence []domain.DispositionEvidence) (domain.ReviewDifference, error) {
	var returned *domain.ReviewSnapshot
	for i := len(reviews) - 1; i >= 0; i-- {
		if reviews[i].Outcome == "RETURNED" {
			x := reviews[i]
			returned = &x
			break
		}
	}
	out := domain.ReviewDifference{Revision: r.Revision, ReturnedTargets: []domain.ReviewDifferenceItem{}, DriftedUntargeted: []domain.ReviewDifferenceItem{}, BlockingAnomalyIDs: []string{}}
	if returned == nil {
		return out, nil
	}
	out.ComparisonReviewID = returned.ID
	basis := map[string]domain.ReviewBasis{}
	for _, b := range returned.Basis {
		basis[b.AnomalyID] = b
	}
	targets := map[string]domain.ReviewReturnTarget{}
	for _, t := range returned.Targets {
		targets[t.AnomalyID] = t
	}
	byID := map[string]domain.DispositionEvidence{}
	for _, e := range evidence {
		byID[e.ID] = e
	}
	for _, a := range anomalies {
		old := basis[a.ID]
		current := domain.ReviewBasisFor(packages, a, evidence)
		target, isTarget := targets[a.ID]
		item := domain.ReviewDifferenceItem{AnomalyID: a.ID, CurrentStatus: a.Status, Classification: "UNCHANGED", SupersessionChain: []string{}}
		if ev, ok := domain.CurrentEvidence(evidence, a.ID); ok {
			item.CurrentEvidence = &ev
			cur := ev
			for cur.ID != "" {
				item.SupersessionChain = append([]string{cur.ID}, item.SupersessionChain...)
				if cur.SupersedesEvidenceID == "" {
					break
				}
				cur = byID[cur.SupersedesEvidenceID]
			}
		}
		if isTarget {
			item.ReasonCategory = target.ReasonCategory
			item.Requirement = target.Requirement
			item.ReturnedEvidenceID = old.EvidenceID
			item.ReturnedEvidenceDigest = old.EvidenceDigest
			if item.CurrentEvidence == nil || item.CurrentEvidence.ID == old.EvidenceID {
				item.Classification = "UNREVISED"
				item.BlockingReasons = append(item.BlockingReasons, "退回目标尚未提交新证据")
			} else {
				item.Classification = "REVISED_PENDING_VALIDATION"
				if item.CurrentEvidence.SupersedesEvidenceID != old.EvidenceID {
					item.BlockingReasons = append(item.BlockingReasons, "新证据未精确引用退回时有效版本")
				}
				if item.CurrentEvidence.BeforeDigest == item.CurrentEvidence.AfterDigest || item.CurrentEvidence.AfterDigest == "" {
					item.BlockingReasons = append(item.BlockingReasons, "证据摘要未形成有效变化")
				}
				if a.Status != "CLOSED" {
					item.BlockingReasons = append(item.BlockingReasons, "异常尚未 CLOSED")
				}
				if len(item.BlockingReasons) == 0 {
					item.Classification = "STRUCTURALLY_VALID"
				}
			}
			out.ReturnedTargets = append(out.ReturnedTargets, item)
		} else if old.CombinedDigest != "" && old.CombinedDigest != current.CombinedDigest {
			item.Classification = "DRIFTED"
			item.Drifted = true
			item.BlockingReasons = []string{"未退回异常的当前依据相对上轮快照发生漂移"}
			out.DriftedUntargeted = append(out.DriftedUntargeted, item)
		}
		if len(item.BlockingReasons) > 0 {
			out.BlockingAnomalyIDs = append(out.BlockingAnomalyIDs, a.ID)
		}
	}
	sort.Slice(out.ReturnedTargets, func(i, j int) bool { return out.ReturnedTargets[i].AnomalyID < out.ReturnedTargets[j].AnomalyID })
	sort.Slice(out.DriftedUntargeted, func(i, j int) bool { return out.DriftedUntargeted[i].AnomalyID < out.DriftedUntargeted[j].AnomalyID })
	sort.Strings(out.BlockingAnomalyIDs)
	return out, nil
}
