package transaction_panic_rollback_test

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"testing"
)

func runPanickingTransaction(repo *store.Memory) (panicked bool) {
	defer func() {
		panicked = recover() != nil
	}()
	_ = repo.WithinTx(context.Background(), func(tx domain.Transaction) error {
		if err := tx.InsertRun(context.Background(), domain.TestRun{ID: "panic-run", Status: domain.StatusDraft, Revision: 1}); err != nil {
			return err
		}
		panic("forced transaction panic")
	})
	return false
}

func TestPanickingTransactionRollsBack(t *testing.T) {
	repo, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if !runPanickingTransaction(repo) {
		t.Fatal("事务 panic 未继续向调用方传播")
	}
	_, err = repo.GetRun(context.Background(), "panic-run")
	if !domain.IsCode(err, domain.ErrNotFound) {
		t.Fatalf("事务 panic 后留下了部分写入: %v", err)
	}
}
