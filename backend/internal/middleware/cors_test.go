package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORS_PreflightAndHeaders(t *testing.T) {
	handler := CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))

	// Test 1: OPTIONS Preflight
	req := httptest.NewRequest(http.MethodOptions, "/api/user/verification/start", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for OPTIONS preflight, got %d", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("expected allow origin http://localhost:3000, got %s", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("expected allow credentials true, got %s", rec.Header().Get("Access-Control-Allow-Credentials"))
	}

	// Test 2: Standard GET with Origin
	req2 := httptest.NewRequest(http.MethodGet, "/api/user/profile", nil)
	req2.Header.Set("Origin", "http://localhost:3000")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for GET, got %d", rec2.Code)
	}
	if rec2.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("expected allow origin http://localhost:3000, got %s", rec2.Header().Get("Access-Control-Allow-Origin"))
	}
}
