package httpapi

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/domain"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/web"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	app *application.Service
	mux *http.ServeMux
}

func New(app *application.Service) *Server {
	s := &Server{app: app, mux: http.NewServeMux()}
	web.Register(s.mux)
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return s.mux }
func (s *Server) routes() {
	s.mux.HandleFunc("/", s.index)
	s.mux.HandleFunc("/healthz", s.health)
	s.mux.HandleFunc("/readyz", s.ready)
	s.mux.HandleFunc("/api/runs", s.runs)
	s.mux.HandleFunc("/api/runs/", s.runSub)
	s.mux.HandleFunc("/api/archives", s.archives)
}
func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(web.IndexHTML)
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok"})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ready"})
}
func decodeBody(r *http.Request, v any) error {
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		return domain.Invalid("Content-Type 必须为 application/json")
	}
	lr := io.LimitReader(r.Body, 1<<20)
	d := json.NewDecoder(lr)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return domain.Invalid("请求 JSON 无效")
	}
	if d.Decode(&struct{}{}) != io.EOF {
		return domain.Invalid("请求 JSON 只能包含一个对象")
	}
	return nil
}
func actor(r *http.Request) string {
	return r.Header.Get("X-Actor")
}
func reqID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return r.URL.Query().Get("request_id")
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func status(err error) int {
	var b *domain.BusinessError
	if errors.As(err, &b) {
		switch b.Code {
		case domain.ErrNotFound:
			return 404
		case domain.ErrConflict, domain.ErrIdempotency:
			return 409
		case domain.ErrForbidden:
			return 403
		case domain.ErrInvalidTransition:
			return 422
		case domain.ErrInvalidInput:
			return 400
		case domain.ErrIntegrity:
			return 500
		}
	}
	return 500
}
func fail(w http.ResponseWriter, err error) {
	var b *domain.BusinessError
	if errors.As(err, &b) {
		writeJSON(w, status(err), map[string]any{"error": string(b.Code), "message": b.Message, "details": b.Details})
		return
	}
	writeJSON(w, 500, map[string]string{"error": "INTERNAL", "message": err.Error()})
}
func (s *Server) runs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		f := application.QueueFilter{Status: domain.Status(r.URL.Query().Get("status")), RigID: r.URL.Query().Get("rig_id"), EngineRef: r.URL.Query().Get("engine_ref"), Severity: r.URL.Query().Get("severity"), Cursor: r.URL.Query().Get("cursor")}
		if n, _ := strconv.Atoi(r.URL.Query().Get("limit")); n > 0 {
			f.Limit = n
		}
		if v := r.URL.Query().Get("has_blocking"); v != "" {
			b := v == "true"
			if v != "true" && v != "false" {
				fail(w, domain.Invalid("has_blocking 无效"))
				return
			}
			f.HasBlocking = &b
		}
		if v := r.URL.Query().Get("created_from"); v != "" {
			var er error
			f.CreatedFrom, er = timeParse(v)
			if er != nil {
				fail(w, domain.Invalid("created_from 无效"))
				return
			}
		}
		if v := r.URL.Query().Get("created_to"); v != "" {
			var er error
			f.CreatedTo, er = timeParse(v)
			if er != nil {
				fail(w, domain.Invalid("created_to 无效"))
				return
			}
		}
		page, e := s.app.SearchQueue(r.Context(), f)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, page)
	case http.MethodPost:
		var in struct {
			RigID            string   `json:"rig_id"`
			EngineRef        string   `json:"engine_ref"`
			Objective        string   `json:"objective"`
			ScheduledStart   string   `json:"scheduled_start"`
			ScheduledEnd     string   `json:"scheduled_end"`
			ExpectedChannels []string `json:"expected_channels"`
		}
		if e := decodeBody(r, &in); e != nil {
			fail(w, e)
			return
		}
		start, e := timeParse(in.ScheduledStart)
		if e != nil {
			fail(w, domain.Invalid("scheduled_start 无效"))
			return
		}
		end, e := timeParse(in.ScheduledEnd)
		if e != nil {
			fail(w, domain.Invalid("scheduled_end 无效"))
			return
		}
		out, e := s.app.CreateRun(r.Context(), application.CreateRunCommand{RequestID: reqID(r), Actor: actor(r), RigID: in.RigID, EngineRef: in.EngineRef, Objective: in.Objective, ScheduledStart: start, ScheduledEnd: end, ExpectedChannels: in.ExpectedChannels})
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 201, out)
	default:
		w.WriteHeader(405)
	}
}
func (s *Server) archives(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	q := r.URL.Query()
	f := application.ArchiveLedgerFilter{RigID: q.Get("rig_id"), EngineRef: q.Get("engine_ref"), Verdict: strings.ToUpper(q.Get("verdict")), SignedBy: q.Get("signed_by"), Objective: q.Get("objective"), Cursor: q.Get("cursor")}
	if v := q.Get("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n <= 0 {
			fail(w, domain.FieldInvalid("limit", "limit 无效"))
			return
		}
		f.Limit = n
	}
	if v := q.Get("has_limitations"); v != "" {
		if v != "true" && v != "false" {
			fail(w, domain.FieldInvalid("has_limitations", "has_limitations 无效"))
			return
		}
		b := v == "true"
		f.HasLimitations = &b
	}
	if v := q.Get("signed_from"); v != "" {
		t, e := timeParse(v)
		if e != nil {
			fail(w, domain.FieldInvalid("signed_from", "signed_from 无效"))
			return
		}
		f.SignedFrom = t
	}
	if v := q.Get("signed_to"); v != "" {
		t, e := timeParse(v)
		if e != nil {
			fail(w, domain.FieldInvalid("signed_to", "signed_to 无效"))
			return
		}
		f.SignedTo = t
	}
	o, e := s.app.SearchArchives(r.Context(), f)
	if e != nil {
		fail(w, e)
		return
	}
	writeJSON(w, 200, o)
}
func timeParse(v string) (time.Time, error) { return time.Parse(time.RFC3339, v) }
func (s *Server) runSub(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/runs/"), "/")
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	if len(parts) == 1 && r.Method == http.MethodGet {
		d, e := s.app.Detail(r.Context(), id)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, d)
		return
	}
	if len(parts) == 1 && (r.Method == http.MethodPatch || r.Method == http.MethodPut) {
		var in struct {
			RigID            string   `json:"rig_id"`
			EngineRef        string   `json:"engine_ref"`
			Objective        string   `json:"objective"`
			ScheduledStart   string   `json:"scheduled_start"`
			ScheduledEnd     string   `json:"scheduled_end"`
			ExpectedChannels []string `json:"expected_channels"`
			ExpectedRevision int64    `json:"expected_revision"`
		}
		if e := decodeBody(r, &in); e != nil {
			fail(w, e)
			return
		}
		st, e := timeParse(in.ScheduledStart)
		if e != nil {
			fail(w, domain.Invalid("scheduled_start 无效"))
			return
		}
		en, e := timeParse(in.ScheduledEnd)
		if e != nil {
			fail(w, domain.Invalid("scheduled_end 无效"))
			return
		}
		o, e := s.app.ReviseBaseline(r.Context(), application.ReviseBaselineCommand{RequestID: reqID(r), Actor: actor(r), RunID: id, RigID: in.RigID, EngineRef: in.EngineRef, Objective: in.Objective, ScheduledStart: st, ScheduledEnd: en, ExpectedChannels: in.ExpectedChannels, ExpectedRevision: in.ExpectedRevision})
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
		return
	}
	if len(parts) == 2 && parts[1] == "timeline" {
		a, e := s.app.Timeline(r.Context(), id)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"events": a})
		return
	}
	if len(parts) == 2 && parts[1] == "baseline" && r.Method == http.MethodGet {
		p, e := s.app.PrecheckBaseline(r.Context(), id)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, p)
		return
	}
	if len(parts) == 2 && (parts[1] == "impact" || parts[1] == "anomaly-impact") && r.Method == http.MethodGet {
		q := r.URL.Query()
		f := domain.AnomalyImpactFilter{Statuses: queryValues(q, "status"), Kinds: queryValues(q, "type"), Severities: queryValues(q, "severity"), Channels: queryValues(q, "channel")}
		o, e := s.app.AnomalyImpact(r.Context(), id, f)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
		return
	}
	if len(parts) == 2 && parts[1] == "evidence" && r.Method == http.MethodGet {
		o, e := s.app.EvidencePrecheck(r.Context(), id)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
		return
	}
	if len(parts) == 2 && parts[1] == "review" && r.Method == http.MethodGet {
		o, e := s.app.ReviewDifference(r.Context(), id)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
		return
	}
	if len(parts) == 2 && parts[1] == "decision" && r.Method == http.MethodGet {
		p, e := s.app.DecisionReadiness(r.Context(), id)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, p)
		return
	}
	if len(parts) == 2 && parts[1] == "archive" && r.Method == http.MethodGet {
		if r.URL.Query().Get("report") == "true" {
			p, e := s.app.IntegrityReport(r.Context(), id)
			if e != nil {
				fail(w, e)
				return
			}
			writeJSON(w, 200, p)
			return
		}
		d, e := s.app.Detail(r.Context(), id)
		if e != nil {
			fail(w, e)
			return
		}
		if d.TestRun.Status != domain.StatusArchived {
			fail(w, domain.Transition("档案尚未封存"))
			return
		}
		body := domain.CanonicalArchive(d)
		hash := domain.ArchiveDigest(d)
		if d.ArchiveHash == "" || hash != d.ArchiveHash {
			fail(w, domain.Integrity("档案摘要校验失败"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+d.TestRun.ID+"-archive.json\"")
		w.Header().Set("ETag", hash)
		if r.Header.Get("If-None-Match") == hash {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write(body)
		return
	}
	var in map[string]any
	if e := decodeBody(r, &in); e != nil {
		fail(w, e)
		return
	}
	rev := int64(0)
	if !(parts[1] == "package" && r.Method == http.MethodPut) {
		var er error
		rev, er = requiredRevision(in)
		if er != nil {
			fail(w, er)
			return
		}
	}
	switch parts[1] {
	case "baseline":
		if e := onlyFields(in, "expected_revision", "candidate_baseline_hash"); e != nil {
			fail(w, e)
			return
		}
		candidate := str(in, "candidate_baseline_hash")
		var o application.Result
		var e error
		if candidate != "" {
			o, e = s.app.FreezeBaselineChecked(r.Context(), id, actor(r), reqID(r), rev, candidate)
		} else {
			o, e = s.app.FreezeBaseline(r.Context(), id, actor(r), reqID(r), rev)
		}
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
	case "package":
		if e := onlyFields(in, "expected_revision", "candidate_package_hash", "capture_start", "capture_end", "clock_drift_ms", "files", "channel_summaries", "duplicate_segments", "manifest_hash", "bytes", "channel"); e != nil {
			fail(w, e)
			return
		}
		if e := arrayFields(in, "files", "name", "digest", "bytes"); e != nil {
			fail(w, e)
			return
		}
		if e := arrayFields(in, "channel_summaries", "name", "samples", "sample_rate_hz", "min", "max"); e != nil {
			fail(w, e)
			return
		}
		if r.Method == http.MethodPut || r.URL.Query().Get("preview") == "true" {
			o, e := s.app.PreviewPackage(r.Context(), id, packageInput(in))
			if e != nil {
				fail(w, e)
				return
			}
			writeJSON(w, 200, o)
			return
		}
		p := application.RegisterPackageCommand{RequestID: reqID(r), Actor: actor(r), RunID: id, ExpectedRevision: rev, CandidatePackageHash: str(in, "candidate_package_hash"), Package: packageInput(in)}
		o, e := s.app.RegisterPackage(r.Context(), p)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
	case "triage":
		if e := onlyFields(in, "expected_revision", "items", "anomaly_id", "kind", "severity", "impact_statement", "disposition", "time_range", "affected_channels"); e != nil {
			fail(w, e)
			return
		}
		if e := arrayFields(in, "items", "anomaly_id", "kind", "severity", "impact_statement", "disposition", "time_range", "affected_channels"); e != nil {
			fail(w, e)
			return
		}
		var o application.Result
		var e error
		if raw, ok := in["items"].([]any); ok {
			items := []application.TriageItem{}
			for _, x := range raw {
				m, _ := x.(map[string]any)
				items = append(items, application.TriageItem{AnomalyID: str(m, "anomaly_id"), Kind: str(m, "kind"), Severity: str(m, "severity"), Impact: str(m, "impact_statement"), Disposition: str(m, "disposition"), TimeRange: str(m, "time_range"), Channels: arr(m, "affected_channels")})
			}
			o, e = s.app.TriageBatch(r.Context(), id, actor(r), reqID(r), rev, items)
		} else {
			o, e = s.app.Triage(r.Context(), triageInput(id, in, actor(r), reqID(r), rev))
		}
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
	case "evidence":
		if e := onlyFields(in, "expected_revision", "items", "anomaly_id", "rationale", "method", "before_digest", "after_digest", "evidence_refs", "supersedes_evidence_id", "risk_statement"); e != nil {
			fail(w, e)
			return
		}
		if e := arrayFields(in, "items", "anomaly_id", "rationale", "method", "before_digest", "after_digest", "evidence_refs", "supersedes_evidence_id", "risk_statement"); e != nil {
			fail(w, e)
			return
		}
		if raw, ok := in["items"].([]any); ok {
			items := []application.EvidenceBatchItem{}
			for _, x := range raw {
				m, _ := x.(map[string]any)
				items = append(items, application.EvidenceBatchItem{AnomalyID: str(m, "anomaly_id"), Evidence: evidenceInput(id, m, actor(r), reqID(r), rev).Evidence})
			}
			o, e := s.app.SubmitEvidenceBatch(r.Context(), application.EvidenceBatchCommand{RequestID: reqID(r), Actor: actor(r), RunID: id, ExpectedRevision: rev, Items: items})
			if e != nil {
				fail(w, e)
				return
			}
			writeJSON(w, 200, o)
			return
		}
		o, e := s.app.SubmitEvidence(r.Context(), evidenceInput(id, in, actor(r), reqID(r), rev))
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
	case "review":
		if e := onlyFields(in, "expected_revision", "outcome", "notes", "reason", "comparison_review_id", "checklist", "targets"); e != nil {
			fail(w, e)
			return
		}
		if e := arrayFields(in, "checklist", "anomaly_id", "data_package_confirmed", "gate_confirmed", "impact_confirmed", "evidence_confirmed", "before_after_confirmed", "passed", "reason"); e != nil {
			fail(w, e)
			return
		}
		if e := arrayFields(in, "targets", "anomaly_id", "reason_category", "requirement"); e != nil {
			fail(w, e)
			return
		}
		ri := reviewInput(id, in, actor(r), reqID(r), rev)
		ri.ComparisonReviewID = str(in, "comparison_review_id")
		if raw, ok := in["checklist"].([]any); ok {
			for _, x := range raw {
				m, _ := x.(map[string]any)
				ri.Checklist = append(ri.Checklist, domain.ReviewChecklistItem{AnomalyID: str(m, "anomaly_id"), Passed: boolVal(m, "passed"), DataPackageConfirmed: boolVal(m, "data_package_confirmed"), GateConfirmed: boolVal(m, "gate_confirmed"), ImpactConfirmed: boolVal(m, "impact_confirmed"), EvidenceConfirmed: boolVal(m, "evidence_confirmed"), BeforeAfterConfirmed: boolVal(m, "before_after_confirmed"), Reason: str(m, "reason")})
			}
		}
		if raw, ok := in["targets"].([]any); ok {
			for _, x := range raw {
				m, _ := x.(map[string]any)
				ri.Targets = append(ri.Targets, domain.ReviewReturnTarget{AnomalyID: str(m, "anomaly_id"), ReasonCategory: str(m, "reason_category"), Requirement: str(m, "requirement")})
			}
		}
		o, e := s.app.Review(r.Context(), ri)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
	case "decision":
		if e := onlyFields(in, "expected_revision", "verdict", "applicable_objectives", "limitations"); e != nil {
			fail(w, e)
			return
		}
		o, e := s.app.Decide(r.Context(), decisionInput(id, in, actor(r), reqID(r), rev))
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
	case "archive":
		if e := onlyFields(in, "expected_revision"); e != nil {
			fail(w, e)
			return
		}
		o, e := s.app.Archive(r.Context(), id, actor(r), reqID(r), rev)
		if e != nil {
			fail(w, e)
			return
		}
		writeJSON(w, 200, o)
	default:
		http.NotFound(w, r)
	}
}
func inNum(m map[string]any, k string) int64 {
	if f, ok := m[k].(float64); ok {
		return int64(f)
	}
	return 1
}
func requiredRevision(m map[string]any) (int64, error) {
	v, ok := m["expected_revision"].(float64)
	if !ok || v < 1 || math.Trunc(v) != v {
		return 0, domain.FieldInvalid("expected_revision", "expected_revision 必须为正整数")
	}
	return int64(v), nil
}
func str(m map[string]any, k string) string   { v, _ := m[k].(string); return v }
func boolVal(m map[string]any, k string) bool { v, _ := m[k].(bool); return v }
func onlyFields(m map[string]any, allowed ...string) error {
	ok := map[string]bool{}
	for _, k := range allowed {
		ok[k] = true
	}
	for k := range m {
		if !ok[k] {
			return domain.Invalid("未知字段 " + k)
		}
	}
	return nil
}
func arrayFields(m map[string]any, key string, allowed ...string) error {
	raw, exists := m[key]
	if !exists {
		return nil
	}
	xs, ok := raw.([]any)
	if !ok {
		return domain.Invalid(key + " 必须为数组")
	}
	for i, x := range xs {
		v, ok := x.(map[string]any)
		if !ok {
			return domain.Invalid(key + " 中的项必须为对象")
		}
		if e := onlyFields(v, allowed...); e != nil {
			return domain.Invalid(key + "[" + strconv.Itoa(i) + "] " + e.Error())
		}
	}
	return nil
}
func arr(m map[string]any, k string) []string {
	v, _ := m[k].([]any)
	o := []string{}
	for _, x := range v {
		if s, ok := x.(string); ok {
			o = append(o, s)
		}
	}
	return o
}
func queryValues(q map[string][]string, key string) []string {
	out := []string{}
	for _, v := range q[key] {
		for _, x := range strings.Split(v, ",") {
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, x)
			}
		}
	}
	return out
}
func packageInput(m map[string]any) domain.DataPackage {
	p := domain.DataPackage{CaptureStart: mustTime(str(m, "capture_start")), CaptureEnd: mustTime(str(m, "capture_end")), ClockDriftMS: inNum(m, "clock_drift_ms"), DuplicateSegments: arr(m, "duplicate_segments")}
	if raw, ok := m["files"].([]any); ok {
		for _, x := range raw {
			v, _ := x.(map[string]any)
			p.Files = append(p.Files, domain.FileEntry{Name: str(v, "name"), Digest: str(v, "digest"), Bytes: inNum(v, "bytes")})
		}
	} else {
		p.Files = []domain.FileEntry{{Name: "capture.dat", Digest: str(m, "manifest_hash"), Bytes: inNum(m, "bytes")}}
	}
	if raw, ok := m["channel_summaries"].([]any); ok {
		for _, x := range raw {
			v, _ := x.(map[string]any)
			p.ChannelSummaries = append(p.ChannelSummaries, domain.ChannelSummary{Name: str(v, "name"), Samples: inNum(v, "samples"), SampleRateHz: floatVal(v, "sample_rate_hz"), Min: floatVal(v, "min"), Max: floatVal(v, "max")})
		}
	} else {
		p.ChannelSummaries = []domain.ChannelSummary{{Name: str(m, "channel")}}
	}
	return p
}
func floatVal(m map[string]any, k string) float64 { v, _ := m[k].(float64); return v }
func mustTime(v string) time.Time                 { t, _ := time.Parse(time.RFC3339, v); return t }
func triageInput(id string, m map[string]any, a, q string, r int64) application.TriageCommand {
	return application.TriageCommand{RequestID: q, Actor: a, RunID: id, AnomalyID: str(m, "anomaly_id"), Kind: str(m, "kind"), Severity: str(m, "severity"), Impact: str(m, "impact_statement"), Disposition: str(m, "disposition"), TimeRange: str(m, "time_range"), Channels: arr(m, "affected_channels"), ExpectedRevision: r}
}
func evidenceInput(id string, m map[string]any, a, q string, r int64) application.EvidenceCommand {
	return application.EvidenceCommand{RequestID: q, Actor: a, RunID: id, AnomalyID: str(m, "anomaly_id"), ExpectedRevision: r, Evidence: domain.DispositionEvidence{Rationale: str(m, "rationale"), Method: str(m, "method"), BeforeDigest: str(m, "before_digest"), AfterDigest: str(m, "after_digest"), EvidenceRefs: arr(m, "evidence_refs"), SupersedesEvidenceID: str(m, "supersedes_evidence_id"), RiskStatement: str(m, "risk_statement")}}
}
func reviewInput(id string, m map[string]any, a, q string, r int64) application.ReviewCommand {
	return application.ReviewCommand{RequestID: q, Actor: a, RunID: id, Outcome: str(m, "outcome"), Notes: str(m, "notes"), Reason: str(m, "reason"), ExpectedRevision: r}
}
func decisionInput(id string, m map[string]any, a, q string, r int64) application.DecisionCommand {
	return application.DecisionCommand{RequestID: q, Actor: a, RunID: id, Verdict: str(m, "verdict"), Objectives: arr(m, "applicable_objectives"), Limitations: arr(m, "limitations"), ExpectedRevision: r}
}

var _ = context.Background
