package mutable_read_alias_test

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"testing"
	"time"
)

func TestReadModelsMustNotExposeMutableStoreState(t *testing.T) {
	ctx := context.Background()
	repo, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	created, err := application.New(repo).CreateRun(ctx, application.CreateRunCommand{
		RequestID: "alias-create", Actor: "creator", RigID: "R1", EngineRef: "E1", Objective: "验证",
		ScheduledStart: now, ScheduledEnd: now.Add(time.Hour), ExpectedChannels: []string{"N1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	runID := created.Run.ID
	err = repo.WithinTx(ctx, func(tx domain.Transaction) error {
		a := domain.Anomaly{ID: "a1", TestRunID: runID, Kind: "drift", Severity: "MAJOR", AffectedChannels: []string{"N1"}, Status: "CLOSED"}
		if err := tx.InsertAnomalies(ctx, []domain.Anomaly{a}); err != nil {
			return err
		}
		if err := tx.InsertEvidence(ctx, domain.DispositionEvidence{ID: "e1", AnomalyID: a.ID, EvidenceRefs: []string{"record-1"}}); err != nil {
			return err
		}
		if err := tx.InsertReview(ctx, domain.ReviewSnapshot{ID: "v1", TestRunID: runID, Checklist: []domain.ReviewChecklistItem{{AnomalyID: a.ID, Reason: "original"}}, Targets: []domain.ReviewReturnTarget{{AnomalyID: a.ID, Requirement: "original"}}}); err != nil {
			return err
		}
		return tx.InsertDecision(ctx, domain.ValidityDecision{ID: "d1", TestRunID: runID, ApplicableObjectives: []string{"验证"}, Limitations: []string{"original"}})
	})
	if err != nil {
		t.Fatal(err)
	}

	run, _ := repo.GetRun(ctx, runID)
	anomalies, _ := repo.GetAnomalies(ctx, runID)
	evidence, _ := repo.GetEvidence(ctx, runID)
	reviews, _ := repo.GetReviews(ctx, runID)
	decision, _ := repo.GetDecision(ctx, runID)
	run.ExpectedChannels[0] = "CORRUPTED"
	anomalies[0].AffectedChannels[0] = "CORRUPTED"
	evidence[0].EvidenceRefs[0] = "CORRUPTED"
	reviews[0].Checklist[0].Reason = "CORRUPTED"
	reviews[0].Targets[0].Requirement = "CORRUPTED"
	decision.ApplicableObjectives[0] = "CORRUPTED"
	decision.Limitations[0] = "CORRUPTED"

	run, _ = repo.GetRun(ctx, runID)
	anomalies, _ = repo.GetAnomalies(ctx, runID)
	evidence, _ = repo.GetEvidence(ctx, runID)
	reviews, _ = repo.GetReviews(ctx, runID)
	decision, _ = repo.GetDecision(ctx, runID)
	if run.ExpectedChannels[0] != "N1" || anomalies[0].AffectedChannels[0] != "N1" || evidence[0].EvidenceRefs[0] != "record-1" || reviews[0].Checklist[0].Reason != "original" || reviews[0].Targets[0].Requirement != "original" || decision.ApplicableObjectives[0] != "验证" || decision.Limitations[0] != "original" {
		t.Fatalf("mutating read models changed stored aggregate without a transaction or revision")
	}
}
