package httpapi

import (
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/application"
	"benzhi-project-9eedf7ec-2bf3-4fe3-8e34-1e9cd0c7af77/internal/store"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testServer() *Server { r, _ := store.Open(":memory:"); return New(application.New(r)) }
func TestIndexAndReadiness(t *testing.T) {
	s := testServer()
	for _, p := range []string{"/", "/readyz"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		if w.Code != 200 {
			t.Fatalf("%s: %d", p, w.Code)
		}
	}
}
func TestStrictJSON(t *testing.T) {
	s := testServer()
	body := `{"rig_id":"R","unknown":true}`
	q := httptest.NewRequest(http.MethodPost, "/api/runs", strings.NewReader(body))
	q.Header.Set("Content-Type", "application/json")
	q.Header.Set("X-Request-ID", "req")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, q)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	var v map[string]any
	if json.Unmarshal(w.Body.Bytes(), &v) != nil || v["error"] != "INVALID_INPUT" {
		t.Fatal(w.Body.String())
	}
}
