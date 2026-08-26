package domain

import "time"

type TestRun struct {
	ID               string         `json:"id"`
	RigID            string         `json:"rig_id"`
	EngineRef        string         `json:"engine_ref"`
	Objective        string         `json:"objective"`
	ScheduledStart   time.Time      `json:"scheduled_start"`
	ScheduledEnd     time.Time      `json:"scheduled_end"`
	ExpectedChannels []string       `json:"expected_channels"`
	BaselineHash     string         `json:"baseline_hash"`
	Status           Status         `json:"status"`
	Revision         int64          `json:"revision"`
	CreatedBy        string         `json:"created_by"`
	CreatedAt        time.Time      `json:"created_at"`
	PreviousDraft    *BaselineDraft `json:"previous_draft,omitempty"`
}
type BaselineDraft struct {
	RigID            string    `json:"rig_id"`
	EngineRef        string    `json:"engine_ref"`
	Objective        string    `json:"objective"`
	ScheduledStart   time.Time `json:"scheduled_start"`
	ScheduledEnd     time.Time `json:"scheduled_end"`
	ExpectedChannels []string  `json:"expected_channels"`
	Revision         int64     `json:"revision"`
}
type DataPackage struct {
	ID                string           `json:"id"`
	TestRunID         string           `json:"test_run_id"`
	ManifestHash      string           `json:"manifest_hash"`
	Files             []FileEntry      `json:"files"`
	ChannelSummaries  []ChannelSummary `json:"channel_summaries"`
	CaptureStart      time.Time        `json:"capture_start"`
	CaptureEnd        time.Time        `json:"capture_end"`
	ClockDriftMS      int64            `json:"clock_drift_ms"`
	GateResults       []GateResult     `json:"gate_results"`
	DuplicateSegments []string         `json:"duplicate_segments,omitempty"`
	RegisteredBy      string           `json:"registered_by"`
	RegisteredAt      time.Time        `json:"registered_at"`
}
type FileEntry struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Bytes  int64  `json:"bytes"`
}
type ChannelSummary struct {
	Name         string  `json:"name"`
	Samples      int64   `json:"samples"`
	SampleRateHz float64 `json:"sample_rate_hz"`
	Min          float64 `json:"min"`
	Max          float64 `json:"max"`
}
type GateResult struct {
	Name             string   `json:"name"`
	Passed           bool     `json:"passed"`
	Message          string   `json:"message"`
	Observed         any      `json:"observed,omitempty"`
	Threshold        any      `json:"threshold,omitempty"`
	AffectedChannels []string `json:"affected_channels,omitempty"`
}
type Anomaly struct {
	ID               string    `json:"id"`
	TestRunID        string    `json:"test_run_id"`
	Kind             string    `json:"kind"`
	Severity         string    `json:"severity"`
	AffectedChannels []string  `json:"affected_channels"`
	TimeRange        string    `json:"time_range"`
	ImpactStatement  string    `json:"impact_statement"`
	Disposition      string    `json:"disposition"`
	Status           string    `json:"status"`
	Owner            string    `json:"owner"`
	UpdatedAt        time.Time `json:"updated_at"`
}
type DispositionEvidence struct {
	ID                   string    `json:"id"`
	AnomalyID            string    `json:"anomaly_id"`
	Rationale            string    `json:"rationale"`
	Method               string    `json:"method"`
	BeforeDigest         string    `json:"before_digest"`
	AfterDigest          string    `json:"after_digest"`
	EvidenceRefs         []string  `json:"evidence_refs"`
	SubmittedBy          string    `json:"submitted_by"`
	SubmittedAt          time.Time `json:"submitted_at"`
	SupersedesEvidenceID string    `json:"supersedes_evidence_id,omitempty"`
	RiskStatement        string    `json:"risk_statement,omitempty"`
	ReplacedBy           string    `json:"replaced_by,omitempty"`
}
type ValidityDecision struct {
	ID                   string    `json:"id"`
	TestRunID            string    `json:"test_run_id"`
	Reviewer             string    `json:"reviewer"`
	ReviewOutcome        string    `json:"review_outcome"`
	ReviewNotes          string    `json:"review_notes"`
	Verdict              string    `json:"verdict"`
	ApplicableObjectives []string  `json:"applicable_objectives"`
	Limitations          []string  `json:"limitations"`
	SignedBy             string    `json:"signed_by"`
	SignedAt             time.Time `json:"signed_at"`
	ArchiveHash          string    `json:"archive_hash"`
}
type AuditEvent struct {
	Sequence       int64     `json:"sequence"`
	TestRunID      string    `json:"test_run_id"`
	RequestID      string    `json:"request_id"`
	Actor          string    `json:"actor"`
	Action         string    `json:"action"`
	FromStatus     Status    `json:"from_status"`
	ToStatus       Status    `json:"to_status"`
	BeforeRevision int64     `json:"before_revision"`
	AfterRevision  int64     `json:"after_revision"`
	PayloadDigest  string    `json:"payload_digest"`
	PreviousHash   string    `json:"previous_hash"`
	EventHash      string    `json:"event_hash"`
	OccurredAt     time.Time `json:"occurred_at"`
}
type ReviewChecklistItem struct {
	AnomalyID            string `json:"anomaly_id"`
	DataPackageConfirmed bool   `json:"data_package_confirmed"`
	GateConfirmed        bool   `json:"gate_confirmed"`
	ImpactConfirmed      bool   `json:"impact_confirmed"`
	EvidenceConfirmed    bool   `json:"evidence_confirmed"`
	BeforeAfterConfirmed bool   `json:"before_after_confirmed"`
	Passed               bool   `json:"passed"`
	Reason               string `json:"reason,omitempty"`
}
type ReviewReturnTarget struct {
	AnomalyID      string `json:"anomaly_id"`
	ReasonCategory string `json:"reason_category"`
	Requirement    string `json:"requirement"`
}
type ReviewSnapshot struct {
	ID                 string                `json:"id"`
	TestRunID          string                `json:"test_run_id"`
	Reviewer           string                `json:"reviewer"`
	Outcome            string                `json:"outcome"`
	Notes              string                `json:"notes"`
	ReturnedReason     string                `json:"returned_reason,omitempty"`
	LockedAt           time.Time             `json:"locked_at"`
	SnapshotHash       string                `json:"snapshot_hash,omitempty"`
	Checklist          []ReviewChecklistItem `json:"checklist,omitempty"`
	Targets            []ReviewReturnTarget  `json:"targets,omitempty"`
	Basis              []ReviewBasis         `json:"basis,omitempty"`
	ComparisonReviewID string                `json:"comparison_review_id,omitempty"`
	DifferenceDigest   string                `json:"difference_digest,omitempty"`
}
type Archive struct {
	TestRun      TestRun               `json:"test_run"`
	DataPackages []DataPackage         `json:"data_packages"`
	Anomalies    []Anomaly             `json:"anomalies"`
	Evidence     []DispositionEvidence `json:"evidence"`
	Decision     ValidityDecision      `json:"decision"`
	Reviews      []ReviewSnapshot      `json:"reviews"`
	Audit        []AuditEvent          `json:"audit"`
	ArchiveHash  string                `json:"archive_hash"`
}
