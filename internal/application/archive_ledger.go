package application

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ArchiveLedgerFilter struct {
	RigID, EngineRef, Verdict, SignedBy, Objective string
	SignedFrom, SignedTo                           time.Time
	HasLimitations                                 *bool
	Cursor                                         string
	Limit                                          int
}
type ArchiveLedgerRecord struct {
	TestRunID                 string    `json:"test_run_id"`
	RigID                     string    `json:"rig_id"`
	EngineRef                 string    `json:"engine_ref"`
	ArchiveHash               string    `json:"archive_hash"`
	Verdict                   string    `json:"verdict"`
	SignedBy                  string    `json:"signed_by"`
	SignedAt                  time.Time `json:"signed_at"`
	ApplicableObjectives      []string  `json:"applicable_objectives"`
	LimitationCount           int       `json:"limitation_count"`
	AnomalyCount              int       `json:"anomaly_count"`
	AcceptedHighSeverityCount int       `json:"accepted_high_severity_count"`
	DownloadURL               string    `json:"download_url"`
	DetailURL                 string    `json:"detail_url"`
	TimelineURL               string    `json:"timeline_url"`
	hasHighSeverity           bool
}
type ArchiveLedgerStats struct {
	Valid            int `json:"valid"`
	Invalid          int `json:"invalid"`
	WithLimitations  int `json:"with_limitations"`
	WithHighSeverity int `json:"with_major_or_blocking_anomalies"`
}
type ArchiveLedgerPage struct {
	Records    []ArchiveLedgerRecord `json:"records"`
	Stats      ArchiveLedgerStats    `json:"stats"`
	NextCursor string                `json:"next_cursor,omitempty"`
}

func (s *Service) SearchArchives(ctx context.Context, f ArchiveLedgerFilter) (ArchiveLedgerPage, error) {
	if f.Verdict != "" && f.Verdict != "VALID" && f.Verdict != "INVALID" {
		return ArchiveLedgerPage{}, domain.FieldInvalid("verdict", "verdict 无效")
	}
	if !f.SignedFrom.IsZero() && !f.SignedTo.IsZero() && f.SignedTo.Before(f.SignedFrom) {
		return ArchiveLedgerPage{}, domain.FieldInvalid("signed_to", "签发时间范围倒置")
	}
	if f.Limit <= 0 {
		f.Limit = 20
	}
	if f.Limit > 100 {
		f.Limit = 100
	}
	runs, err := s.repo.ListRuns(ctx)
	if err != nil {
		return ArchiveLedgerPage{}, err
	}
	records := []ArchiveLedgerRecord{}
	for _, r := range runs {
		if r.Status != domain.StatusArchived {
			continue
		}
		if f.RigID != "" && !strings.Contains(strings.ToLower(r.RigID), strings.ToLower(f.RigID)) {
			continue
		}
		if f.EngineRef != "" && !strings.Contains(strings.ToLower(r.EngineRef), strings.ToLower(f.EngineRef)) {
			continue
		}
		d, er := s.repo.GetDecision(ctx, r.ID)
		if er != nil {
			return ArchiveLedgerPage{}, er
		}
		if f.Verdict != "" && d.Verdict != f.Verdict || f.SignedBy != "" && d.SignedBy != f.SignedBy || !f.SignedFrom.IsZero() && d.SignedAt.Before(f.SignedFrom) || !f.SignedTo.IsZero() && d.SignedAt.After(f.SignedTo) {
			continue
		}
		if f.Objective != "" {
			matched := false
			for _, o := range d.ApplicableObjectives {
				if strings.Contains(strings.ToLower(o), strings.ToLower(f.Objective)) {
					matched = true
				}
			}
			if !matched {
				continue
			}
		}
		if f.HasLimitations != nil && (*f.HasLimitations) != (len(d.Limitations) > 0) {
			continue
		}
		as, er := s.repo.GetAnomalies(ctx, r.ID)
		if er != nil {
			return ArchiveLedgerPage{}, er
		}
		evidence, er := s.repo.GetEvidence(ctx, r.ID)
		if er != nil {
			return ArchiveLedgerPage{}, er
		}
		accepted := 0
		hasHigh := false
		for _, a := range as {
			if a.Severity == "MAJOR" || a.Severity == "BLOCKING" {
				hasHigh = true
				current, ok := domain.CurrentEvidence(evidence, a.ID)
				if ok && strings.EqualFold(current.Method, "ACCEPT") {
					accepted++
				}
			}
		}
		records = append(records, ArchiveLedgerRecord{TestRunID: r.ID, RigID: r.RigID, EngineRef: r.EngineRef, ArchiveHash: d.ArchiveHash, Verdict: d.Verdict, SignedBy: d.SignedBy, SignedAt: d.SignedAt, ApplicableObjectives: d.ApplicableObjectives, LimitationCount: len(d.Limitations), AnomalyCount: len(as), AcceptedHighSeverityCount: accepted, DownloadURL: "/api/runs/" + r.ID + "/archive", DetailURL: "/api/runs/" + r.ID, TimelineURL: "/api/runs/" + r.ID + "/timeline", hasHighSeverity: hasHigh})
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].SignedAt.Equal(records[j].SignedAt) {
			return records[i].TestRunID < records[j].TestRunID
		}
		return records[i].SignedAt.After(records[j].SignedAt)
	})
	stats := ArchiveLedgerStats{}
	for _, x := range records {
		if x.Verdict == "VALID" {
			stats.Valid++
		} else {
			stats.Invalid++
		}
		if x.LimitationCount > 0 {
			stats.WithLimitations++
		}
		if x.hasHighSeverity {
			stats.WithHighSeverity++
		}
	}
	start := 0
	if f.Cursor != "" {
		raw, er := base64.RawURLEncoding.DecodeString(f.Cursor)
		if er != nil {
			return ArchiveLedgerPage{}, domain.FieldInvalid("cursor", "cursor 无效")
		}
		parts := strings.Split(string(raw), "|")
		if len(parts) != 2 {
			return ArchiveLedgerPage{}, domain.FieldInvalid("cursor", "cursor 无效")
		}
		ns, er := strconv.ParseInt(parts[0], 10, 64)
		if er != nil {
			return ArchiveLedgerPage{}, domain.FieldInvalid("cursor", "cursor 无效")
		}
		found := false
		for i, x := range records {
			if x.SignedAt.UnixNano() == ns && x.TestRunID == parts[1] {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return ArchiveLedgerPage{}, domain.FieldInvalid("cursor", "cursor 无效")
		}
	}
	end := start + f.Limit
	if end > len(records) {
		end = len(records)
	}
	page := ArchiveLedgerPage{Records: records[start:end], Stats: stats}
	if end < len(records) && end > 0 {
		x := records[end-1]
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("%d|%s", x.SignedAt.UnixNano(), x.TestRunID)))
	}
	return page, nil
}
