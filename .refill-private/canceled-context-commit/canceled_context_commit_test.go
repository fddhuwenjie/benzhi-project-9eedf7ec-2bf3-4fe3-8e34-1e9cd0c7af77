package canceled_context_commit_test

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"errors"
	"testing"
	"time"
)

func TestCanceledContextDoesNotCommit(t *testing.T) {
	repo, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	svc := application.New(repo)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	_, err = svc.CreateRun(ctx, application.CreateRunCommand{
		RequestID:        "canceled-create",
		Actor:            "engineer",
		RigID:            "R1",
		EngineRef:        "E1",
		Objective:        "验证",
		ScheduledStart:   now,
		ScheduledEnd:     now.Add(time.Hour),
		ExpectedChannels: []string{"N1"},
	})
	runs, listErr := repo.ListRuns(context.Background())
	if !errors.Is(err, context.Canceled) || listErr != nil || len(runs) != 0 {
		t.Fatalf("已取消的 context 仍提交事务: err=%v listErr=%v runs=%d", err, listErr, len(runs))
	}
}
