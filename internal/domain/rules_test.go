package domain

import (
	"testing"
	"time"
)

func TestStatusTransitions(t *testing.T) {
	ok := map[[2]Status]bool{{StatusDraft, StatusBaselined}: true, {StatusBaselined, StatusDataChecked}: true, {StatusReviewPending, StatusReviewed}: true, {StatusReviewPending, StatusTriaged}: true, {StatusDecided, StatusArchived}: true}
	for p := range ok {
		if !CanTransition(p[0], p[1]) {
			t.Fatalf("应允许 %s -> %s", p[0], p[1])
		}
	}
	if CanTransition(StatusDraft, StatusArchived) {
		t.Fatal("不应跨状态封存")
	}
}
func TestBaselineHashStable(t *testing.T) {
	a := BaselineHash("R", "E", "O", []string{"B", "A"}, "s", "e")
	b := BaselineHash("R", "E", "O", []string{"A", "B"}, "s", "e")
	if a != b || a == "" {
		t.Fatal("基线摘要应稳定")
	}
}
func TestEvaluateGates(t *testing.T) {
	now := time.Now()
	r := TestRun{ExpectedChannels: []string{"N1", "EGT"}, ScheduledStart: now, ScheduledEnd: now.Add(time.Hour)}
	p := DataPackage{CaptureStart: now, CaptureEnd: now.Add(time.Minute), ClockDriftMS: 100, ChannelSummaries: []ChannelSummary{{Name: "N1"}}}
	g, a := EvaluateGates(r, p)
	if len(g) < 4 || len(a) != 2 {
		t.Fatalf("门禁=%d 异常=%d", len(g), len(a))
	}
}
func TestReviewerSeparation(t *testing.T) {
	if ValidateReview("same", "same") == nil {
		t.Fatal("必须阻止自我复核")
	}
	if ValidateReview("reviewer", "engineer") != nil {
		t.Fatal("独立身份应允许复核")
	}
}
