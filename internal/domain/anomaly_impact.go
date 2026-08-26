package domain

import (
	"sort"
	"strings"
	"time"
)

func BuildAnomalyImpact(revision int64, baseline []string, anomalies []Anomaly, filter AnomalyImpactFilter) (AnomalyImpact, error) {
	allowedStatus := map[string]bool{"OPEN": true, "TRIAGED": true, "CLOSED": true, "NEEDS_REVISION": true}
	validate := func(field string, values []string, allowed map[string]bool) error {
		for _, v := range values {
			if !allowed[v] {
				return FieldInvalid(field, field+" 包含无效枚举值")
			}
		}
		return nil
	}
	if err := validate("status", filter.Statuses, allowedStatus); err != nil {
		return AnomalyImpact{}, err
	}
	if err := validate("type", filter.Kinds, allowedKinds); err != nil {
		return AnomalyImpact{}, err
	}
	if err := validate("severity", filter.Severities, allowedSeverities); err != nil {
		return AnomalyImpact{}, err
	}
	base := map[string]bool{}
	for _, c := range baseline {
		base[c] = true
	}
	if err := validate("channel", filter.Channels, base); err != nil {
		return AnomalyImpact{}, err
	}
	contains := func(xs []string, v string) bool {
		if len(xs) == 0 {
			return true
		}
		for _, x := range xs {
			if x == v {
				return true
			}
		}
		return false
	}
	has := func(xs []string, v string) bool {
		for _, x := range xs {
			if x == v {
				return true
			}
		}
		return false
	}
	channelMatch := func(a Anomaly) bool {
		if len(filter.Channels) == 0 {
			return true
		}
		for _, c := range a.AffectedChannels {
			if contains(filter.Channels, c) {
				return true
			}
		}
		return false
	}
	selected := []Anomaly{}
	for _, a := range anomalies {
		if contains(filter.Statuses, a.Status) && contains(filter.Kinds, a.Kind) && contains(filter.Severities, a.Severity) && channelMatch(a) {
			selected = append(selected, a)
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		ri, rj := SeverityRank(selected[i].Severity), SeverityRank(selected[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if !selected[i].UpdatedAt.Equal(selected[j].UpdatedAt) {
			return selected[i].UpdatedAt.After(selected[j].UpdatedAt)
		}
		return selected[i].ID < selected[j].ID
	})
	out := AnomalyImpact{Revision: revision, GroupCounts: map[string]map[string]int{"status": {}, "type": {}, "severity": {}}, Matrix: []AnomalyImpactRow{}, OverlapGroups: []AnomalyOverlapGroup{}, Priority: []AnomalyPriorityItem{}, InvalidRanges: []string{}}
	type span struct {
		start, end time.Time
		ok         bool
	}
	spans := map[string]span{}
	for _, a := range selected {
		start, end, ok := ParseAnomalyRange(a.TimeRange)
		spans[a.ID] = span{start, end, ok}
		out.Priority = append(out.Priority, AnomalyPriorityItem{Anomaly: a, Aggregatable: ok, RangeStart: start, RangeEnd: end})
		out.GroupCounts["status"][a.Status]++
		out.GroupCounts["type"][a.Kind]++
		out.GroupCounts["severity"][a.Severity]++
		if !ok {
			out.InvalidRanges = append(out.InvalidRanges, a.ID)
		}
	}
	for _, channel := range baseline {
		row := AnomalyImpactRow{Channel: channel, AnomalyIDs: []string{}, Kinds: []string{}, AllImpactStatementsReady: true}
		kindSet := map[string]bool{}
		highest := 4
		for _, a := range selected {
			if !has(a.AffectedChannels, channel) {
				continue
			}
			row.AnomalyIDs = append(row.AnomalyIDs, a.ID)
			kindSet[a.Kind] = true
			if SeverityRank(a.Severity) < highest {
				highest = SeverityRank(a.Severity)
				row.HighestSeverity = a.Severity
			}
			if a.Status != "CLOSED" {
				row.OpenCount++
			}
			if strings.TrimSpace(a.ImpactStatement) == "" {
				row.AllImpactStatementsReady = false
			}
			if sp := spans[a.ID]; sp.ok {
				if row.ImpactStart.IsZero() || sp.start.Before(row.ImpactStart) {
					row.ImpactStart = sp.start
				}
				if sp.end.After(row.ImpactEnd) {
					row.ImpactEnd = sp.end
				}
			}
		}
		for k := range kindSet {
			row.Kinds = append(row.Kinds, k)
		}
		sort.Strings(row.Kinds)
		sort.Strings(row.AnomalyIDs)
		if len(row.AnomalyIDs) == 0 {
			row.AllImpactStatementsReady = false
		}
		out.Matrix = append(out.Matrix, row)
	}
	// Connected components keep overlapping anomalies together without changing source records.
	parent := make([]int, len(selected))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(x int) int {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b int) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	shared := func(a, b []string) bool {
		for _, x := range a {
			for _, y := range b {
				if x == y {
					return true
				}
			}
		}
		return false
	}
	for i := range selected {
		for j := i + 1; j < len(selected); j++ {
			a, b := spans[selected[i].ID], spans[selected[j].ID]
			if a.ok && b.ok && shared(selected[i].AffectedChannels, selected[j].AffectedChannels) && a.start.Before(b.end) && b.start.Before(a.end) {
				union(i, j)
			}
		}
	}
	groups := map[int][]int{}
	for i := range selected {
		groups[find(i)] = append(groups[find(i)], i)
	}
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		g := AnomalyOverlapGroup{}
		chs := map[string]int{}
		for _, i := range idxs {
			a := selected[i]
			g.AnomalyIDs = append(g.AnomalyIDs, a.ID)
			sp := spans[a.ID]
			if g.MergedStart.IsZero() || sp.start.Before(g.MergedStart) {
				g.MergedStart = sp.start
			}
			if sp.end.After(g.MergedEnd) {
				g.MergedEnd = sp.end
			}
			if a.Severity == "BLOCKING" {
				g.BlockingCount++
			}
			for _, c := range a.AffectedChannels {
				chs[c]++
			}
		}
		for c, n := range chs {
			if n > 1 {
				g.Channels = append(g.Channels, c)
			}
		}
		sort.Strings(g.AnomalyIDs)
		sort.Strings(g.Channels)
		out.OverlapGroups = append(out.OverlapGroups, g)
	}
	sort.Slice(out.OverlapGroups, func(i, j int) bool {
		return strings.Join(out.OverlapGroups[i].AnomalyIDs, ",") < strings.Join(out.OverlapGroups[j].AnomalyIDs, ",")
	})
	sort.Strings(out.InvalidRanges)
	return out, nil
}
