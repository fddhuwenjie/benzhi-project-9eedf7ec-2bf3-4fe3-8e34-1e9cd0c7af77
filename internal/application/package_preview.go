package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
)

func packagePreview(r domain.TestRun, p domain.DataPackage) (domain.PackagePreview, error) {
	if r.Status != domain.StatusBaselined {
		return domain.PackagePreview{}, domain.Transition("只有已冻结基线的任务可以预演数据包")
	}
	if err := domain.ValidatePackageCandidate(p); err != nil {
		return domain.PackagePreview{}, err
	}
	p.ManifestHash = domain.ManifestHash(p.Files)
	gates, anomalies := domain.EvaluateGates(r, p)
	return domain.PackagePreview{TestRunID: r.ID, Revision: r.Revision, BaselineHash: r.BaselineHash, ManifestHash: p.ManifestHash, CandidatePackageHash: domain.PackageCandidateHash(r, p, gates, anomalies), Files: append([]domain.FileEntry{}, p.Files...), GateResults: gates, ExpectedAnomalies: anomalies}, nil
}

func (s *Service) PreviewPackage(ctx context.Context, runID string, p domain.DataPackage) (domain.PackagePreview, error) {
	r, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return domain.PackagePreview{}, err
	}
	return packagePreview(r, p)
}
