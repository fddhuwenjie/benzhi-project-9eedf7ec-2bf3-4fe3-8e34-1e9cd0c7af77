package store

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"encoding/json"
)

func (s *Memory) Snapshot(ctx context.Context, id string) ([]byte, error) {
	r, e := s.GetRun(ctx, id)
	if e != nil {
		return nil, e
	}
	a, _ := s.GetAnomalies(ctx, id)
	p, _ := s.GetPackages(ctx, id)
	ev, _ := s.GetEvidence(ctx, id)
	rv, _ := s.GetReviews(ctx, id)
	d, _ := s.GetDecision(ctx, id)
	au, _ := s.GetAudit(ctx, id)
	return json.Marshal(domain.Archive{TestRun: r, Anomalies: a, DataPackages: p, Evidence: ev, Reviews: rv, Decision: d, Audit: au, ArchiveHash: d.ArchiveHash})
}
func (s *Memory) Count(ctx context.Context, id string) map[string]int {
	a, _ := s.GetAnomalies(ctx, id)
	p, _ := s.GetPackages(ctx, id)
	e, _ := s.GetEvidence(ctx, id)
	r, _ := s.GetReviews(ctx, id)
	u, _ := s.GetAudit(ctx, id)
	return map[string]int{"packages": len(p), "anomalies": len(a), "evidence": len(e), "reviews": len(r), "audit": len(u)}
}
