package readiness_error_swallowed_test

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"errors"
	"testing"
)

type failingReviewsRepository struct {
	domain.Repository
	err error
}

func (r failingReviewsRepository) GetReviews(context.Context, string) ([]domain.ReviewSnapshot, error) {
	return nil, r.err
}

func TestDecisionReadinessPropagatesRepositoryFailure(t *testing.T) {
	base, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	run := domain.TestRun{ID: "run-reviewed", Status: domain.StatusReviewed, Revision: 9}
	review := domain.ReviewSnapshot{ID: "review-approved", TestRunID: run.ID, Outcome: "APPROVED"}
	review.SnapshotHash = domain.Digest([]any{[]domain.DataPackage{}, []domain.Anomaly{}, []domain.DispositionEvidence{}, review.Checklist, review.DifferenceDigest})
	if err = base.WithinTx(context.Background(), func(tx domain.Transaction) error {
		if insertErr := tx.InsertRun(context.Background(), run); insertErr != nil {
			return insertErr
		}
		return tx.InsertReview(context.Background(), review)
	}); err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("reviews storage unavailable")
	svc := application.New(failingReviewsRepository{Repository: base, err: sentinel})
	_, err = svc.DecisionReadiness(context.Background(), run.ID)
	if !errors.Is(err, sentinel) {
		t.Fatalf("就绪检查吞掉 reviews 存储错误并伪装成业务结果: err=%v", err)
	}
}
