package concurrent_detail_scratch_test

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"testing"
	"time"
)

type coordinatedRepository struct {
	domain.Repository
	firstID         string
	secondID        string
	firstAtPackage  chan struct{}
	secondAtPackage chan struct{}
	releaseSecond   chan struct{}
}

func (r *coordinatedRepository) GetPackages(ctx context.Context, id string) ([]domain.DataPackage, error) {
	switch id {
	case r.firstID:
		close(r.firstAtPackage)
		<-r.secondAtPackage
	case r.secondID:
		close(r.secondAtPackage)
		<-r.releaseSecond
	}
	return r.Repository.GetPackages(ctx, id)
}

type detailResult struct {
	archive domain.Archive
	err     error
}

func TestConcurrentDetailsMustRemainIsolated(t *testing.T) {
	base, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	seed := application.New(base)
	now := time.Date(2026, 8, 26, 9, 0, 0, 0, time.UTC)
	first, err := seed.CreateRun(context.Background(), application.CreateRunCommand{
		RequestID: "detail-first-create", Actor: "engineer-a", RigID: "RIG-A", EngineRef: "ENGINE-A",
		Objective: "目标-A", ScheduledStart: now, ScheduledEnd: now.Add(time.Hour), ExpectedChannels: []string{"N1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := seed.CreateRun(context.Background(), application.CreateRunCommand{
		RequestID: "detail-second-create", Actor: "engineer-b", RigID: "RIG-B", EngineRef: "ENGINE-B",
		Objective: "目标-B", ScheduledStart: now, ScheduledEnd: now.Add(time.Hour), ExpectedChannels: []string{"EGT"},
	})
	if err != nil {
		t.Fatal(err)
	}

	gate := &coordinatedRepository{
		Repository: base, firstID: first.Run.ID, secondID: second.Run.ID,
		firstAtPackage: make(chan struct{}), secondAtPackage: make(chan struct{}), releaseSecond: make(chan struct{}),
	}
	service := application.New(gate)
	firstResult := make(chan detailResult, 1)
	secondResult := make(chan detailResult, 1)
	go func() {
		a, e := service.Detail(context.Background(), first.Run.ID)
		firstResult <- detailResult{archive: a, err: e}
	}()
	<-gate.firstAtPackage
	go func() {
		a, e := service.Detail(context.Background(), second.Run.ID)
		secondResult <- detailResult{archive: a, err: e}
	}()

	gotFirst := <-firstResult
	close(gate.releaseSecond)
	gotSecond := <-secondResult
	if gotFirst.err != nil || gotSecond.err != nil {
		t.Fatalf("并发详情读取失败: first=%v second=%v", gotFirst.err, gotSecond.err)
	}
	if gotFirst.archive.TestRun.ID != first.Run.ID {
		t.Fatalf("请求 A 的详情被请求 B 污染: got=%s want=%s", gotFirst.archive.TestRun.ID, first.Run.ID)
	}
	if gotSecond.archive.TestRun.ID != second.Run.ID {
		t.Fatalf("请求 B 的详情被请求 A 污染: got=%s want=%s", gotSecond.archive.TestRun.ID, second.Run.ID)
	}
}
