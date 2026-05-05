package httpapi

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	defaultLicenseCredits       = 5
	defaultLicenseExpiresInDays = 30
	defaultMaxConcurrent        = 6
	imageModel                  = "gpt-image-2"
)

type licenseRow struct {
	ID               string
	KeyCiphertext    sql.NullString
	ServiceCode      string
	Credits          int
	MaxConcurrent    int
	Status           string
	ExpiresAt        sql.NullTime
	RedeemedAt       sql.NullTime
	RedeemedWalletID sql.NullString
	RedeemedAddress  sql.NullString
	Note             string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type adminLicenseListFilters struct {
	SearchHash  string
	ServiceCode string
	Redeemed    string
	Limit       int
	Offset      int
}

type serviceProfileRow struct {
	ID               string
	Label            string
	TierBucket       string
	APIBaseURL       string
	APIKeyCiphertext string
	Model            string
	APIMode          string
	CodexCLI         bool
	Priority         int
	MaxConcurrent    int
	Status           string
	SelectionCount   int
	SuccessCount     int
	FailureCount     int
	LastSelectedAt   sql.NullTime
	LastFailedAt     sql.NullTime
	DisabledUntil    sql.NullTime
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (s *Server) handleAdminCreateLicenses(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}

	var body struct {
		Count         int    `json:"count"`
		Credits       int    `json:"credits"`
		MaxConcurrent int    `json:"maxConcurrent"`
		ExpiresAt     string `json:"expiresAt"`
		ExpiresInDays int    `json:"expiresInDays"`
		Note          string `json:"note"`
		ServiceCode   string `json:"serviceCode"`
	}
	if !readJSON(w, r, &body) {
		return
	}

	count := minPositive(body.Count, 1, 100)
	serviceCode := normalizeServiceCode(body.ServiceCode)
	credits := positiveOr(body.Credits, defaultLicenseCredits)
	maxConcurrent := positiveOr(body.MaxConcurrent, defaultMaxConcurrent)
	expiresAt, err := parseExpiresAt(body.ExpiresAt, body.ExpiresInDays)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "过期时间格式无效")
		return
	}
	note := strings.TrimSpace(body.Note)
	now := time.Now().UTC()

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()

	keys := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		key, err := randomLicenseKey()
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		id, err := randomUUID()
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		keyCiphertext, err := s.encryptSecret(key)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		_, err = s.db.Exec(ctx, `
			INSERT INTO license_keys (
				id, key_hash, key_ciphertext, service_code, credits, max_concurrent,
				status, expires_at, note, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, 'active', $7, $8, $9, $10)
		`, id, s.hashSecret(key), keyCiphertext, serviceCode, credits, maxConcurrent, expiresAt, note, now, now)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		keys = append(keys, map[string]any{
			"id":            id,
			"key":           key,
			"serviceCode":   serviceCode,
			"credits":       credits,
			"maxConcurrent": maxConcurrent,
			"note":          note,
		})
	}

	writeJSON(w, http.StatusCreated, map[string]any{"keys": keys})
}

func (s *Server) handleAdminListLicenses(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}

	limit := minPositive(intFromString(r.URL.Query().Get("limit")), 20, 50)
	offset := intFromString(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	searchHash := ""
	if search != "" {
		searchHash = s.hashSecret(search)
	}
	serviceCode := adminLicenseServiceFilter(r.URL.Query().Get("serviceCode"))
	redeemed := adminLicenseRedeemedFilter(r.URL.Query().Get("redeemed"))
	queryLimit := limit + 1
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()

	query, args := adminLicenseListQuery(adminLicenseListFilters{
		SearchHash:  searchHash,
		ServiceCode: serviceCode,
		Redeemed:    redeemed,
		Limit:       queryLimit,
		Offset:      offset,
	})
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	licenses := []map[string]any{}
	seen := 0
	for rows.Next() {
		license, err := scanLicense(rows)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if seen < limit {
			licenses = append(licenses, s.publicLicense(license))
		}
		seen++
	}
	if err := rows.Err(); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"licenses": licenses,
		"page": map[string]any{
			"limit":       limit,
			"offset":      offset,
			"hasMore":     seen > limit,
			"search":      search,
			"serviceCode": serviceCode,
			"redeemed":    redeemed,
		},
	})
}

