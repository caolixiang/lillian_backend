package httpapi

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/CookSleep/lillian_backend/internal/config"
	"github.com/CookSleep/lillian_backend/internal/storage"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	cfg            config.Config
	db             *pgxpool.Pool
	wallets        walletStore
	store          storage.ObjectStore
	logger         *log.Logger
	upstreamClient *http.Client
	hashSecretFunc func(string) string
}

func New(cfg config.Config, db *pgxpool.Pool, store storage.ObjectStore, logger *log.Logger) *Server {
	server := &Server{
		cfg:            cfg,
		db:             db,
		store:          store,
		logger:         logger,
		upstreamClient: newUpstreamHTTPClient(),
	}
	if db != nil {
		server.wallets = postgresWalletStore{db: db}
	}
	return server
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(s.cors)
	r.Get("/health", s.handleHealth)
	r.Get("/ready", s.handleReady)
	r.Get("/config.json", s.handleConfig)
	r.Get("/admin", s.handleAdminPage)
	r.Head("/admin", s.handleAdminPage)
	r.Get("/admin/", s.handleAdminPage)
	r.Head("/admin/", s.handleAdminPage)
	r.Get("/admin/index.html", s.handleAdminPage)
	r.Head("/admin/index.html", s.handleAdminPage)
	r.Get("/admin/assets/*", s.handleAdminAsset)
	r.Head("/admin/assets/*", s.handleAdminAsset)
	r.Get("/admin/lillian-icon.svg", s.handleIcon)
	r.Head("/admin/lillian-icon.svg", s.handleIcon)
	r.Get("/lillian-icon.svg", s.handleIcon)
	r.Head("/lillian-icon.svg", s.handleIcon)
	r.Post("/api/keys/activate", s.handleActivateLicense)
	r.Get("/api/me/credits", s.handleCredits)
	r.Post("/api/wallets/create", s.handleCreateWallet)
	r.Post("/api/wallets/restore", s.handleRestoreWallet)
	r.Post("/api/wallets/redeem", s.handleRedeemWallet)
	r.Get("/api/wallets/{address}", s.handleGetWallet)
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
	r.Get("/admin/runtime-settings", s.handleAdminGetRuntimeSettings)
	r.Post("/admin/runtime-settings", s.handleAdminUpdateRuntimeSettings)
	r.Patch("/admin/runtime-settings", s.handleAdminUpdateRuntimeSettings)
	return r
}

func newUpstreamHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 128
	transport.MaxIdleConnsPerHost = 32
	transport.MaxConnsPerHost = 0
	transport.IdleConnTimeout = 90 * time.Second
	transport.TLSHandshakeTimeout = 10 * time.Second
	transport.ExpectContinueTimeout = time.Second
	return &http.Client{Transport: transport}
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
		"apiBaseUrl": s.publicBaseURL(r),
	})
}

func (s *Server) publicBaseURL(r *http.Request) string {
	if base := strings.TrimRight(s.cfg.PublicAPIBaseURL, "/"); base != "" {
		return base
	}

	proto := firstHeaderValue(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}

	host := firstHeaderValue(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return ""
	}
	return proto + "://" + host
}

func firstHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if before, _, ok := strings.Cut(value, ","); ok {
		value = before
	}
	return strings.TrimSpace(value)
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
