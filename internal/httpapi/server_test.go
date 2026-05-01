package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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
	t.Run("uses configured public API base URL", func(t *testing.T) {
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
	})

	t.Run("derives public API base URL from forwarded request", func(t *testing.T) {
		server := New(config.Config{CORSOrigin: "*"}, nil, nil, nil)

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "http://internal.example/config.json", nil)
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Forwarded-Host", "api.example.com")
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
	})
}

func TestCorsDefaultsToWildcard(t *testing.T) {
	server := New(config.Config{}, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/config.json", nil)
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q", got)
	}
}

func TestAdminPage(t *testing.T) {
	server := New(config.Config{CORSOrigin: "*"}, nil, nil, nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content-type = %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "莉莉安的后台") {
		t.Fatalf("admin page missing expected content")
	}
	if !strings.Contains(body, "/admin/assets/") && !strings.Contains(body, "Admin frontend is not built") {
		t.Fatalf("admin page is neither built index nor fallback")
	}
}

func TestIcon(t *testing.T) {
	server := New(config.Config{CORSOrigin: "*"}, nil, nil, nil)

	for _, path := range []string{"/lillian-icon.svg", "/admin/lillian-icon.svg"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		server.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		if got := rec.Header().Get("Content-Type"); got != "image/svg+xml; charset=utf-8" {
			t.Fatalf("%s content-type = %q", path, got)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "<svg") {
			t.Fatalf("%s icon response is not svg", path)
		}
		if !strings.Contains(body, `viewBox="0 0 1024 1024"`) {
			t.Fatalf("%s icon response is not the shared Lillian icon", path)
		}
	}
}