func adminLicenseListQuery(filters adminLicenseListFilters) (string, []any) {
	where := []string{"l.status <> 'deleted'"}
	args := []any{}
	if filters.SearchHash != "" {
		args = append(args, filters.SearchHash)
		where = append(where, fmt.Sprintf("l.key_hash = $%d", len(args)))
	}
	if filters.ServiceCode != "" {
		args = append(args, filters.ServiceCode)
		where = append(where, fmt.Sprintf("l.service_code = $%d", len(args)))
	}
	switch filters.Redeemed {
	case "used":
		where = append(where, "l.redeemed_wallet_id IS NOT NULL")
	case "unused":
		where = append(where, "l.redeemed_wallet_id IS NULL")
	}
	args = append(args, filters.Limit)
	limitPlaceholder := fmt.Sprintf("$%d", len(args))
	args = append(args, filters.Offset)
	offsetPlaceholder := fmt.Sprintf("$%d", len(args))
	query := `
			SELECT l.id, l.key_ciphertext, l.service_code, l.credits, l.max_concurrent,
				l.status, l.expires_at, l.redeemed_at, l.redeemed_wallet_id,
				w.address, l.note, l.created_at, l.updated_at
			FROM license_keys l
			LEFT JOIN wallets w ON w.id = l.redeemed_wallet_id
			WHERE ` + strings.Join(where, " AND ") + `
			ORDER BY l.created_at DESC
			LIMIT ` + limitPlaceholder + ` OFFSET ` + offsetPlaceholder
	return query, args
}

func adminLicenseServiceFilter(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case serviceCodeImage2SD, serviceCodeImage2HD:
		return value
	default:
		return ""
	}
}

