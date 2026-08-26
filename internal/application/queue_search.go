package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"time"
)

type QueueFilter struct {
	Status                     domain.Status
	RigID, EngineRef, Severity string
	CreatedFrom, CreatedTo     time.Time
	HasBlocking                *bool
	Cursor                     string
	Limit                      int
}
type QueueRecord struct {
	ID              string        `json:"id"`
	RigID           string        `json:"rig_id"`
	EngineRef       string        `json:"engine_ref"`
	Objective       string        `json:"objective"`
	Status          domain.Status `json:"status"`
	Revision        int64         `json:"revision"`
	CreatedAt       time.Time     `json:"created_at"`
	LatestActionAt  time.Time     `json:"latest_action_at"`
	PendingTriage   int           `json:"pending_triage"`
	PendingEvidence int           `json:"pending_evidence"`
	OpenBlocking    int           `json:"open_blocking"`
	TodoRole        string        `json:"todo_role"`
}
type QueuePage struct {
	Runs           []QueueRecord         `json:"runs"`
	StatusCounts   map[domain.Status]int `json:"status_counts"`
	SeverityCounts map[string]int        `json:"severity_counts"`
	NextCursor     string                `json:"next_cursor,omitempty"`
}

func (s *Service) SearchQueue(ctx context.Context, f QueueFilter) (QueuePage, error) {
	if f.Status != "" && !domain.ValidStatus(f.Status) {
		return QueuePage{}, domain.Invalid("status 无效")
	}
	if f.Severity != "" && f.Severity != "MINOR" && f.Severity != "MAJOR" && f.Severity != "BLOCKING" {
		return QueuePage{}, domain.Invalid("severity 无效")
	}
	if !f.CreatedFrom.IsZero() && !f.CreatedTo.IsZero() && f.CreatedTo.Before(f.CreatedFrom) {
		return QueuePage{}, domain.Invalid("created_to 不得早于 created_from")
	}
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Limit > 100 {
		f.Limit = 100
	}
	runs, e := s.repo.ListRuns(ctx)
	if e != nil {
		return QueuePage{}, e
	}
	all := []QueueRecord{}
	st := map[domain.Status]int{}
	sv := map[string]int{}
	for _, r := range runs {
		if f.Status != "" && r.Status != f.Status || f.RigID != "" && !strings.Contains(strings.ToLower(r.RigID), strings.ToLower(f.RigID)) || f.EngineRef != "" && !strings.Contains(strings.ToLower(r.EngineRef), strings.ToLower(f.EngineRef)) {
			continue
		}
		if !f.CreatedFrom.IsZero() && r.CreatedAt.Before(f.CreatedFrom) || !f.CreatedTo.IsZero() && r.CreatedAt.After(f.CreatedTo) {
			continue
		}
		as, _ := s.repo.GetAnomalies(ctx, r.ID)
		hasSeverity := f.Severity == ""
		q := QueueRecord{ID: r.ID, RigID: r.RigID, EngineRef: r.EngineRef, Objective: r.Objective, Status: r.Status, Revision: r.Revision, CreatedAt: r.CreatedAt}
		matchedSeverities := map[string]bool{}
		for _, a := range as {
			if a.Severity == f.Severity {
				hasSeverity = true
			}
			if a.Status == "OPEN" {
				q.PendingTriage++
			}
			if a.Status == "NEEDS_REVISION" {
				q.PendingEvidence++
			}
			if a.Severity == "BLOCKING" && a.Status != "CLOSED" {
				q.OpenBlocking++
			}
			if a.Severity != "" {
				matchedSeverities[a.Severity] = true
			}
		}
		if !hasSeverity {
			continue
		}
		if f.HasBlocking != nil && (*f.HasBlocking) != (q.OpenBlocking > 0) {
			continue
		}
		for severity := range matchedSeverities {
			sv[severity]++
		}
		events, _ := s.repo.GetAudit(ctx, r.ID)
		if len(events) > 0 {
			sort.Slice(events, func(i, j int) bool { return events[i].Sequence < events[j].Sequence })
			q.LatestActionAt = events[len(events)-1].OccurredAt
		}
		q.TodoRole = todoRole(r.Status, q)
		st[r.Status]++
		all = append(all, q)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt.Equal(all[j].CreatedAt) {
			return all[i].ID < all[j].ID
		}
		return all[i].CreatedAt.After(all[j].CreatedAt)
	})
	start := 0
	if f.Cursor != "" {
		raw, er := base64.RawURLEncoding.DecodeString(f.Cursor)
		if er != nil {
			return QueuePage{}, domain.Invalid("cursor 无效")
		}
		key := string(raw)
		for i, q := range all {
			if fmt.Sprintf("%d|%s", q.CreatedAt.UnixNano(), q.ID) == key {
				start = i + 1
				break
			}
		}
	}
	end := start + f.Limit
	if end > len(all) {
		end = len(all)
	}
	page := QueuePage{Runs: all[start:end], StatusCounts: st, SeverityCounts: sv}
	if end < len(all) && end > 0 {
		q := all[end-1]
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d|%s", q.CreatedAt.UnixNano(), q.ID)))
	}
	return page, nil
}
func todoRole(s domain.Status, q QueueRecord) string {
	switch s {
	case domain.StatusDraft, domain.StatusBaselined:
		return "数据工程师"
	case domain.StatusDataChecked:
		if q.PendingTriage > 0 {
			return "数据工程师"
		}
	case domain.StatusTriaged:
		return "处置工程师"
	case domain.StatusReviewPending:
		return "独立复核员"
	case domain.StatusReviewed:
		return "平台主管"
	case domain.StatusDecided:
		return "档案管理员"
	}
	return ""
}
