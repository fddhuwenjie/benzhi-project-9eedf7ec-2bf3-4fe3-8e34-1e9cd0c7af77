package domain

import (
	"regexp"
	"strings"
	"time"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,63}$`)

func ValidateActor(v string) error {
	if strings.TrimSpace(v) == "" || len(v) > 80 {
		return Invalid("操作者标识无效")
	}
	return nil
}
func ValidateRequestID(v string) error {
	if !idPattern.MatchString(v) {
		return Invalid("request_id 格式无效")
	}
	return nil
}
func ValidateChannels(ch []string) error {
	seen := map[string]bool{}
	for _, v := range ch {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			return Invalid("测点名称不能为空且不可重复")
		}
		seen[v] = true
	}
	return nil
}
func ValidateWindow(start, end time.Time) error {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return Invalid("时间窗口必须为正区间")
	}
	if end.Sub(start) > 24*time.Hour {
		return Invalid("采样窗口不得超过 24 小时")
	}
	return nil
}
func NormalizeSeverity(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	switch v {
	case "BLOCKING", "MAJOR", "MINOR":
		return v
	default:
		return "MAJOR"
	}
}
func NormalizeKind(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "data_gap", "out_of_range", "drift", "duplicate", "quality_gate":
		return v
	default:
		return "quality_gate"
	}
}
func GateSummary(gs []GateResult) (passed, failed int) {
	for _, g := range gs {
		if g.Passed {
			passed++
		} else {
			failed++
		}
	}
	return
}