func adminLicenseRedeemedFilter(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "used", "unused":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func (s *Server) handleAdminUpdateLicense(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}

	var body struct {
		Note string `json:"note"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		errorJSON(w, http.StatusBadRequest, "缺少兑换码 ID")
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	note := strings.TrimSpace(body.Note)
	tag, err := s.db.Exec(ctx, `UPDATE license_keys SET note = $1, updated_at = $2 WHERE id = $3`, note, time.Now().UTC(), id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		errorJSON(w, http.StatusNotFound, "兑换码不存在")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"id": id, "note": note})
}

func (s *Server) handleAdminDeleteLicenses(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}

	ids := []string{}
	if r.Method == http.MethodDelete {
		ids = append(ids, chi.URLParam(r, "id"))
	} else {
		var body struct {
			IDs []string `json:"ids"`
		}
		if !readJSON(w, r, &body) {
			return
		}
		ids = body.IDs
	}
	ids = uniqueTrimmed(ids)
	if len(ids) == 0 {
		errorJSON(w, http.StatusBadRequest, "请选择要删除的兑换码")
		return
	}

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	now := time.Now().UTC()
	deleted := int64(0)
	for _, id := range ids {
		tag, err := s.db.Exec(ctx, `UPDATE license_keys SET status = 'deleted', updated_at = $1 WHERE id = $2`, now, id)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		deleted += tag.RowsAffected()
	}

	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "ids": ids})
}

func (s *Server) handleAdminListServiceProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	rows, err := s.db.Query(ctx, `
		SELECT id, label, tier_bucket, api_base_url, api_key_ciphertext, model, api_mode,
			codex_cli, priority, max_concurrent, status, selection_count, success_count, failure_count,
			last_selected_at, last_failed_at, disabled_until, created_at, updated_at
		FROM service_profiles
		WHERE status <> 'deleted'
		ORDER BY tier_bucket, priority, id
	`)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	profiles := []map[string]any{}
	for rows.Next() {
		profile, err := scanServiceProfile(rows)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		profiles = append(profiles, publicServiceProfile(profile))
	}
	if err := rows.Err(); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"serviceProfiles": profiles})
}

func (s *Server) handleAdminUpsertServiceProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}

	var body struct {
		ID            string `json:"id"`
		Label         string `json:"label"`
		TierBucket    string `json:"tierBucket"`
		TierBucket2   string `json:"tier_bucket"`
		APIBaseURL    string `json:"apiBaseUrl"`
		APIKey        string `json:"apiKey"`
		Model         string `json:"model"`
		APIMode       string `json:"apiMode"`
		CodexCLI      *bool  `json:"codexCli"`
		Priority      int    `json:"priority"`
		MaxConcurrent *int   `json:"maxConcurrent"`
		Status        string `json:"status"`
	}
	if !readJSON(w, r, &body) {
		return
	}

	id := strings.TrimSpace(body.ID)
	tierBucket := normalizeTierBucket(firstNonEmpty(body.TierBucket, body.TierBucket2))
	apiBaseURL := strings.TrimSpace(body.APIBaseURL)
	apiKey := strings.TrimSpace(body.APIKey)
	requestedModel := strings.TrimSpace(body.Model)
	model := imageModel
	apiMode := normalizeAPIMode(body.APIMode)
	priority := positiveOr(body.Priority, 100)
	status := "active"
	if body.Status == "disabled" {
		status = "disabled"
	}
	var disabledUntil any
	if apiBaseURL == "" {
		errorJSON(w, http.StatusBadRequest, "Base URL 为必填")
		return
	}
	if requestedModel != "" && requestedModel != imageModel {
		errorJSON(w, http.StatusBadRequest, fmt.Sprintf("当前仅支持 %s", imageModel))
		return
	}
	if id == "" {
		generated, err := newServiceProfileID(tierBucket, apiMode)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		id = generated
	}
	label := strings.TrimSpace(body.Label)
	if label == "" {
		label = id
	}

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()

	existing, err := s.serviceProfileByID(ctx, id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	apiKeyCiphertext := ""
	if existing != nil {
		apiKeyCiphertext = existing.APIKeyCiphertext
	}
	codexCLI := false
	if existing != nil {
		codexCLI = existing.CodexCLI
	}
	if body.CodexCLI != nil {
		codexCLI = *body.CodexCLI
	}
	maxConcurrent := 0
	if existing != nil {
		maxConcurrent = existing.MaxConcurrent
	}
	if body.MaxConcurrent != nil {
		maxConcurrent = boundedInt(*body.MaxConcurrent, 0, 100, 0)
	}
	if apiKey != "" {
		apiKeyCiphertext, err = s.encryptSecret(apiKey)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if apiKeyCiphertext == "" {
		errorJSON(w, http.StatusBadRequest, "新增服务商时 apiKey 为必填")
		return
	}

	now := time.Now().UTC()
	createdAt := now
	if existing != nil {
		createdAt = existing.CreatedAt
	}
	normalizedBase := normalizeBaseURL(apiBaseURL)
	_, err = s.db.Exec(ctx, `
		INSERT INTO service_profiles (
			id, label, tier_bucket, api_base_url, api_key_ciphertext, model, api_mode,
			codex_cli, priority, max_concurrent, status, disabled_until, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT(id) DO UPDATE SET
			label = excluded.label,
			tier_bucket = excluded.tier_bucket,
			api_base_url = excluded.api_base_url,
			api_key_ciphertext = excluded.api_key_ciphertext,
			model = excluded.model,
			api_mode = excluded.api_mode,
			codex_cli = excluded.codex_cli,
			priority = excluded.priority,
			max_concurrent = excluded.max_concurrent,
			status = excluded.status,
			disabled_until = excluded.disabled_until,
			updated_at = excluded.updated_at
	`, id, label, tierBucket, normalizedBase, apiKeyCiphertext, model, apiMode, codexCLI, priority, maxConcurrent, status, disabledUntil, createdAt, now)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, publicServiceProfile(serviceProfileRow{
		ID:               id,
		Label:            label,
		TierBucket:       tierBucket,
		APIBaseURL:       normalizedBase,
		APIKeyCiphertext: apiKeyCiphertext,
		Model:            model,
		APIMode:          apiMode,
		CodexCLI:         codexCLI,
		Priority:         priority,
		MaxConcurrent:    maxConcurrent,
		Status:           status,
		CreatedAt:        createdAt,
		UpdatedAt:        now,
	}))
}

func (s *Server) handleAdminDeleteServiceProfile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		errorJSON(w, http.StatusBadRequest, "缺少服务商 ID")
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	var activeTasks int
	if err := s.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE service_profile = $1 AND status IN ('queued', 'running')
	`, id).Scan(&activeTasks); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if activeTasks > 0 {
		errorJSON(w, http.StatusConflict, "这个服务商仍有进行中的任务，不能删除")
		return
	}

	tag, err := s.db.Exec(ctx, `UPDATE service_profiles SET status = 'deleted', updated_at = $1 WHERE id = $2`, time.Now().UTC(), id)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		errorJSON(w, http.StatusNotFound, "服务商不存在")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "deleted": true})
}

