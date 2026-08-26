package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"testing"
	"time"
)

func TestRevisePrecheckAndCheckedFreeze(t *testing.T) {
	repo, _ := store.Open(":memory:")
	s := New(repo)
	now := time.Now().UTC().Truncate(time.Second)
	created, err := s.CreateRun(context.Background(), CreateRunCommand{RequestID: "create", Actor: "engineer", RigID: "R1", EngineRef: "E1", Objective: "目标", ScheduledStart: now, ScheduledEnd: now.Add(time.Hour), ExpectedChannels: []string{"N1"}})
	if err != nil {
		t.Fatal(err)
	}
	revised, err := s.ReviseBaseline(context.Background(), ReviseBaselineCommand{RequestID: "revise", Actor: "engineer", RunID: created.Run.ID, RigID: "R2", EngineRef: "E1", Objective: "目标", ScheduledStart: now, ScheduledEnd: now.Add(2 * time.Hour), ExpectedChannels: []string{" EGT ", "N1"}, ExpectedRevision: 1})
	if err != nil || revised.Run.Revision != 2 {
		t.Fatal(err)
	}
	p, err := s.PrecheckBaseline(context.Background(), created.Run.ID)
	if err != nil || p.CandidateHash == "" || len(p.Differences) == 0 || p.NormalizedChannels[0] != "EGT" {
		t.Fatalf("预检无效: %#v %v", p, err)
	}
	if _, err = s.FreezeBaselineChecked(context.Background(), created.Run.ID, "engineer", "stale", 1, p.CandidateHash); !domain.IsCode(err, domain.ErrConflict) {
		t.Fatalf("旧版本应冲突: %v", err)
	}
	frozen, err := s.FreezeBaselineChecked(context.Background(), created.Run.ID, "engineer", "freeze", 2, p.CandidateHash)
	if err != nil || frozen.Run.Status != domain.StatusBaselined {
		t.Fatal(err)
	}
	replay, err := s.FreezeBaselineChecked(context.Background(), created.Run.ID, "engineer", "freeze", 2, p.CandidateHash)
	if err != nil || replay.Run.BaselineHash != frozen.Run.BaselineHash {
		t.Fatal("冻结重放失败", err)
	}
	events, _ := repo.GetAudit(context.Background(), created.Run.ID)
	count := 0
	for _, e := range events {
		if e.Action == "FREEZE_BASELINE" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("冻结事件=%d", count)
	}
}

func TestInvalidManifestIsAtomic(t *testing.T) {
	repo, _ := store.Open(":memory:")
	s := New(repo)
	now := time.Now().UTC()
	r, _ := s.CreateRun(context.Background(), CreateRunCommand{RequestID: "c", Actor: "e", RigID: "R", EngineRef: "E", Objective: "O", ScheduledStart: now, ScheduledEnd: now.Add(time.Hour), ExpectedChannels: []string{"N1"}})
	f, _ := s.FreezeBaseline(context.Background(), r.Run.ID, "e", "f", 1)
	_, err := s.RegisterPackage(context.Background(), RegisterPackageCommand{RequestID: "p", Actor: "e", RunID: r.Run.ID, ExpectedRevision: f.Run.Revision, Package: domain.DataPackage{Files: []domain.FileEntry{{Name: "a", Digest: "abcd", Bytes: 0}}, CaptureStart: now, CaptureEnd: now.Add(time.Minute)}})
	if !domain.IsCode(err, domain.ErrInvalidInput) {
		t.Fatalf("应拒绝零字节文件: %v", err)
	}
	cur, _ := repo.GetRun(context.Background(), r.Run.ID)
	pk, _ := repo.GetPackages(context.Background(), r.Run.ID)
	if cur.Status != domain.StatusBaselined || len(pk) != 0 {
		t.Fatal("无效登记留下了状态或数据包")
	}
}
