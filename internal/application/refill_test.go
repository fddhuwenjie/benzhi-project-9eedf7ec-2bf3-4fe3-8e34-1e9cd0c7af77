package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"testing"
	"time"
)

func baselinedRun(t *testing.T) (*Service, *store.Memory, domain.TestRun, time.Time) {
	t.Helper()
	repo, _ := store.Open(":memory:")
	s := New(repo)
	now := time.Now().UTC().Truncate(time.Second)
	created, err := s.CreateRun(context.Background(), CreateRunCommand{RequestID: "create", Actor: "engineer", RigID: "R1", EngineRef: "E1", Objective: "目标", ScheduledStart: now, ScheduledEnd: now.Add(time.Minute), ExpectedChannels: []string{"N1", "EGT"}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := s.FreezeBaseline(context.Background(), created.Run.ID, "engineer", "freeze", 1)
	if err != nil {
		t.Fatal(err)
	}
	return s, repo, frozen.Run, now
}

func candidate(now time.Time) domain.DataPackage {
	return domain.DataPackage{Files: []domain.FileEntry{{Name: "capture.dat", Digest: "abcd", Bytes: 100}}, ChannelSummaries: []domain.ChannelSummary{{Name: "N1", Samples: 60, SampleRateHz: 1, Min: 1, Max: 2}, {Name: "EGT", Samples: 30, SampleRateHz: 1, Min: 1, Max: 2}}, CaptureStart: now, CaptureEnd: now.Add(time.Minute), ClockDriftMS: 80}
}

func TestPackagePreviewLocksPayloadAndIsAtomic(t *testing.T) {
	s, repo, r, now := baselinedRun(t)
	p := candidate(now)
	pv, err := s.PreviewPackage(context.Background(), r.ID, p)
	if err != nil || len(pv.ExpectedAnomalies) != 2 {
		t.Fatalf("预演异常: %#v %v", pv, err)
	}
	p.Files[0].Digest = "changed"
	_, err = s.RegisterPackage(context.Background(), RegisterPackageCommand{RequestID: "package", Actor: "engineer", RunID: r.ID, ExpectedRevision: r.Revision, CandidatePackageHash: pv.CandidatePackageHash, Package: p})
	if !domain.IsCode(err, domain.ErrConflict) {
		t.Fatalf("应发生候选冲突: %v", err)
	}
	cur, _ := repo.GetRun(context.Background(), r.ID)
	packages, _ := repo.GetPackages(context.Background(), r.ID)
	anomalies, _ := repo.GetAnomalies(context.Background(), r.ID)
	if cur.Status != domain.StatusBaselined || cur.Revision != r.Revision || len(packages) != 0 || len(anomalies) != 0 {
		t.Fatal("候选冲突产生了部分写入")
	}
	p.Files[0].Digest = "abcd"
	out, err := s.RegisterPackage(context.Background(), RegisterPackageCommand{RequestID: "package-ok", Actor: "engineer", RunID: r.ID, ExpectedRevision: r.Revision, CandidatePackageHash: pv.CandidatePackageHash, Package: p})
	if err != nil || out.Run.Status != domain.StatusDataChecked {
		t.Fatal(err)
	}
}

func TestImpactKeepsZeroRowsAndRejectsFilter(t *testing.T) {
	s, repo, r, now := baselinedRun(t)
	p := candidate(now)
	pv, _ := s.PreviewPackage(context.Background(), r.ID, p)
	_, _ = s.RegisterPackage(context.Background(), RegisterPackageCommand{RequestID: "p", Actor: "e", RunID: r.ID, ExpectedRevision: r.Revision, CandidatePackageHash: pv.CandidatePackageHash, Package: p})
	before, _ := repo.GetAudit(context.Background(), r.ID)
	impact, err := s.AnomalyImpact(context.Background(), r.ID, domain.AnomalyImpactFilter{})
	if err != nil || len(impact.Matrix) != 2 || len(impact.Priority) != 2 {
		t.Fatalf("影响查询异常: %#v %v", impact, err)
	}
	_, err = s.AnomalyImpact(context.Background(), r.ID, domain.AnomalyImpactFilter{Severities: []string{"CRITICAL"}})
	if !domain.IsCode(err, domain.ErrInvalidInput) {
		t.Fatalf("应拒绝严重度: %v", err)
	}
	after, _ := repo.GetAudit(context.Background(), r.ID)
	if len(before) != len(after) {
		t.Fatal("只读查询写入了审计")
	}
}

func TestEvidenceBatchValidationRollsBack(t *testing.T) {
	s, repo, r, now := baselinedRun(t)
	p := candidate(now)
	pv, _ := s.PreviewPackage(context.Background(), r.ID, p)
	registered, _ := s.RegisterPackage(context.Background(), RegisterPackageCommand{RequestID: "p", Actor: "e", RunID: r.ID, ExpectedRevision: r.Revision, CandidatePackageHash: pv.CandidatePackageHash, Package: p})
	as, _ := repo.GetAnomalies(context.Background(), r.ID)
	items := []TriageItem{}
	for _, a := range as {
		items = append(items, TriageItem{AnomalyID: a.ID, Kind: a.Kind, Severity: a.Severity, Impact: "影响", Disposition: "待处置", TimeRange: a.TimeRange, Channels: []string{"N1"}})
	}
	triaged, err := s.TriageBatch(context.Background(), r.ID, "e", "t", registered.Run.Revision, items)
	if err != nil {
		t.Fatal(err)
	}
	bad := []EvidenceBatchItem{{AnomalyID: as[0].ID, Evidence: domain.DispositionEvidence{Method: "CORRECT", Rationale: "修正", BeforeDigest: "same", AfterDigest: "same", EvidenceRefs: []string{"x"}}}, {AnomalyID: as[1].ID, Evidence: domain.DispositionEvidence{Method: "EXCLUDE", Rationale: "排除", BeforeDigest: "a", AfterDigest: "b", EvidenceRefs: []string{"y"}, SupersedesEvidenceID: "foreign"}}}
	_, err = s.SubmitEvidenceBatch(context.Background(), EvidenceBatchCommand{RequestID: "bad", Actor: "e", RunID: r.ID, ExpectedRevision: triaged.Run.Revision, Items: bad})
	if !domain.IsCode(err, domain.ErrInvalidInput) {
		t.Fatalf("应整批拒绝: %v", err)
	}
	ev, _ := repo.GetEvidence(context.Background(), r.ID)
	cur, _ := repo.GetRun(context.Background(), r.ID)
	if len(ev) != 0 || cur.Revision != triaged.Run.Revision {
		t.Fatal("无效批次留下写入")
	}
}

func TestReturnedReviewDifferenceAndFocusedApproval(t *testing.T) {
	s, repo, r, now := baselinedRun(t)
	p := candidate(now)
	pv, _ := s.PreviewPackage(context.Background(), r.ID, p)
	registered, _ := s.RegisterPackage(context.Background(), RegisterPackageCommand{RequestID: "p", Actor: "engineer", RunID: r.ID, ExpectedRevision: r.Revision, CandidatePackageHash: pv.CandidatePackageHash, Package: p})
	anomalies, _ := repo.GetAnomalies(context.Background(), r.ID)
	triage := []TriageItem{}
	for _, a := range anomalies {
		triage = append(triage, TriageItem{AnomalyID: a.ID, Kind: a.Kind, Severity: a.Severity, Impact: "影响", Disposition: "待处置", TimeRange: a.TimeRange, Channels: a.AffectedChannels})
	}
	triaged, err := s.TriageBatch(context.Background(), r.ID, "engineer", "triage", registered.Run.Revision, triage)
	if err != nil {
		t.Fatal(err)
	}
	items := []EvidenceBatchItem{}
	for _, a := range anomalies {
		items = append(items, EvidenceBatchItem{AnomalyID: a.ID, Evidence: domain.DispositionEvidence{Method: "EXCLUDE", Rationale: "排除", BeforeDigest: "before", AfterDigest: "after", EvidenceRefs: []string{"record"}}})
	}
	closed, err := s.SubmitEvidenceBatch(context.Background(), EvidenceBatchCommand{RequestID: "evidence", Actor: "engineer", RunID: r.ID, ExpectedRevision: triaged.Run.Revision, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	checklist := []domain.ReviewChecklistItem{}
	for i, a := range anomalies {
		checklist = append(checklist, domain.ReviewChecklistItem{AnomalyID: a.ID, DataPackageConfirmed: true, GateConfirmed: true, ImpactConfirmed: true, EvidenceConfirmed: true, BeforeAfterConfirmed: true, Passed: i != 0, Reason: map[bool]string{true: "", false: "需修订"}[i != 0]})
	}
	returned, err := s.Review(context.Background(), ReviewCommand{RequestID: "return", Actor: "reviewer", RunID: r.ID, Outcome: "RETURNED", Reason: "需修订", ExpectedRevision: closed.Run.Revision, Checklist: checklist, Targets: []domain.ReviewReturnTarget{{AnomalyID: anomalies[0].ID, ReasonCategory: "EVIDENCE", Requirement: "更新摘要"}}})
	if err != nil {
		t.Fatal(err)
	}
	allEvidence, _ := repo.GetEvidence(context.Background(), r.ID)
	old, _ := domain.CurrentEvidence(allEvidence, anomalies[0].ID)
	revised, err := s.SubmitEvidenceBatch(context.Background(), EvidenceBatchCommand{RequestID: "revise", Actor: "engineer", RunID: r.ID, ExpectedRevision: returned.Run.Revision, Items: []EvidenceBatchItem{{AnomalyID: anomalies[0].ID, Evidence: domain.DispositionEvidence{Method: "CORRECT", Rationale: "已修订", BeforeDigest: "old", AfterDigest: "new", EvidenceRefs: []string{"new-record"}, SupersedesEvidenceID: old.ID}}}})
	if err != nil {
		t.Fatal(err)
	}
	diff, err := s.ReviewDifference(context.Background(), r.ID)
	if err != nil || len(diff.ReturnedTargets) != 1 || diff.ReturnedTargets[0].Classification != "STRUCTURALLY_VALID" || len(diff.BlockingAnomalyIDs) != 0 {
		t.Fatalf("差异异常: %#v %v", diff, err)
	}
	for i := range checklist {
		checklist[i].Passed = true
		checklist[i].Reason = ""
	}
	approved, err := s.Review(context.Background(), ReviewCommand{RequestID: "approve", Actor: "reviewer-2", RunID: r.ID, Outcome: "APPROVED", ExpectedRevision: revised.Run.Revision, ComparisonReviewID: diff.ComparisonReviewID, Checklist: checklist})
	if err != nil || approved.Run.Status != domain.StatusReviewed {
		t.Fatalf("再审未通过: %#v %v", approved, err)
	}
}
