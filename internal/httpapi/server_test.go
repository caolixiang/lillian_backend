package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CookSleep/lillian_backend/internal/config"
)

func TestHealth(t *testing.T) {
	server := New(config.Config{
		ServiceName: "lillian-backend",
		Version:     "1.2.3",
		Environment: "test",
		CORSOrigin:  "*",
	}, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["version"] != "1.2.3" {
		t.Fatalf("version = %v", payload["version"])
	}
}

func TestConfig(t *testing.T) {
	server := New(config.Config{
		PublicAPIBaseURL: "https://api.example.com",
		CORSOrigin:       "*",
	}, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/config.json", nil)
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["apiBaseUrl"] != "https://api.example.com" {
		t.Fatalf("apiBaseUrl = %v", payload["apiBaseUrl"])
	}
}
