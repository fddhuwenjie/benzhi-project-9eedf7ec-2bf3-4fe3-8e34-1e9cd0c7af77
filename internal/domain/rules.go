package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

var allowedKinds = map[string]bool{"data_gap": true, "out_of_range": true, "drift": true, "duplicate": true, "quality_gate": true}
var allowedSeverities = map[string]bool{"MINOR": true, "MAJOR": true, "BLOCKING": true}

func NormalizeChannels(ch []string) ([]string, error) {
	out := make([]string, 0, len(ch))
	seen := map[string]bool{}
	for i, v := range ch {
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, Invalid("预期测点不能为空")
		}
		if seen[v] {
			return nil, Invalid("预期测点不得重复")
		}
		seen[v] = true
		out = append(out, v)
		_ = i
	}
	return out, nil
}
func ValidatePackageInput(p DataPackage) error {
	if len(p.Files) == 0 {
		return FieldInvalid("files", "files 不能为空")
	}
	names := map[string]bool{}
	digests := map[string]string{}
	for i, f := range p.Files {
		if strings.TrimSpace(f.Name) == "" {
			return FieldInvalid(fmt.Sprintf("files[%d].name", i), "文件名无效")
		}
		if names[f.Name] {
			return FieldInvalid(fmt.Sprintf("files[%d].name", i), "文件名不得重复")
		}
		names[f.Name] = true
		if f.Bytes <= 0 {
			return FieldInvalid(fmt.Sprintf("files[%d].bytes", i), "文件字节数必须为正")
		}
		if !regexpDigest(f.Digest) {
			return FieldInvalid(fmt.Sprintf("files[%d].digest", i), "文件摘要格式无效")
		}
		if old, ok := digests[f.Digest]; ok && old != f.Name {
			return FieldInvalid(fmt.Sprintf("files[%d].digest", i), "文件摘要重复但文件内容声明不一致")
		}
		digests[f.Digest] = f.Name
	}
	if !p.CaptureEnd.After(p.CaptureStart) {
		return FieldInvalid("capture_end", "采集窗口无效")
	}
	return nil
}
func regexpDigest(v string) bool { return len(strings.TrimSpace(v)) >= 4 }
func ValidateTriage(a Anomaly, channels []string) error {
	if !allowedKinds[a.Kind] {
		return Invalid("异常类型无效")
	}
	if !allowedSeverities[a.Severity] {
		return Invalid("严重度无效")
	}
	if len(channels) == 0 || strings.TrimSpace(a.TimeRange) == "" || strings.TrimSpace(a.ImpactStatement) == "" {
		return Invalid("分诊必须包含测点、时间范围和影响说明")
	}
	return nil
}

