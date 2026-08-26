package domain

import "time"

type PackagePreview struct {
	TestRunID            string       `json:"test_run_id"`
	Revision             int64        `json:"revision"`
	BaselineHash         string       `json:"baseline_hash"`
	ManifestHash         string       `json:"manifest_hash"`
	CandidatePackageHash string       `json:"candidate_package_hash"`
	Files                []FileEntry  `json:"files"`
	GateResults          []GateResult `json:"gate_results"`
	ExpectedAnomalies    []Anomaly    `json:"expected_anomalies"`
}

type AnomalyImpactFilter struct {
	Statuses   []string
	Kinds      []string
	Severities []string
	Channels   []string
}

type AnomalyImpactRow struct {
	Channel                  string    `json:"channel"`
	AnomalyIDs               []string  `json:"anomaly_ids"`
	Kinds                    []string  `json:"anomaly_types"`
	HighestSeverity          string    `json:"highest_severity,omitempty"`
	OpenCount                int       `json:"open_count"`
	ImpactStart              time.Time `json:"impact_start,omitempty"`
	ImpactEnd                time.Time `json:"impact_end,omitempty"`
	AllImpactStatementsReady bool      `json:"all_impact_statements_ready"`
}

type AnomalyPriorityItem struct {
	Anomaly      Anomaly   `json:"anomaly"`
	Aggregatable bool      `json:"aggregatable"`
	RangeStart   time.Time `json:"range_start,omitempty"`
	RangeEnd     time.Time `json:"range_end,omitempty"`
}

type AnomalyOverlapGroup struct {
	AnomalyIDs    []string  `json:"anomaly_ids"`
	Channels      []string  `json:"channels"`
	MergedStart   time.Time `json:"merged_start"`
	MergedEnd     time.Time `json:"merged_end"`
	BlockingCount int       `json:"blocking_count"`
}

type AnomalyImpact struct {
	Revision      int64                     `json:"revision"`
	Matrix        []AnomalyImpactRow        `json:"matrix"`
	OverlapGroups []AnomalyOverlapGroup     `json:"overlap_groups"`
	Priority      []AnomalyPriorityItem     `json:"priority"`
	GroupCounts   map[string]map[string]int `json:"group_counts"`
	InvalidRanges []string                  `json:"invalid_range_anomaly_ids"`
}

type EvidencePrecheckItem struct {
	AnomalyID            string               `json:"anomaly_id"`
	Status               string               `json:"status"`
	EffectiveEvidence    *DispositionEvidence `json:"effective_evidence,omitempty"`
	MissingFields        []string             `json:"missing_fields"`
	SupersedesEvidenceID string               `json:"supersedes_evidence_id,omitempty"`
	BlockingReasons      []string             `json:"blocking_reasons"`
}

type EvidencePrecheck struct {
	Revision       int64                  `json:"revision"`
	Items          []EvidencePrecheckItem `json:"items"`
	ExpectedStatus Status                 `json:"expected_status"`
	RemainingIDs   []string               `json:"remaining_anomaly_ids"`
}

type ReviewBasis struct {
	AnomalyID      string `json:"anomaly_id"`
	PackageDigest  string `json:"package_digest"`
	GateDigest     string `json:"gate_digest"`
	ImpactDigest   string `json:"impact_digest"`
	EvidenceID     string `json:"evidence_id,omitempty"`
	EvidenceDigest string `json:"evidence_digest,omitempty"`
	CombinedDigest string `json:"combined_digest"`
}

type ReviewDifferenceItem struct {
	AnomalyID              string               `json:"anomaly_id"`
	ReasonCategory         string               `json:"reason_category,omitempty"`
	Requirement            string               `json:"requirement,omitempty"`
	ReturnedEvidenceID     string               `json:"returned_evidence_id,omitempty"`
	ReturnedEvidenceDigest string               `json:"returned_evidence_digest,omitempty"`
	CurrentEvidence        *DispositionEvidence `json:"current_evidence,omitempty"`
	SupersessionChain      []string             `json:"supersession_chain"`
	CurrentStatus          string               `json:"current_status"`
	Classification         string               `json:"classification"`
	Drifted                bool                 `json:"drifted"`
	BlockingReasons        []string             `json:"blocking_reasons"`
}

type ReviewDifference struct {
	Revision           int64                  `json:"revision"`
	ComparisonReviewID string                 `json:"comparison_review_id"`
	ReturnedTargets    []ReviewDifferenceItem `json:"returned_targets"`
	DriftedUntargeted  []ReviewDifferenceItem `json:"drifted_untargeted"`
	BlockingAnomalyIDs []string               `json:"blocking_anomaly_ids"`
}
