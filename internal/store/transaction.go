package store

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"time"
)

func NewAudit(run, req, actor, action string, from, to domain.Status, before, after int64, payload any) domain.AuditEvent {
	return domain.AuditEvent{TestRunID: run, RequestID: req, Actor: actor, Action: action, FromStatus: from, ToStatus: to, BeforeRevision: before, AfterRevision: after, PayloadDigest: domain.Digest(payload), OccurredAt: time.Now().UTC()}
}
func (s *Memory) Health(ctx context.Context) error { return s.VerifyAudit(ctx) }