func BaselineHash(rig, engine, objective string, channels []string, start, end string) string {
	c := append([]string{}, channels...)
	sort.Strings(c)
	b, _ := json.Marshal([]any{rig, engine, objective, c, start, end})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func ManifestHash(files []FileEntry) string {
	c := append([]FileEntry{}, files...)
	sort.Slice(c, func(i, j int) bool { return c[i].Name < c[j].Name })
	b, _ := json.Marshal(c)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func ValidateBaseline(r TestRun) error {
	if strings.TrimSpace(r.RigID) == "" || strings.TrimSpace(r.EngineRef) == "" || strings.TrimSpace(r.Objective) == "" {
		return Invalid("试车台、发动机标识和试验目的不能为空")
	}
	if len(r.ExpectedChannels) == 0 {
		return Invalid("至少需要一个预期测点")
	}
	if !r.ScheduledEnd.After(r.ScheduledStart) {
		return Invalid("允许采样窗口无效")
	}
	return nil
}
func EvaluateGates(r TestRun, p DataPackage) ([]GateResult, []Anomaly) {
	results := []GateResult{}
	present := map[string]ChannelSummary{}
	for _, c := range p.ChannelSummaries {
		present[strings.TrimSpace(c.Name)] = c
	}
	for _, ch := range r.ExpectedChannels {
		c, found := present[ch]
		results = append(results, GateResult{Name: "required_channel:" + ch, Passed: found, Message: map[bool]string{true: "测点存在", false: "缺少必需测点"}[found], Observed: found, Threshold: true, AffectedChannels: []string{ch}})
		if found && (c.Samples > 0 || c.SampleRateHz > 0) {
			want := int64(p.CaptureEnd.Sub(p.CaptureStart).Seconds() * c.SampleRateHz)
			ok := c.Samples > 0 && c.SampleRateHz > 0 && (c.Samples-want <= 1 && want-c.Samples <= 1)
			results = append(results, GateResult{Name: "sample_rate:" + ch, Passed: ok, Message: map[bool]string{true: "样本数与采样率一致", false: "样本数与采样率不一致"}[ok], Observed: map[string]any{"samples": c.Samples, "sample_rate_hz": c.SampleRateHz}, Threshold: want, AffectedChannels: []string{ch}})
		}
	}
	drift := p.ClockDriftMS
	if drift < 0 {
		drift = -drift
	}
	driftOK := drift <= 50
	results = append(results, GateResult{Name: "clock_drift", Passed: driftOK, Message: map[bool]string{true: "时钟漂移在允许范围内", false: "时钟漂移绝对值超过 50ms"}[driftOK], Observed: drift, Threshold: 50, AffectedChannels: append([]string{}, r.ExpectedChannels...)})
	rangeOK := p.CaptureStart.Before(p.CaptureEnd) && !p.CaptureStart.Before(r.ScheduledStart) && !p.CaptureEnd.After(r.ScheduledEnd)
	results = append(results, GateResult{Name: "time_window", Passed: rangeOK, Message: map[bool]string{true: "采集时段有效", false: "采集时段超出基线窗口"}[rangeOK], Observed: map[string]any{"start": p.CaptureStart, "end": p.CaptureEnd}, Threshold: map[string]any{"start": r.ScheduledStart, "end": r.ScheduledEnd}, AffectedChannels: append([]string{}, r.ExpectedChannels...)})
	for i, seg := range p.DuplicateSegments {
		results = append(results, GateResult{Name: fmt.Sprintf("duplicate:%03d", i), Passed: false, Message: "发现重复数据片段", Observed: seg, AffectedChannels: append([]string{}, r.ExpectedChannels...)})
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Name < results[j].Name })
	anoms := []Anomaly{}
	for _, g := range results {
		if !g.Passed {
			kind := "quality_gate"
			if strings.Contains(g.Name, "required_channel:") {
				kind = "data_gap"
			}
			if g.Name == "clock_drift" {
				kind = "drift"
			}
			if g.Name == "time_window" {
				kind = "out_of_range"
			}
			if strings.HasPrefix(g.Name, "duplicate:") {
				kind = "duplicate"
			}
			anoms = append(anoms, Anomaly{Kind: kind, Severity: "BLOCKING", Status: "OPEN", ImpactStatement: g.Message, AffectedChannels: g.AffectedChannels, TimeRange: p.CaptureStart.Format(time.RFC3339) + "/" + p.CaptureEnd.Format(time.RFC3339)})
		}
	}
	return results, anoms
}
func ValidateEvidence(a Anomaly, e DispositionEvidence) error {
	if a.Status == "CLOSED" {
		return Conflict("异常已经关闭")
	}
	if strings.TrimSpace(e.Rationale) == "" || strings.TrimSpace(e.Method) == "" || strings.TrimSpace(e.BeforeDigest) == "" || strings.TrimSpace(e.AfterDigest) == "" {
		return Invalid("处置证据必须包含理由、方法和修正前后摘要")
	}
	if len(e.EvidenceRefs) == 0 {
		return Invalid("至少提供一个证据引用")
	}
	seen := map[string]bool{}
	for _, r := range e.EvidenceRefs {
		if strings.TrimSpace(r) != "" {
			seen[r] = true
		}
	}
	if len(seen) == 0 {
		return Invalid("证据引用不能为空")
	}
	if e.Method == "ACCEPT" && (strings.EqualFold(a.Severity, "BLOCKING") || strings.EqualFold(a.Severity, "MAJOR")) && strings.TrimSpace(e.RiskStatement) == "" {
		return Invalid("接受阻断或重大异常必须填写风险说明")
	}
	if strings.EqualFold(e.Method, "CORRECT") && e.BeforeDigest == e.AfterDigest {
		return Invalid("修正前后摘要必须不同")
	}
	return nil
}
func BlockingOpen(anoms []Anomaly) bool {
	for _, a := range anoms {
		if strings.EqualFold(a.Severity, "BLOCKING") && a.Status != "CLOSED" {
			return true
		}
	}
	return false
}
func ValidateReview(reviewer, submitter string) error {
	if strings.TrimSpace(reviewer) == "" {
		return Invalid("复核员不能为空")
	}
	if reviewer == submitter {
		return Forbidden("处置提交人与独立复核员不得为同一身份")
	}
	return nil
}
func ValidateVerdict(v string, objectives []string, signer string) error {
	if v != "VALID" && v != "INVALID" {
		return Invalid("裁定只能是 VALID 或 INVALID")
	}
	if len(objectives) == 0 {
		return Invalid("必须指定适用试验目标")
	}
	if strings.TrimSpace(signer) == "" {
		return Invalid("签发人不能为空")
	}
	return nil
}
func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
