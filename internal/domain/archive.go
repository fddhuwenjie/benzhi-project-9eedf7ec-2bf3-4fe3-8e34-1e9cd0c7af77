package domain

import (
	"encoding/json"
	"sort"
)

func CanonicalArchive(a Archive) []byte {
	a.DataPackages = append([]DataPackage{}, a.DataPackages...)
	a.Anomalies = append([]Anomaly{}, a.Anomalies...)
	a.Evidence = append([]DispositionEvidence{}, a.Evidence...)
	a.Reviews = append([]ReviewSnapshot{}, a.Reviews...)
	a.Audit = append([]AuditEvent{}, a.Audit...)
	sort.Slice(a.DataPackages, func(i, j int) bool { return a.DataPackages[i].ID < a.DataPackages[j].ID })
	for i := range a.DataPackages {
		sort.Slice(a.DataPackages[i].Files, func(x, y int) bool { return a.DataPackages[i].Files[x].Name < a.DataPackages[i].Files[y].Name })
		sort.Slice(a.DataPackages[i].ChannelSummaries, func(x, y int) bool {
			return a.DataPackages[i].ChannelSummaries[x].Name < a.DataPackages[i].ChannelSummaries[y].Name
		})
		sort.Slice(a.DataPackages[i].GateResults, func(x, y int) bool {
			return a.DataPackages[i].GateResults[x].Name < a.DataPackages[i].GateResults[y].Name
		})
	}
	sort.Slice(a.Anomalies, func(i, j int) bool { return a.Anomalies[i].ID < a.Anomalies[j].ID })
	sort.Slice(a.Evidence, func(i, j int) bool {
		if a.Evidence[i].AnomalyID == a.Evidence[j].AnomalyID {
			return a.Evidence[i].SubmittedAt.Before(a.Evidence[j].SubmittedAt)
		}
		return a.Evidence[i].AnomalyID < a.Evidence[j].AnomalyID
	})
	sort.Slice(a.Reviews, func(i, j int) bool { return a.Reviews[i].LockedAt.Before(a.Reviews[j].LockedAt) })
	sort.Slice(a.Audit, func(i, j int) bool { return a.Audit[i].Sequence < a.Audit[j].Sequence })
	b, _ := json.Marshal(a)
	return b
}
func ArchiveDigest(a Archive) string {
	a.ArchiveHash = ""
	a.Decision.ArchiveHash = ""
	if a.TestRun.Status == StatusArchived {
		a.TestRun.Status = StatusDecided
		a.TestRun.Revision--
	}
	events := a.Audit[:0]
	for _, e := range a.Audit {
		if e.Action != "ARCHIVE" {
			events = append(events, e)
		}
	}
	a.Audit = events
	return Digest(json.RawMessage(CanonicalArchive(a)))
}
func ReviewLocked(rs []ReviewSnapshot) bool {
	for _, r := range rs {
		if r.Outcome == "APPROVED" && !r.LockedAt.IsZero() {
			return true
		}
	}
	return false
}
func LatestReview(rs []ReviewSnapshot) (ReviewSnapshot, bool) {
	if len(rs) == 0 {
		return ReviewSnapshot{}, false
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].LockedAt.Before(rs[j].LockedAt) })
	return rs[len(rs)-1], true
}
