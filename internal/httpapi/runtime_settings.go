package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	settingImageGlobalConcurrency          = "image_global_concurrency"
	settingImageProviderDefaultConcurrency = "image_provider_default_concurrency"
	settingUpstreamTimeoutSeconds          = "upstream_timeout_seconds"

	defaultImageGlobalConcurrency          = 6
	defaultImageProviderDefaultConcurrency = 2
	defaultUpstreamTimeoutSeconds          = 600
)

type runtimeSettings struct {
	ImageGlobalConcurrency          int
	ImageProviderDefaultConcurrency int
	UpstreamTimeoutSeconds          int
}

func defaultRuntimeSettings() runtimeSettings {
	return runtimeSettings{
		ImageGlobalConcurrency:          defaultImageGlobalConcurrency,
		ImageProviderDefaultConcurrency: defaultImageProviderDefaultConcurrency,
		UpstreamTimeoutSeconds:          defaultUpstreamTimeoutSeconds,
	}
}

func (s *Server) loadRuntimeSettings(ctx context.Context) runtimeSettings {
	settings := defaultRuntimeSettings()
	if s == nil || s.db == nil {
		return settings
	}
	rows, err := s.db.Query(ctx, `
		SELECT key, value
		FROM app_settings
		WHERE key IN (
			'image_global_concurrency',
			'image_provider_default_concurrency',
			'upstream_timeout_seconds'
		)
	`)
	if err != nil {
		if s.logger != nil {
			s.logger.Printf("load runtime settings failed: %v", err)
		}
		return settings
	}
	defer rows.Close()

	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			continue
		}
		switch key {
		case settingImageGlobalConcurrency:
			settings.ImageGlobalConcurrency = boundedInt(parsed, 1, 100, defaultImageGlobalConcurrency)
		case settingImageProviderDefaultConcurrency:
			settings.ImageProviderDefaultConcurrency = boundedInt(parsed, 1, 100, defaultImageProviderDefaultConcurrency)
		case settingUpstreamTimeoutSeconds:
			settings.UpstreamTimeoutSeconds = boundedInt(parsed, 60, 1800, defaultUpstreamTimeoutSeconds)
		}
	}
	return settings
}

func (s *Server) handleAdminGetRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]any{"settings": publicRuntimeSettings(s.loadRuntimeSettings(ctx))})
}

func (s *Server) handleAdminUpdateRuntimeSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}
	var body struct {
		ImageGlobalConcurrency          int `json:"imageGlobalConcurrency"`
		ImageProviderDefaultConcurrency int `json:"imageProviderDefaultConcurrency"`
		UpstreamTimeoutSeconds          int `json:"upstreamTimeoutSeconds"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	settings := runtimeSettings{
		ImageGlobalConcurrency:          boundedInt(body.ImageGlobalConcurrency, 1, 100, defaultImageGlobalConcurrency),
		ImageProviderDefaultConcurrency: boundedInt(body.ImageProviderDefaultConcurrency, 1, 100, defaultImageProviderDefaultConcurrency),
		UpstreamTimeoutSeconds:          boundedInt(body.UpstreamTimeoutSeconds, 60, 1800, defaultUpstreamTimeoutSeconds),
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	entries := []struct {
		key         string
		value       int
		description string
	}{
		{settingImageGlobalConcurrency, settings.ImageGlobalConcurrency, "Maximum number of image generation tasks running upstream at the same time."},
		{settingImageProviderDefaultConcurrency, settings.ImageProviderDefaultConcurrency, "Default per-provider upstream concurrency when a provider does not override it."},
		{settingUpstreamTimeoutSeconds, settings.UpstreamTimeoutSeconds, "Timeout in seconds for one synchronous upstream image call and image retrieval."},
	}
	for _, entry := range entries {
		if _, err := s.db.Exec(ctx, `
			INSERT INTO app_settings (key, value, description, updated_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (key) DO UPDATE SET
				value = excluded.value,
				description = excluded.description,
				updated_at = excluded.updated_at
		`, entry.key, strconv.Itoa(entry.value), entry.description, now); err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"settings": publicRuntimeSettings(settings)})
}

func publicRuntimeSettings(settings runtimeSettings) map[string]any {
	return map[string]any{
		"imageGlobalConcurrency":          settings.ImageGlobalConcurrency,
		"imageProviderDefaultConcurrency": settings.ImageProviderDefaultConcurrency,
		"upstreamTimeoutSeconds":          settings.UpstreamTimeoutSeconds,
	}
}

func boundedInt(value, min, max, fallback int) int {
	if value <= 0 {
		value = fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
