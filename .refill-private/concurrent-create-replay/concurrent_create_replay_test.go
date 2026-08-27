package concurrent_create_replay_test

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"sync"
	"testing"
	"time"
)

type replayBarrierRepository struct {
	domain.Repository

	mu           sync.Mutex
	started      int
	finished     int
	bothStarted  chan struct{}
	bothFinished chan struct{}
}

func newReplayBarrierRepository(repo domain.Repository) *replayBarrierRepository {
	return &replayBarrierRepository{
		Repository:   repo,
		bothStarted:  make(chan struct{}),
		bothFinished: make(chan struct{}),
	}
}

func (r *replayBarrierRepository) WithinTx(ctx context.Context, fn func(domain.Transaction) error) error {
	r.mu.Lock()
	r.started++
	call := r.started
	if r.started == 2 {
		close(r.bothStarted)
	}
	r.mu.Unlock()

	if call > 2 {
		return r.Repository.WithinTx(ctx, fn)
	}
	<-r.bothStarted
	err := r.Repository.WithinTx(ctx, fn)

	r.mu.Lock()
	r.finished++
	if r.finished == 2 {
		close(r.bothFinished)
	}
	r.mu.Unlock()
	<-r.bothFinished
	return err
}

func TestConcurrentCreateMustReplaySingleResult(t *testing.T) {
	base, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	repo := newReplayBarrierRepository(base)
	service := application.New(repo)
	start := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	command := application.CreateRunCommand{
		RequestID:        "concurrent-create-request",
		Actor:            "engineer-a",
		RigID:            "RIG-07",
		EngineRef:        "ENGINE-42",
		Objective:        "并发幂等建档验证",
		ScheduledStart:   start,
		ScheduledEnd:     start.Add(time.Hour),
		ExpectedChannels: []string{"N1", "EGT"},
	}

	results := make(chan application.Result, 2)
	errors := make(chan error, 2)
	launch := make(chan struct{})
	var callers sync.WaitGroup
	callers.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer callers.Done()
			<-launch
			result, callErr := service.CreateRun(context.Background(), command)
			results <- result
			errors <- callErr
		}()
	}
	close(launch)
	callers.Wait()
	close(results)
	close(errors)

	for callErr := range errors {
		if callErr != nil {
			t.Fatalf("并发重复请求应成功重放，实际返回错误: %v", callErr)
		}
	}
	got := make([]application.Result, 0, 2)
	for result := range results {
		got = append(got, result)
	}
	if got[0].Run.ID != got[1].Run.ID {
		t.Fatalf("并发重复请求返回了两个任务: %s 和 %s", got[0].Run.ID, got[1].Run.ID)
	}
	runs, err := repo.ListRuns(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("并发重复请求持久化了 %d 个任务，期望 1 个", len(runs))
	}
}
