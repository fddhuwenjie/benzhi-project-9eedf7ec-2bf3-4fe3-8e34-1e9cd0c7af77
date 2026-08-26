package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"testing"
	"time"
)

func TestCreateFreezeAndConflict(t *testing.T) {
	repo, _ := store.Open(":memory:")
	s := New(repo)
	now := time.Now().UTC()
	r, e := s.CreateRun(context.Background(), CreateRunCommand{RequestID: "r1", Actor: "engineer", RigID: "R1", EngineRef: "E1", Objective: "验证", ScheduledStart: now, ScheduledEnd: now.Add(time.Hour), ExpectedChannels: []string{"N1"}})
	if e != nil {
		t.Fatal(e)
	}
	if r.Run.Status != domain.StatusDraft {
		t.Fatal(r.Run.Status)
	}
	f, e := s.FreezeBaseline(context.Background(), r.Run.ID, "engineer", "r2", 1)
	if e != nil {
		t.Fatal(e)
	}
	if f.Run.Status != domain.StatusBaselined || f.Run.BaselineHash == "" {
		t.Fatal("基线未冻结")
	}
	if _, e = s.FreezeBaseline(context.Background(), r.Run.ID, "engineer", "r3", 1); e == nil {
		t.Fatal("重复冻结应失败")
	}
}
func TestIdempotentCreate(t *testing.T) {
	repo, _ := store.Open(":memory:")
	s := New(repo)
	now := time.Now()
	c := CreateRunCommand{RequestID: "same", Actor: "a", RigID: "R", EngineRef: "E", Objective: "O", ScheduledStart: now, ScheduledEnd: now.Add(time.Minute), ExpectedChannels: []string{"N"}}
	a, e := s.CreateRun(context.Background(), c)
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.CreateRun(context.Background(), c)
	if e != nil || a.Run.ID != b.Run.ID {
		t.Fatal("幂等重放失败")
	}
}
