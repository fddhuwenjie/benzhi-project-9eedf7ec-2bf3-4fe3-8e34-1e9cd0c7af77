package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
)

func (s *Service) AnomalyImpact(ctx context.Context, runID string, filter domain.AnomalyImpactFilter) (domain.AnomalyImpact, error) {
	r, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return domain.AnomalyImpact{}, err
	}
	anomalies, err := s.repo.GetAnomalies(ctx, runID)
	if err != nil {
		return domain.AnomalyImpact{}, err
	}
	return domain.BuildAnomalyImpact(r.Revision, r.ExpectedChannels, anomalies, filter)
}
