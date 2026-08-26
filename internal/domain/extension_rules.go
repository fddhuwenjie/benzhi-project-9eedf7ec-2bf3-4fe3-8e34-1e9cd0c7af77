package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func FieldInvalid(field, message string) error {
	return Wrap(ErrInvalidInput, message, map[string]any{"field": field})
}

func ValidatePackageCandidate(p DataPackage) error {
	if err := ValidatePackageInput(p); err != nil {
		return err
	}
	if len(p.ChannelSummaries) == 0 {
		return FieldInvalid("channel_summaries", "channel_summaries 不能为空")
	}
	seen := map[string]bool{}
	for i, c := range p.ChannelSummaries {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return FieldInvalid(fmt.Sprintf("channel_summaries[%d].name", i), "测点名称不能为空")
		}
		if seen[name] {
			return FieldInvalid(fmt.Sprintf("channel_summaries[%d].name", i), "测点摘要不得重复")
		}
		seen[name] = true
		if c.Samples <= 0 {
			return FieldInvalid(fmt.Sprintf("channel_summaries[%d].samples", i), "样本数必须为正")
		}
		if c.SampleRateHz <= 0 {
			return FieldInvalid(fmt.Sprintf("channel_summaries[%d].sample_rate_hz", i), "采样率必须为正")
		}
		if c.Min > c.Max {
			return FieldInvalid(fmt.Sprintf("channel_summaries[%d]", i), "测点最小值不得大于最大值")
		}
	}
	dup := map[string]bool{}
	for i, segment := range p.DuplicateSegments {
		segment = strings.TrimSpace(segment)
		if segment == "" || dup[segment] {
			return FieldInvalid(fmt.Sprintf("duplicate_segments[%d]", i), "重复片段必须非空且不可重复")
		}
		dup[segment] = true
	}
	return nil
}

func PackageCandidateHash(r TestRun, p DataPackage, gates []GateResult, anomalies []Anomaly) string {
	p.ID, p.TestRunID, p.RegisteredBy = "", "", ""
	p.RegisteredAt = time.Time{}
	p.ManifestHash = ManifestHash(p.Files)
	p.GateResults = gates
	clean := append([]Anomaly{}, anomalies...)
	for i := range clean {
		clean[i].ID, clean[i].TestRunID, clean[i].Owner = "", "", ""
		clean[i].UpdatedAt = time.Time{}
	}
	return Digest([]any{r.ID, r.BaselineHash, r.Revision, p, clean})
}

func ParseAnomalyRange(v string) (time.Time, time.Time, bool) {
	parts := strings.Split(v, "/")
	if len(parts) != 2 {
		return time.Time{}, time.Time{}, false
	}
	start, e1 := time.Parse(time.RFC3339, parts[0])
	end, e2 := time.Parse(time.RFC3339, parts[1])
	return start, end, e1 == nil && e2 == nil && end.After(start)
}

func SeverityRank(v string) int {
	switch strings.ToUpper(v) {
	case "BLOCKING":
		return 0
	case "MAJOR":
		return 1
	case "MINOR":
		return 2
	default:
		return 3
	}
}

func CurrentEvidence(evidence []DispositionEvidence, anomalyID string) (DispositionEvidence, bool) {
	var current DispositionEvidence
	found := false
	for _, e := range evidence {
		if e.AnomalyID == anomalyID && e.ReplacedBy == "" && (!found || e.SubmittedAt.After(current.SubmittedAt)) {
			current, found = e, true
		}
	}
	return current, found
}

func ReviewBasisFor(packages []DataPackage, anomaly Anomaly, evidence []DispositionEvidence) ReviewBasis {
	b := ReviewBasis{AnomalyID: anomaly.ID, PackageDigest: Digest(packages), GateDigest: Digest(packageGates(packages)), ImpactDigest: Digest([]any{anomaly.Kind, anomaly.Severity, anomaly.AffectedChannels, anomaly.TimeRange, anomaly.ImpactStatement})}
	if e, ok := CurrentEvidence(evidence, anomaly.ID); ok {
		b.EvidenceID, b.EvidenceDigest = e.ID, Digest(e)
	}
	b.CombinedDigest = Digest([]any{b.PackageDigest, b.GateDigest, b.ImpactDigest, b.EvidenceID, b.EvidenceDigest})
	return b
}

func packageGates(packages []DataPackage) []GateResult {
	var out []GateResult
	for _, p := range packages {
		out = append(out, p.GateResults...)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
