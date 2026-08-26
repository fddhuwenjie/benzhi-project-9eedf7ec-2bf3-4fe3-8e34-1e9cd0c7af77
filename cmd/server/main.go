package main

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/httpapi"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "", "监听地址")
	self := flag.Bool("self-check", false, "运行自检")
	dbPath := flag.String("db", "test_runs.db", "SQLite 数据库路径")
	flag.Parse()
	listen := resolveAddr(*addr)
	if *self {
		if err := selfCheck(listen); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("自检通过")
		return
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		panic(err)
	}
	defer st.Close()
	if err = st.VerifyAudit(context.Background()); err != nil {
		panic(err)
	}
	app := application.New(st)
	srv := &http.Server{Addr: listen, Handler: httpapi.New(app).Handler(), ReadHeaderTimeout: 5 * time.Second}
	fmt.Println("服务监听", listen)
	if err = srv.ListenAndServe(); err != nil && !strings.Contains(err.Error(), "Server closed") {
		panic(err)
	}
}
func resolveAddr(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if p := os.Getenv("PORT"); p != "" {
		return "127.0.0.1:" + p
	}
	return "127.0.0.1:19081"
}
func selfCheck(addr string) error {
	tmp, err := os.CreateTemp("", "engine-check-*.db")
	if err != nil {
		return err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)
	// CGO-free builds persist the aggregate in a sidecar next to the requested
	// SQLite path; clean it up together with the temporary database used by the
	// bounded self-check.
	defer os.Remove(path + ".state")
	st, err := store.Open(path)
	if err != nil {
		return err
	}
	defer st.Close()
	app := application.New(st)
	srv := &http.Server{Handler: httpapi.New(app).Handler()}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	go srv.Serve(ln)
	defer srv.Shutdown(context.Background())
	base := "http://" + ln.Addr().String()
	client := &http.Client{Timeout: 3 * time.Second}
	post := func(path string, v any) (map[string]any, error) {
		b, _ := json.Marshal(v)
		req, _ := http.NewRequest(http.MethodPost, base+path, strings.NewReader(string(b)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Actor", "engineer")
		req.Header.Set("X-Request-ID", fmt.Sprintf("req-%d", time.Now().UnixNano()))
		res, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode >= 300 {
			return nil, fmt.Errorf("%s: %s", res.Status, string(raw))
		}
		var out map[string]any
		json.Unmarshal(raw, &out)
		return out, nil
	}
	now := time.Now().UTC()
	r, e := post("/api/runs", map[string]any{"rig_id": "RIG-CHECK", "engine_ref": "E-CHECK", "objective": "自检目标", "scheduled_start": now.Format(time.RFC3339), "scheduled_end": now.Add(time.Hour).Format(time.RFC3339), "expected_channels": []string{"N1", "EGT"}})
	if e != nil {
		return e
	}
	run := r["run"].(map[string]any)
	id := run["id"].(string)
	rev := int64(run["revision"].(float64))
	hdr := func(actor string) http.Header {
		h := http.Header{}
		h.Set("Content-Type", "application/json")
		h.Set("X-Actor", actor)
		h.Set("X-Request-ID", fmt.Sprintf("self-%d", time.Now().UnixNano()))
		return h
	}
	call := func(path string, v any, actor string) (map[string]any, error) {
		return httpCall(client, http.MethodPost, base+path, v, hdr(actor))
	}
	preview := func(path string, v any, actor string) (map[string]any, error) {
		return httpCall(client, http.MethodPut, base+path, v, hdr(actor))
	}
	_, e = call("/api/runs/"+id+"/baseline", map[string]any{"expected_revision": rev}, "engineer")
	if e != nil {
		return e
	}
	rev++
	packageBody := map[string]any{"expected_revision": rev, "capture_start": now.Format(time.RFC3339), "capture_end": now.Add(30 * time.Minute).Format(time.RFC3339), "clock_drift_ms": 75, "files": []map[string]any{{"name": "capture.dat", "digest": "digest", "bytes": 1024}}, "channel_summaries": []map[string]any{{"name": "N1", "samples": 1800, "sample_rate_hz": 1, "min": 0, "max": 1}, {"name": "EGT", "samples": 1800, "sample_rate_hz": 1, "min": 0, "max": 1}}}
	pv, e := preview("/api/runs/"+id+"/package", packageBody, "engineer")
	if e != nil {
		return e
	}
	packageBody["candidate_package_hash"] = pv["candidate_package_hash"]
	_, e = call("/api/runs/"+id+"/package", packageBody, "engineer")
	if e != nil {
		return e
	}
	rev++
	d, e := app.Detail(context.Background(), id)
	if e != nil {
		return e
	}
	for _, a := range d.Anomalies {
		_, e = call("/api/runs/"+id+"/triage", map[string]any{"expected_revision": rev, "anomaly_id": a.ID, "kind": "quality_gate", "severity": "MAJOR", "impact_statement": "已确认影响可控", "disposition": "已核查", "affected_channels": []string{"N1"}, "time_range": now.Format(time.RFC3339) + "/" + now.Add(30*time.Minute).Format(time.RFC3339)}, "engineer")
		if e != nil {
			return e
		}
		rev++
	}
	d, _ = app.Detail(context.Background(), id)
	for _, a := range d.Anomalies {
		_, e = call("/api/runs/"+id+"/evidence", map[string]any{"expected_revision": rev, "anomaly_id": a.ID, "rationale": "采用校准记录排除", "method": "对照校准曲线", "before_digest": "before", "after_digest": "after", "evidence_refs": []string{"calibration.log"}}, "engineer")
		if e != nil {
			return e
		}
		rev++
	}
	_, e = call("/api/runs/"+id+"/review", map[string]any{"expected_revision": rev, "outcome": "APPROVED", "notes": "数据链完整"}, "reviewer")
	if e != nil {
		return e
	}
	rev++
	_, e = call("/api/runs/"+id+"/decision", map[string]any{"expected_revision": rev, "verdict": "VALID", "applicable_objectives": []string{"自检目标"}, "limitations": []string{}}, "manager")
	if e != nil {
		return e
	}
	rev++
	_, e = call("/api/runs/"+id+"/archive", map[string]any{"expected_revision": rev}, "manager")
	if e != nil {
		return e
	}
	return st.VerifyAudit(context.Background())
}

func httpCall(client *http.Client, method, url string, v any, headers http.Header) (map[string]any, error) {
	b, _ := json.Marshal(v)
	req, _ := http.NewRequest(method, url, strings.NewReader(string(b)))
	req.Header = headers
	res, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("%s: %s", res.Status, string(raw))
	}
	var out map[string]any
	json.Unmarshal(raw, &out)
	return out, nil
}
func _unused() { _ = filepath.Separator }