func (s *Server) requireDatabase(w http.ResponseWriter) bool {
	if s.db == nil {
		errorJSON(w, http.StatusServiceUnavailable, "database is not configured")
		return false
	}
	return true
}

func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	expected := strings.TrimSpace(s.cfg.AdminToken)
	if expected == "" {
		errorJSON(w, http.StatusInternalServerError, "Admin token is not configured")
		return false
	}
	actual := bearerToken(r)
	if subtle.ConstantTimeCompare([]byte(actual), []byte(expected)) != 1 {
		errorJSON(w, http.StatusUnauthorized, "管理员认证失败")
		return false
	}
	return true
}

func (s *Server) serviceProfileByID(ctx context.Context, id string) (*serviceProfileRow, error) {
	row := s.db.QueryRow(ctx, `
		SELECT id, label, tier_bucket, api_base_url, api_key_ciphertext, model, api_mode,
			codex_cli, priority, max_concurrent, status, selection_count, success_count, failure_count,
			last_selected_at, last_failed_at, disabled_until, created_at, updated_at
		FROM service_profiles
		WHERE id = $1
	`, id)
	profile, err := scanServiceProfile(row)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanLicense(row scanner) (licenseRow, error) {
	var license licenseRow
	err := row.Scan(
		&license.ID,
		&license.KeyCiphertext,
		&license.ServiceCode,
		&license.Credits,
		&license.MaxConcurrent,
		&license.Status,
		&license.ExpiresAt,
		&license.RedeemedAt,
		&license.RedeemedWalletID,
		&license.RedeemedAddress,
		&license.Note,
		&license.CreatedAt,
		&license.UpdatedAt,
	)
	return license, err
}

func scanServiceProfile(row scanner) (serviceProfileRow, error) {
	var profile serviceProfileRow
	err := row.Scan(
		&profile.ID,
		&profile.Label,
		&profile.TierBucket,
		&profile.APIBaseURL,
		&profile.APIKeyCiphertext,
		&profile.Model,
		&profile.APIMode,
		&profile.CodexCLI,
		&profile.Priority,
		&profile.MaxConcurrent,
		&profile.Status,
		&profile.SelectionCount,
		&profile.SuccessCount,
		&profile.FailureCount,
		&profile.LastSelectedAt,
		&profile.LastFailedAt,
		&profile.DisabledUntil,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	return profile, err
}

func (s *Server) publicLicense(license licenseRow) map[string]any {
	key := any(nil)
	if license.KeyCiphertext.Valid && license.KeyCiphertext.String != "" {
		if plaintext, err := s.decryptSecret(license.KeyCiphertext.String); err == nil {
			key = plaintext
		}
	}
	return map[string]any{
		"id":                    license.ID,
		"key":                   key,
		"serviceCode":           normalizeServiceCode(license.ServiceCode),
		"credits":               license.Credits,
		"max_concurrent":        license.MaxConcurrent,
		"status":                license.Status,
		"expires_at":            nullableTime(license.ExpiresAt),
		"redeemed_at":           nullableTime(license.RedeemedAt),
		"redeemedWalletId":      nullableString(license.RedeemedWalletID),
		"redeemedWalletAddress": nullableString(license.RedeemedAddress),
		"note":                  license.Note,
		"created_at":            license.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":            license.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func publicServiceProfile(profile serviceProfileRow) map[string]any {
	return map[string]any{
		"id":             profile.ID,
		"label":          profile.Label,
		"tierBucket":     profile.TierBucket,
		"apiBaseUrl":     profile.APIBaseURL,
		"model":          profile.Model,
		"apiMode":        profile.APIMode,
		"codexCli":       profile.CodexCLI,
		"priority":       profile.Priority,
		"maxConcurrent":  profile.MaxConcurrent,
		"status":         profile.Status,
		"hasApiKey":      profile.APIKeyCiphertext != "",
		"selectionCount": profile.SelectionCount,
		"successCount":   profile.SuccessCount,
		"failureCount":   profile.FailureCount,
		"lastSelectedAt": nullableTime(profile.LastSelectedAt),
		"lastFailedAt":   nullableTime(profile.LastFailedAt),
		"disabledUntil":  nullableTime(profile.DisabledUntil),
		"createdAt":      profile.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt":      profile.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func readJSON(w http.ResponseWriter, r *http.Request, dest any) bool {
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dest); err != nil {
		errorJSON(w, http.StatusBadRequest, "Invalid JSON body")
		return false
	}
	return true
}

func errorJSON(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"message": message}})
}

