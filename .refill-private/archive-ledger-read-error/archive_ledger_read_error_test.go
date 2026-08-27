package archiveledgerreaderror

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/httpapi"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type failingArchiveRepo struct {
	domain.Repository
	evidenceErr error
}

func (r failingArchiveRepo) ListRuns(context.Context) ([]domain.TestRun, error) {
	return []domain.TestRun{{
		ID: "run-1", RigID: "rig-1", EngineRef: "engine-1", Objective: "目标",
		Status: domain.StatusArchived, Revision: 4, CreatedAt: time.Unix(10, 0).UTC(),
	}}, nil
}

func (r failingArchiveRepo) GetDecision(context.Context, string) (domain.ValidityDecision, error) {
	return domain.ValidityDecision{
		ID: "decision-1", TestRunID: "run-1", Verdict: "VALID", SignedBy: "owner", SignedAt: time.Unix(20, 0).UTC(),
	}, nil
}

func (r failingArchiveRepo) GetAnomalies(context.Context, string) ([]domain.Anomaly, error) {
	return []domain.Anomaly{{ID: "a-1", TestRunID: "run-1", Severity: "BLOCKING", Status: "CLOSED"}}, nil
}

func (r failingArchiveRepo) GetEvidence(context.Context, string) ([]domain.DispositionEvidence, error) {
	return nil, r.evidenceErr
}

func TestArchiveLedgerMustPropagateEvidenceReadErrors(t *testing.T) {
	want := errors.New("evidence backend unavailable")
	repo := failingArchiveRepo{evidenceErr: want}
	service := application.New(repo)

	server := httpapi.New(service)
	req := httptest.NewRequest(http.MethodGet, "/api/archives", nil).WithContext(context.Background())
	res := httptest.NewRecorder()
	server.Handler().ServeHTTP(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("GET /api/archives status = %d, want %d (dependency error %v must propagate)", res.Code, http.StatusInternalServerError, want)
	}
}
