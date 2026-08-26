package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"sort"
)

type QueueItem struct {
	ID            string             `json:"id"`
	EngineRef     string             `json:"engine_ref"`
	Objective     string             `json:"objective"`
	Status        domain.Status      `json:"status"`
	Revision      int64              `json:"revision"`
	OpenAnomalies int                `json:"open_anomalies"`
	LastEvent     *domain.AuditEvent `json:"last_event,omitempty"`
}

func (s *Service) Queue(ctx context.Context) ([]QueueItem, error) {
	runs, e := s.repo.ListRuns(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]QueueItem, 0, len(runs))
	for _, r := range runs {
		as, _ := s.repo.GetAnomalies(ctx, r.ID)
		events, _ := s.repo.GetAudit(ctx, r.ID)
		open := 0
		for _, a := range as {
			if a.Status != "CLOSED" {
				open++
			}
		}
		q := QueueItem{ID: r.ID, EngineRef: r.EngineRef, Objective: r.Objective, Status: r.Status, Revision: r.Revision, OpenAnomalies: open}
		if len(events) > 0 {
			sort.Slice(events, func(i, j int) bool { return events[i].Sequence < events[j].Sequence })
			q.LastEvent = &events[len(events)-1]
		}
		out = append(out, q)
	}
	return out, nil
}
func (s *Service) ValidateArchive(ctx context.Context, id string) error {
	a, e := s.Detail(ctx, id)
	if e != nil {
		return e
	}
	if a.TestRun.Status != domain.StatusArchived {
		return domain.Transition("档案尚未封存")
	}
	if a.ArchiveHash == "" {
		return domain.Integrity("档案缺少摘要哈希")
	}
	return nil
}
