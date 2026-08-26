package domain

import "context"

type IdempotencyRecord struct {
	RequestID   string
	Fingerprint string
	StatusCode  int
	Response    []byte
}
type Repository interface {
	Migrate(context.Context) error
	VerifyAudit(context.Context) error
	WithinTx(context.Context, func(Transaction) error) error
	GetRun(context.Context, string) (TestRun, error)
	ListRuns(context.Context) ([]TestRun, error)
	GetPackages(context.Context, string) ([]DataPackage, error)
	GetAnomalies(context.Context, string) ([]Anomaly, error)
	GetEvidence(context.Context, string) ([]DispositionEvidence, error)
	GetReviews(context.Context, string) ([]ReviewSnapshot, error)
	GetDecision(context.Context, string) (ValidityDecision, error)
	GetAudit(context.Context, string) ([]AuditEvent, error)
}
type Transaction interface {
	GetRun(context.Context, string) (TestRun, error)
	GetPackages(context.Context, string) ([]DataPackage, error)
	InsertRun(context.Context, TestRun) error
	UpdateRun(context.Context, TestRun, int64) error
	InsertPackage(context.Context, DataPackage) error
	InsertAnomalies(context.Context, []Anomaly) error
	UpdateAnomaly(context.Context, Anomaly) error
	GetAnomalies(context.Context, string) ([]Anomaly, error)
	InsertEvidence(context.Context, DispositionEvidence) error
	UpdateEvidence(context.Context, DispositionEvidence) error
	GetEvidence(context.Context, string) ([]DispositionEvidence, error)
	GetReviews(context.Context, string) ([]ReviewSnapshot, error)
	InsertReview(context.Context, ReviewSnapshot) error
	InsertDecision(context.Context, ValidityDecision) error
	UpdateDecisionArchive(context.Context, string, string) error
	FindIdempotency(context.Context, string) (*IdempotencyRecord, error)
	SaveIdempotency(context.Context, IdempotencyRecord) error
	AppendAudit(context.Context, AuditEvent) error
}