func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	fields := strings.Fields(value)
	if len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
		return strings.TrimSpace(fields[1])
	}
	return ""
}

func (s *Server) hashSecret(value string) string {
	if s.hashSecretFunc != nil {
		return s.hashSecretFunc(value)
	}
	sum := sha256.Sum256([]byte(s.cfg.LicenseKeyPepper + ":" + strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func (s *Server) encryptSecret(value string) (string, error) {
	aead, err := s.credentialAEAD()
	if err != nil {
		return "", err
	}
	iv, err := randomBytes(12)
	if err != nil {
		return "", err
	}
	ciphertext := aead.Seal(nil, iv, []byte(value), nil)
	encoding := base64.RawURLEncoding
	return "v1." + encoding.EncodeToString(iv) + "." + encoding.EncodeToString(ciphertext), nil
}

func (s *Server) decryptSecret(value string) (string, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 || parts[0] != "v1" {
		return "", fmt.Errorf("secret is not configured")
	}
	encoding := base64.RawURLEncoding
	iv, err := encoding.DecodeString(parts[1])
	if err != nil {
		return "", err
	}
	ciphertext, err := encoding.DecodeString(parts[2])
	if err != nil {
		return "", err
	}
	aead, err := s.credentialAEAD()
	if err != nil {
		return "", err
	}
	plaintext, err := aead.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("密钥解密失败：PROVIDER_CREDENTIAL_SECRET 与写入数据库时不一致。请恢复原 secret，或在后台重新保存服务商 API Key")
	}
	return string(plaintext), nil
}

func (s *Server) credentialAEAD() (cipher.AEAD, error) {
	secret := strings.TrimSpace(s.cfg.ProviderSecret)
	if secret == "" {
		return nil, fmt.Errorf("Provider credential encryption secret is not configured")
	}
	sum := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func randomBytes(size int) ([]byte, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}

func randomToken(prefix string, byteLength int) (string, error) {
	bytes, err := randomBytes(byteLength)
	if err != nil {
		return "", err
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(bytes), nil
}

func randomLicenseKey() (string, error) {
	bytes, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	raw := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes)
	if len(raw) < 20 {
		return "", fmt.Errorf("random license entropy was too short")
	}
	raw = raw[:20]
	return fmt.Sprintf("LIL-%s-%s-%s-%s", raw[:5], raw[5:10], raw[10:15], raw[15:20]), nil
}

func randomUUID() (string, error) {
	bytes, err := randomBytes(16)
	if err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}

func newServiceProfileID(tierBucket, apiMode string) (string, error) {
	id, err := randomUUID()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("provider-%s-%s-%s", tierBucket, apiMode, strings.ReplaceAll(id[:8], "-", "")), nil
}

func parseExpiresAt(value string, days int) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		if parsed, err := time.Parse(time.RFC3339, value); err == nil {
			return parsed.UTC(), nil
		}
		if parsed, err := time.Parse("2006-01-02", value); err == nil {
			return parsed.UTC(), nil
		}
		return time.Time{}, fmt.Errorf("invalid expiresAt")
	}
	if days <= 0 {
		days = defaultLicenseExpiresInDays
	}
	return time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour), nil
}

func nullableTime(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time.UTC().Format(time.RFC3339Nano)
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func minPositive(value, fallback, max int) int {
	value = positiveOr(value, fallback)
	if value > max {
		return max
	}
	return value
}

func intFromString(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	var parsed int
	_, _ = fmt.Sscanf(value, "%d", &parsed)
	return parsed
}

func uniqueTrimmed(values []string) []string {
	seen := map[string]struct{}{}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeTierBucket(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "hd") {
		return "hd"
	}
	return "1k"
}

func normalizeServiceCode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case serviceCodeImage2HD:
		return serviceCodeImage2HD
	default:
		return serviceCodeImage2SD
	}
}

func normalizeAPIMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ohmytoken", "responses":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "images"
	}
}

func normalizeBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" || strings.HasSuffix(value, "/v1") {
		return value
	}
	return value + "/v1"
}
