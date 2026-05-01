package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/CookSleep/lillian_backend/internal/config"
	"github.com/CookSleep/lillian_backend/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	cfg    config.Config
	db     *pgxpool.Pool
	store  storage.ObjectStore
	logger *log.Logger
}

func New(cfg config.Config, db *pgxpool.Pool, store storage.ObjectStore, logger *log.Logger) *Server {
	return &Server{
		cfg:    cfg,
		db:     db,
		store:  store,
		logger: logger,
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.cors)
	r.Get("/health", s.handleHealth)
	r.Get("/ready", s.handleReady)
	r.Get("/config.json", s.handleConfig)
	r.Post("/api/keys/activate", s.handleActivateLicense)
	r.Get("/api/me/credits", s.handleCredits)
	r.Post("/api/tasks", s.handleCreateTask)
	r.Get("/api/tasks/{id}", s.handleGetTask)
	r.Get("/api/tasks/{id}/images/{index}", s.handleGetTaskImage)
	r.Post("/admin/licenses", s.handleAdminCreateLicenses)
	r.Get("/admin/licenses", s.handleAdminListLicenses)
	r.Post("/admin/licenses/delete", s.handleAdminDeleteLicenses)
	r.Post("/admin/licenses/{id}", s.handleAdminUpdateLicense)
	r.Patch("/admin/licenses/{id}", s.handleAdminUpdateLicense)
	r.Delete("/admin/licenses/{id}", s.handleAdminDeleteLicenses)
	r.Get("/admin/service-profiles", s.handleAdminListServiceProfiles)
	r.Post("/admin/service-profiles", s.handleAdminUpsertServiceProfile)
	r.Delete("/admin/service-profiles/{id}", s.handleAdminDeleteServiceProfile)
	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": s.cfg.ServiceName,
		"version": s.cfg.Version,
		"env":     s.cfg.Environment,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 3*time.Second)
	defer cancel()

	ready := true
	checks := map[string]any{}
	if s.db == nil {
		ready = false
		checks["database"] = "not_configured"
	} else if err := s.db.Ping(ctx); err != nil {
		ready = false
		checks["database"] = err.Error()
	} else {
		checks["database"] = "ok"
	}

	if s.store == nil {
		ready = false
		checks["storage"] = "not_configured"
	} else {
		checks["storage"] = "configured"
	}

	status := http.StatusOK
	if !ready {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{
		"ok":     ready,
		"checks": checks,
	})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"apiBaseUrl": s.cfg.PublicAPIBaseURL,
	})
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := s.cfg.CORSOrigin
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
