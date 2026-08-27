package archive_snapshot_context_test

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"errors"
	"fmt"
	"testing"
)

var errCancellationWasDetached = errors.New("archive snapshot read received a detached context")

type cancellationRepo struct {
	domain.Repository
	tx     *replayTx
	cancel context.CancelFunc
}

func (r *cancellationRepo) WithinTx(ctx context.Context, fn func(domain.Transaction) error) error {
	err := fn(r.tx)
	r.cancel()
	return err
}

func (r *cancellationRepo) GetDecision(ctx context.Context, _ string) (domain.ValidityDecision, error) {
	if err := ctx.Err(); err != nil {
		return domain.ValidityDecision{}, err
	}
	return domain.ValidityDecision{}, errCancellationWasDetached
}

type replayTx struct {
	domain.Transaction
}

func (*replayTx) FindIdempotency(context.Context, string) (*domain.IdempotencyRecord, error) {
	return nil, nil
}

func TestArchiveSnapshotMustPropagateCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := &cancellationRepo{tx: &replayTx{}, cancel: cancel}
	service := application.New(repo)

	_, err := service.Archive(ctx, "run-1", "manager", "archive-request-1", 7)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("TestArchiveSnapshotMustPropagateCancellation: want context.Canceled, got %v", err)
	}
	if errors.Is(err, errCancellationWasDetached) {
		t.Fatal(fmt.Sprintf("TestArchiveSnapshotMustPropagateCancellation: detached context reached Repository: %v", err))
	}
}
