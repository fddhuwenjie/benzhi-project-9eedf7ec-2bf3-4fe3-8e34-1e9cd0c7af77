package detail_read_error_ignored_test

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"errors"
	"testing"
	"time"
)

type failingReadRepository struct {
	domain.Repository
	operation string
	err       error
}

func (r failingReadRepository) GetPackages(ctx context.Context, id string) ([]domain.DataPackage, error) {
	if r.operation == "packages" {
		return nil, r.err
	}
	return r.Repository.GetPackages(ctx, id)
}
func (r failingReadRepository) GetAnomalies(ctx context.Context, id string) ([]domain.Anomaly, error) {
	if r.operation == "anomalies" {
		return nil, r.err
	}
	return r.Repository.GetAnomalies(ctx, id)
}
func (r failingReadRepository) GetEvidence(ctx context.Context, id string) ([]domain.DispositionEvidence, error) {
	if r.operation == "evidence" {
		return nil, r.err
	}
	return r.Repository.GetEvidence(ctx, id)
}
func (r failingReadRepository) GetReviews(ctx context.Context, id string) ([]domain.ReviewSnapshot, error) {
	if r.operation == "reviews" {
		return nil, r.err
	}
	return r.Repository.GetReviews(ctx, id)
}
func (r failingReadRepository) GetDecision(ctx context.Context, id string) (domain.ValidityDecision, error) {
	if r.operation == "decision" {
		return domain.ValidityDecision{}, r.err
	}
	return r.Repository.GetDecision(ctx, id)
}
func (r failingReadRepository) GetAudit(ctx context.Context, id string) ([]domain.AuditEvent, error) {
	if r.operation == "audit" {
		return nil, r.err
	}
	return r.Repository.GetAudit(ctx, id)
}

func TestDetailMustPropagateDependencyReadErrors(t *testing.T) {
	ctx := context.Background()
	repo, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	created, err := application.New(repo).CreateRun(ctx, application.CreateRunCommand{
		RequestID: "detail-create", Actor: "creator", RigID: "R1", EngineRef: "E1", Objective: "验证",
		ScheduledStart: now, ScheduledEnd: now.Add(time.Hour), ExpectedChannels: []string{"N1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"packages", "anomalies", "evidence", "reviews", "decision", "audit"} {
		sentinel := errors.New("forced " + operation + " read failure")
		_, err := application.New(failingReadRepository{Repository: repo, operation: operation, err: sentinel}).Detail(ctx, created.Run.ID)
		if !errors.Is(err, sentinel) {
			t.Fatalf("Detail returned a partial aggregate after %s failed: %v", operation, err)
		}
	}
}
