package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"encoding/json"
)

func replay[T any](ctx context.Context, tx domain.Transaction, id, fp string, out *T) (bool, error) {
	r, e := tx.FindIdempotency(ctx, id)
	if e != nil || r == nil {
		return false, e
	}
	if r.Fingerprint != fp {
		return false, domain.Wrap(domain.ErrIdempotency, "request_id 已被占用", nil)
	}
	if e = json.Unmarshal(r.Response, out); e != nil {
		return false, e
	}
	return true, nil
}
func requestFingerprint(v any) string { return domain.Digest(v) }
func requireCommand(ctx context.Context, id, actor string) error {
	if e := domain.ValidateRequestID(id); e != nil {
		return e
	}
	return domain.ValidateActor(actor)
}
