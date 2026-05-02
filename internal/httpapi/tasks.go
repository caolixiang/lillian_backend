package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	defaultImageSize               = "1024x1824"
	defaultSizeTier                = "1K"
	serviceProfileCooldownFailures = 2
	serviceProfileCooldown         = 5 * time.Minute
	serviceProfileFallbackRetries  = 1
	taskStaleRecoveryGrace         = 2 * time.Minute
	taskWorkerSlots                = 32
	taskClaimAdvisoryLockKey       = 741190203
)

type taskPayload struct {
	Prompt             string         `json:"prompt"`
	WalletAddress      string         `json:"walletAddress"`
	Params             map[string]any `json:"params"`
	InputImageDataURLs []string       `json:"inputImageDataUrls"`
	MaskDataURL        string         `json:"maskDataUrl,omitempty"`
}

type taskRow struct {
	ID                 string
	WalletID           string
	ServiceCode        string
	CreditReserved     bool
	CreditCharged      bool
	Status             string
	RequestedSize      string
	ServiceProfile     string
	RequestJSON        []byte
	OutputsJSON        sql.NullString
	ActualParamsJSON   sql.NullString
	RevisedPromptsJSON sql.NullString
	Error              sql.NullString
	CreatedAt          time.Time
	UpdatedAt          time.Time
	FinishedAt         sql.NullTime
}

type taskOutput struct {
	Key         string `json:"key"`
	ContentType string `json:"contentType"`
}

type imageResult struct {
	Bytes       []byte
	ContentType string
}

type imageServiceResult struct {
	Images         []imageResult
	ActualParams   map[string]any
	RevisedPrompts []string
}

func (s *Server) StartTaskWorkers(ctx context.Context) {
	if s == nil || s.db == nil {
		return
	}
	s.recoverStaleWalletRunningTasks(ctx)
	concurrency := s.taskWorkerConcurrency()
	for i := 0; i < concurrency; i++ {
		go s.taskWorkerLoop(ctx)
	}
}

func (s *Server) taskWorkerConcurrency() int {
	if s == nil || s.cfg.TaskWorkerConcurrency <= 0 {
		return taskWorkerSlots
	}
	return s.cfg.TaskWorkerConcurrency
}

func (s *Server) taskWorkerLoop(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			processed := s.processNextQueuedTask(ctx)
			delay := s.cfg.TaskPollInterval
			if processed {
				delay = 100 * time.Millisecond
			}
			if delay <= 0 {
				delay = 2 * time.Second
			}
			timer.Reset(delay)
		}
	}
}

func (s *Server) recoverStaleWalletRunningTasks(ctx context.Context) {
	if s == nil || s.db == nil {
		return
	}
	recoveryCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	settings := s.loadRuntimeSettings(recoveryCtx)
	staleAfter := time.Duration(settings.UpstreamTimeoutSeconds)*time.Second + taskStaleRecoveryGrace
	tx, err := s.db.Begin(recoveryCtx)
	if err != nil {
		if s.logger != nil {
			s.logger.Printf("stale wallet task recovery failed: %v", err)
		}
		return
	}
	defer tx.Rollback(recoveryCtx)
	recovered, err := recoverStaleWalletRunningTasks(recoveryCtx, staleTaskTx{tx: tx}, time.Now().UTC(), staleAfter)
	if err != nil {
		if s.logger != nil {
			s.logger.Printf("stale wallet task recovery failed: %v", err)
		}
		return
	}
	if err := tx.Commit(recoveryCtx); err != nil {
		if s.logger != nil {
			s.logger.Printf("stale wallet task recovery commit failed: %v", err)
		}
		return
	}
	if recovered > 0 && s.logger != nil {
		s.logger.Printf("recovered %d stale wallet tasks", recovered)
	}
}

type staleTaskStore interface {
	Query(ctx context.Context, sql string, args ...any) (taskRows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type staleTaskTx struct {
	tx pgx.Tx
}

func (s staleTaskTx) Query(ctx context.Context, sql string, args ...any) (taskRows, error) {
	return s.tx.Query(ctx, sql, args...)
}

func (s staleTaskTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return s.tx.Exec(ctx, sql, args...)
}

type taskRows interface {
	Close()
	Err() error
	Next() bool
	Scan(dest ...any) error
}

func recoverStaleWalletRunningTasks(ctx context.Context, store staleTaskStore, now time.Time, staleAfter time.Duration) (int, error) {
	if staleAfter <= 0 {
		staleAfter = time.Duration(defaultUpstreamTimeoutSeconds)*time.Second + taskStaleRecoveryGrace
	}
	cutoff := now.UTC().Add(-staleAfter)
	rows, err := store.Query(ctx, `
		UPDATE tasks
		SET status = 'error',
			error = $1,
			credit_reserved = false,
			updated_at = $2,
			finished_at = $2
		WHERE status = 'running'
			AND wallet_id IS NOT NULL
			AND service_code IS NOT NULL
			AND credit_reserved = true
			AND credit_charged = false
			AND updated_at < $3
		RETURNING wallet_id, service_code
	`, "Task timed out before completion and reserved wallet credit was released", now, cutoff)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type entitlementKey struct {
		walletID    string
		serviceCode string
	}
	refunds := map[entitlementKey]int{}
	for rows.Next() {
		var key entitlementKey
		if err := rows.Scan(&key.walletID, &key.serviceCode); err != nil {
			return 0, err
		}
		refunds[key]++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	recovered := 0
	for key, count := range refunds {
		tag, err := store.Exec(ctx, `
			UPDATE wallet_entitlements
			SET remaining = remaining + $1, updated_at = $2
			WHERE wallet_id = $3 AND service_code = $4
		`, count, now, key.walletID, key.serviceCode)
		if err != nil {
			return recovered, err
		}
		if tag.RowsAffected() != 1 {
			return recovered, fmt.Errorf("wallet entitlement missing for stale task recovery: wallet_id=%s service_code=%s", key.walletID, key.serviceCode)
		}
		recovered += count
	}
	return recovered, nil
}

func (s *Server) processNextQueuedTask(ctx context.Context) bool {
	taskCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	settings := s.loadRuntimeSettings(taskCtx)
	task, ok, err := s.claimNextQueuedTask(taskCtx, settings)
	if err != nil {
		if s.logger != nil {
			s.logger.Printf("task worker claim failed: %v", err)
		}
		return false
	}
	if !ok {
		return false
	}
	if err := s.executeTask(ctx, task); err != nil && s.logger != nil {
		s.logger.Printf("task %s failed: %v", task.ID, err)
	}
	return true
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	if !s.requireDatabase(w) {
		return
	}
	if s.store == nil {
		errorJSON(w, http.StatusServiceUnavailable, "object store is not configured")
		return
	}

	var body taskPayload
	if !readJSON(w, r, &body) {
		return
	}
	body.Prompt = strings.TrimSpace(body.Prompt)
	if body.Prompt == "" {
		errorJSON(w, http.StatusBadRequest, "请输入提示词")
		return
	}
	if body.Params == nil {
		body.Params = map[string]any{}
	}
	walletAddress := strings.ToLower(strings.TrimSpace(body.WalletAddress))
	if walletAddress == "" {
		walletAddress = strings.ToLower(strings.TrimSpace(stringParam(body.Params, "walletAddress")))
	}
	if !isWalletAddress(walletAddress) {
		errorJSON(w, http.StatusBadRequest, "钱包地址格式无效")
		return
	}
	body.WalletAddress = walletAddress
	requestedSize, requestedSizeTier := normalizeTaskSize(body.Params)
	body.Params["size"] = requestedSize
	body.Params["output_format"] = "png"
	body.Params["output_compression"] = nil
	if requestedSizeTier != "" {
		body.Params["size_tier"] = requestedSizeTier
	}

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()

	wallet, err := s.wallets.WalletByAddress(ctx, walletAddress)
	if errors.Is(err, errWalletNotFound) {
		errorJSON(w, http.StatusNotFound, "钱包不存在")
		return
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	entitlement, err := selectWalletServiceForGeneration(wallet.Entitlements, requestedSizeTier)
	if err != nil {
		errorJSON(w, http.StatusForbidden, err.Error())
		return
	}
	profile, err := s.selectServiceProfileForWallet(ctx, entitlement.ServiceCode, requestedSize, requestedSizeTier, nil)
	if err != nil {
		status := http.StatusServiceUnavailable
		if strings.Contains(err.Error(), "不支持") {
			status = http.StatusForbidden
		}
		errorJSON(w, status, err.Error())
		return
	}

	requestJSON, err := json.Marshal(body)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "任务参数无效")
		return
	}
	taskID, err := randomUUID()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	now := time.Now().UTC()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx)

	var lockedRemaining int
	var lockedMaxConcurrent int
	if err := tx.QueryRow(ctx, `
		SELECT remaining, max_concurrent
		FROM wallet_entitlements
		WHERE wallet_id = $1 AND service_code = $2
		FOR UPDATE
	`, wallet.Wallet.ID, entitlement.ServiceCode).Scan(&lockedRemaining, &lockedMaxConcurrent); err != nil {
		if errorsIsNoRows(err) {
			errorJSON(w, http.StatusForbidden, "钱包权益次数不足")
			return
		}
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if lockedRemaining <= 0 {
		errorJSON(w, http.StatusForbidden, "钱包权益次数不足")
		return
	}
	if lockedMaxConcurrent <= 0 {
		lockedMaxConcurrent = defaultMaxConcurrent
	}

	var activeTasks int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE wallet_id = $1 AND service_code = $2 AND status IN ('queued', 'running')
	`, wallet.Wallet.ID, entitlement.ServiceCode).Scan(&activeTasks); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if activeTasks >= lockedMaxConcurrent {
		errorJSON(w, http.StatusTooManyRequests, "并发任务已达上限")
		return
	}
	tag, err := tx.Exec(ctx, `
		UPDATE wallet_entitlements
		SET remaining = remaining - 1, updated_at = $1
		WHERE wallet_id = $2 AND service_code = $3
	`, now, wallet.Wallet.ID, entitlement.ServiceCode)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() != 1 {
		errorJSON(w, http.StatusForbidden, "钱包权益次数不足")
		return
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO tasks (
			id, wallet_id, service_code, credit_reserved, credit_charged,
			status, requested_size, service_profile, request_json, created_at, updated_at
		) VALUES ($1, $2, $3, true, false, 'queued', $4, $5, $6, $7, $8)
	`, taskID, wallet.Wallet.ID, entitlement.ServiceCode, requestedSize, profile.ID, requestJSON, now, now)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":               taskID,
		"status":           "queued",
		"wallet":           publicWallet(walletSnapshot{Wallet: wallet.Wallet, Entitlements: replaceEntitlementRemaining(wallet.Entitlements, entitlement.ServiceCode, lockedRemaining-1)}),
		"serviceCode":      entitlement.ServiceCode,
		"remainingCredits": maxInt(0, lockedRemaining-1),
	})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	wallet, ok := s.walletFromRequest(w, r)
	if !ok {
		return
	}
	task, ok := s.taskForWallet(w, r, chi.URLParam(r, "id"), wallet.Wallet.ID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.publicTask(r, task, wallet))
}

func (s *Server) handleGetTaskImage(w http.ResponseWriter, r *http.Request) {
	wallet, ok := s.walletFromRequest(w, r)
	if !ok {
		return
	}
	if s.store == nil {
		errorJSON(w, http.StatusServiceUnavailable, "object store is not configured")
		return
	}
	task, ok := s.taskForWallet(w, r, chi.URLParam(r, "id"), wallet.Wallet.ID)
	if !ok {
		return
	}
	if task.Status != "done" {
		errorJSON(w, http.StatusNotFound, "图片不存在")
		return
	}
	outputs := decodeOutputs(task.OutputsJSON)
	index, err := strconv.Atoi(chi.URLParam(r, "index"))
	if err != nil || index < 0 || index >= len(outputs) {
		errorJSON(w, http.StatusNotFound, "图片不存在")
		return
	}
	output := outputs[index]
	ctx, cancel := contextWithTimeout(r, 30*time.Second)
	defer cancel()
	body, contentType, err := s.store.Get(ctx, output.Key)
	if err != nil {
		errorJSON(w, http.StatusNotFound, "图片不存在")
		return
	}
	defer body.Close()
	if output.ContentType != "" {
		contentType = output.ContentType
	}
	if contentType == "" {
		contentType = "image/png"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = io.Copy(w, body)
}

func (s *Server) walletFromRequest(w http.ResponseWriter, r *http.Request) (walletSnapshot, bool) {
	if !s.requireWalletStore(w) {
		return walletSnapshot{}, false
	}
	address := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("walletAddress")))
	if address == "" {
		address = strings.ToLower(strings.TrimSpace(r.Header.Get("X-Wallet-Address")))
	}
	if !isWalletAddress(address) {
		errorJSON(w, http.StatusBadRequest, "钱包地址格式无效")
		return walletSnapshot{}, false
	}
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	snapshot, err := s.wallets.WalletByAddress(ctx, address)
	if errors.Is(err, errWalletNotFound) {
		errorJSON(w, http.StatusNotFound, "钱包不存在")
		return walletSnapshot{}, false
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return walletSnapshot{}, false
	}
	return snapshot, true
}

func (s *Server) taskForWallet(w http.ResponseWriter, r *http.Request, taskID, walletID string) (taskRow, bool) {
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	task, err := s.taskByIDForWallet(ctx, taskID, walletID)
	if errorsIsNoRows(err) {
		errorJSON(w, http.StatusNotFound, "任务不存在")
		return taskRow{}, false
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return taskRow{}, false
	}
	return task, true
}

func (s *Server) claimNextQueuedTask(ctx context.Context, settings runtimeSettings) (taskRow, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return taskRow{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, taskClaimAdvisoryLockKey); err != nil {
		return taskRow{}, false, err
	}

	if ok, err := s.globalTaskSlotAvailable(ctx, tx, settings); err != nil || !ok {
		return taskRow{}, false, err
	}

	task, err := scanTaskRow(tx.QueryRow(ctx, `
		SELECT t.id, COALESCE(t.wallet_id, ''), COALESCE(t.service_code, ''),
			t.credit_reserved, t.credit_charged, t.status, t.requested_size,
			t.service_profile, t.request_json, t.outputs_json, t.actual_params_json,
			t.revised_prompts_json, t.error, t.created_at, t.updated_at, t.finished_at
		FROM tasks t
		JOIN service_profiles sp ON sp.id = t.service_profile
		WHERE t.status = 'queued'
			AND (
				SELECT COUNT(*)
				FROM tasks running
				WHERE running.status = 'running'
					AND running.service_profile = t.service_profile
			) < CASE WHEN sp.max_concurrent > 0 THEN sp.max_concurrent ELSE $1 END
		ORDER BY t.created_at ASC
		LIMIT 1
		FOR UPDATE OF t SKIP LOCKED
	`, settings.ImageProviderDefaultConcurrency))
	if errorsIsNoRows(err) {
		return taskRow{}, false, nil
	}
	if err != nil {
		return taskRow{}, false, err
	}
	if err := markTaskRunning(ctx, tx, &task); err != nil {
		return taskRow{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return taskRow{}, false, err
	}
	return task, true, nil
}

func (s *Server) claimQueuedTaskByID(ctx context.Context, taskID string, settings runtimeSettings) (taskRow, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return taskRow{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, taskClaimAdvisoryLockKey); err != nil {
		return taskRow{}, false, err
	}

	if ok, err := s.globalTaskSlotAvailable(ctx, tx, settings); err != nil || !ok {
		return taskRow{}, false, err
	}

	task, err := scanTaskRow(tx.QueryRow(ctx, `
		SELECT t.id, COALESCE(t.wallet_id, ''), COALESCE(t.service_code, ''),
			t.credit_reserved, t.credit_charged, t.status, t.requested_size,
			t.service_profile, t.request_json, t.outputs_json, t.actual_params_json,
			t.revised_prompts_json, t.error, t.created_at, t.updated_at, t.finished_at
		FROM tasks t
		JOIN service_profiles sp ON sp.id = t.service_profile
		WHERE t.id = $2
			AND t.status = 'queued'
			AND (
				SELECT COUNT(*)
				FROM tasks running
				WHERE running.status = 'running'
					AND running.service_profile = t.service_profile
			) < CASE WHEN sp.max_concurrent > 0 THEN sp.max_concurrent ELSE $1 END
		LIMIT 1
		FOR UPDATE OF t SKIP LOCKED
	`, settings.ImageProviderDefaultConcurrency, taskID))
	if errorsIsNoRows(err) {
		return taskRow{}, false, nil
	}
	if err != nil {
		return taskRow{}, false, err
	}
	if err := markTaskRunning(ctx, tx, &task); err != nil {
		return taskRow{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return taskRow{}, false, err
	}
	return task, true, nil
}

func (s *Server) globalTaskSlotAvailable(ctx context.Context, tx pgx.Tx, settings runtimeSettings) (bool, error) {
	var running int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE status = 'running'`).Scan(&running); err != nil {
		return false, err
	}
	return running < settings.ImageGlobalConcurrency, nil
}

func markTaskRunning(ctx context.Context, tx pgx.Tx, task *taskRow) error {
	now := time.Now().UTC()
	tag, err := tx.Exec(ctx, `
		UPDATE tasks
		SET status = 'running', updated_at = $1
		WHERE id = $2 AND status = 'queued'
	`, now, task.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	task.Status = "running"
	task.UpdatedAt = now
	return nil
}

func (s *Server) processTask(ctx context.Context, taskID string) error {
	settings := s.loadRuntimeSettings(ctx)
	task, ok, err := s.claimQueuedTaskByID(ctx, taskID, settings)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return s.executeTask(ctx, task)
}

func (s *Server) executeTask(ctx context.Context, task taskRow) error {
	if task.Status != "running" {
		return nil
	}

	var payload taskPayload
	if err := json.Unmarshal(task.RequestJSON, &payload); err != nil {
		s.failTaskAndRefund(ctx, task, "Task request payload is invalid")
		return err
	}

	profile, err := s.serviceProfileByID(ctx, task.ServiceProfile)
	if err != nil {
		s.failTaskAndRefund(ctx, task, err.Error())
		return err
	}

	var result imageServiceResult
	tried := []string{}
	providerErrors := []string{}
	for attempt := 0; attempt <= serviceProfileFallbackRetries; attempt++ {
		tried = append(tried, profile.ID)
		result, err = s.callImageService(ctx, *profile, payload)
		if err == nil {
			_ = s.recordServiceProfileSuccess(ctx, profile.ID)
			break
		}
		_ = s.recordServiceProfileFailure(ctx, *profile)
		providerErrors = append(providerErrors, fmt.Sprintf("%s: %s", profile.Label, err.Error()))
		if attempt >= serviceProfileFallbackRetries {
			break
		}
		next, selectErr := s.selectServiceProfileForWallet(ctx, task.ServiceCode, task.RequestedSize, stringParam(payload.Params, "size_tier"), tried)
		if selectErr != nil {
			break
		}
		profile = &next
		_, _ = s.db.Exec(ctx, `UPDATE tasks SET service_profile = $1, updated_at = $2 WHERE id = $3`, profile.ID, time.Now().UTC(), task.ID)
		task.ServiceProfile = profile.ID
	}
	if err != nil {
		message := strings.Join(providerErrors, "；")
		if message == "" {
			message = err.Error()
		}
		s.failTaskAndRefund(ctx, task, message)
		return err
	}

	outputs := make([]taskOutput, 0, len(result.Images))
	for i, image := range result.Images {
		key := fmt.Sprintf("tasks/%s/outputs/%d.%s", task.ID, i, imageExt(image.ContentType))
		if _, err := s.store.PutBytes(ctx, key, image.Bytes, image.ContentType); err != nil {
			s.failTaskAndRefund(ctx, task, err.Error())
			return err
		}
		outputs = append(outputs, taskOutput{Key: key, ContentType: image.ContentType})
	}
	outputsJSON, _ := json.Marshal(outputs)
	actualParamsJSON, _ := json.Marshal(result.ActualParams)
	revisedPromptsJSON, _ := json.Marshal(result.RevisedPrompts)
	finishedAt := time.Now().UTC()
	_, err = s.db.Exec(ctx, `
		UPDATE tasks
		SET status = 'done',
			credit_charged = true,
			outputs_json = $1,
			actual_params_json = $2,
			revised_prompts_json = $3,
			updated_at = $4,
			finished_at = $5
		WHERE id = $6
	`, string(outputsJSON), string(actualParamsJSON), string(revisedPromptsJSON), finishedAt, finishedAt, task.ID)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) callImageService(ctx context.Context, profile serviceProfileRow, payload taskPayload) (imageServiceResult, error) {
	apiKey, err := s.decryptSecret(profile.APIKeyCiphertext)
	if err != nil {
		return imageServiceResult{}, err
	}
	settings := s.loadRuntimeSettings(ctx)
	requestCtx, cancel := context.WithTimeout(ctx, time.Duration(settings.UpstreamTimeoutSeconds)*time.Second)
	defer cancel()
	requestSize, _ := normalizeTaskSize(payload.Params)
	outputFormat := "png"
	fallbackMime := imageMime(outputFormat)
	isEdit := len(payload.InputImageDataURLs) > 0
	aspectRatio := aspectRatioFromSize(requestSize)
	client := s.upstreamClient
	if client == nil {
		client = newUpstreamHTTPClient()
	}

	var resp *http.Response
	if isEdit {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		_ = writer.WriteField("model", profile.Model)
		_ = writer.WriteField("prompt", payload.Prompt)
		_ = writer.WriteField("size", requestSize)
		if profile.APIMode == "ohmytoken" {
			if aspectRatio != "" {
				_ = writer.WriteField("aspect_ratio", aspectRatio)
			}
		} else {
			_ = writer.WriteField("output_format", outputFormat)
			_ = writer.WriteField("moderation", stringParamDefault(payload.Params, "moderation", "auto"))
			_ = writer.WriteField("quality", stringParamDefault(payload.Params, "quality", "auto"))
		}
		if n := intParam(payload.Params, "n"); n > 1 {
			_ = writer.WriteField("n", strconv.Itoa(n))
		}
		for i, value := range payload.InputImageDataURLs {
			bytes, contentType, err := dataURLBytes(value)
			if err != nil {
				return imageServiceResult{}, err
			}
			part, err := writer.CreateFormFile("image[]", fmt.Sprintf("input-%d.%s", i+1, imageExt(contentType)))
			if err != nil {
				return imageServiceResult{}, err
			}
			if _, err := part.Write(bytes); err != nil {
				return imageServiceResult{}, err
			}
		}
		if payload.MaskDataURL != "" {
			bytes, _, err := dataURLBytes(payload.MaskDataURL)
			if err != nil {
				return imageServiceResult{}, err
			}
			part, err := writer.CreateFormFile("mask", "mask.png")
			if err != nil {
				return imageServiceResult{}, err
			}
			if _, err := part.Write(bytes); err != nil {
				return imageServiceResult{}, err
			}
		}
		if err := writer.Close(); err != nil {
			return imageServiceResult{}, err
		}
		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpointURL(profile, "images/edits"), body)
		if err != nil {
			return imageServiceResult{}, err
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, err = client.Do(req)
		if err != nil {
			return imageServiceResult{}, err
		}
	} else {
		requestBody := map[string]any{
			"model":  profile.Model,
			"prompt": payload.Prompt,
			"size":   requestSize,
		}
		if profile.APIMode == "ohmytoken" {
			requestBody["response_format"] = "b64_json"
			if aspectRatio != "" {
				requestBody["aspect_ratio"] = aspectRatio
			}
		} else {
			requestBody["output_format"] = outputFormat
			requestBody["moderation"] = stringParamDefault(payload.Params, "moderation", "auto")
			requestBody["quality"] = stringParamDefault(payload.Params, "quality", "auto")
		}
		if n := intParam(payload.Params, "n"); n > 1 {
			requestBody["n"] = n
		}
		body, _ := json.Marshal(requestBody)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpointURL(profile, "images/generations"), bytes.NewReader(body))
		if err != nil {
			return imageServiceResult{}, err
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err = client.Do(req)
		if err != nil {
			return imageServiceResult{}, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		text, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(text) == 0 {
			return imageServiceResult{}, fmt.Errorf("Image service failed with HTTP %d", resp.StatusCode)
		}
		return imageServiceResult{}, fmt.Errorf("%s", strings.TrimSpace(string(text)))
	}

	var data struct {
		Data []struct {
			B64JSON       string `json:"b64_json"`
			URL           string `json:"url"`
			RevisedPrompt string `json:"revised_prompt"`
		} `json:"data"`
		Detail            any    `json:"detail"`
		Error             any    `json:"error"`
		Size              string `json:"size"`
		AspectRatio       string `json:"aspect_ratio"`
		Quality           string `json:"quality"`
		OutputFormat      string `json:"output_format"`
		ResponseFormat    string `json:"response_format"`
		OutputCompression any    `json:"output_compression"`
		Moderation        string `json:"moderation"`
		N                 any    `json:"n"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return imageServiceResult{}, err
	}
	if len(data.Data) == 0 {
		return imageServiceResult{}, fmt.Errorf("Image service returned no image data")
	}

	result := imageServiceResult{
		Images:         []imageResult{},
		ActualParams:   map[string]any{},
		RevisedPrompts: []string{},
	}
	for _, item := range data.Data {
		result.RevisedPrompts = append(result.RevisedPrompts, item.RevisedPrompt)
		if item.B64JSON != "" {
			bytes, err := base64.StdEncoding.DecodeString(item.B64JSON)
			if err != nil {
				bytes, err = base64.RawStdEncoding.DecodeString(item.B64JSON)
			}
			if err != nil {
				return imageServiceResult{}, err
			}
			result.Images = append(result.Images, imageResult{Bytes: bytes, ContentType: fallbackMime})
			continue
		}
		if item.URL != "" {
			bytes, contentType, err := downloadImage(requestCtx, client, item.URL, fallbackMime)
			if err != nil {
				return imageServiceResult{}, err
			}
			result.Images = append(result.Images, imageResult{Bytes: bytes, ContentType: contentType})
		}
	}
	if len(result.Images) == 0 {
		return imageServiceResult{}, fmt.Errorf("Image service returned no usable image data")
	}
	if data.Size != "" {
		result.ActualParams["size"] = data.Size
	}
	if data.AspectRatio != "" {
		result.ActualParams["aspect_ratio"] = data.AspectRatio
	}
	if data.Quality != "" {
		result.ActualParams["quality"] = data.Quality
	}
	if data.OutputFormat != "" {
		result.ActualParams["output_format"] = data.OutputFormat
	}
	if data.ResponseFormat != "" {
		result.ActualParams["response_format"] = data.ResponseFormat
	}
	if data.OutputCompression != nil {
		result.ActualParams["output_compression"] = data.OutputCompression
	}
	if data.Moderation != "" {
		result.ActualParams["moderation"] = data.Moderation
	}
	if data.N != nil {
		result.ActualParams["n"] = data.N
	}
	return result, nil
}

func publicTask(r *http.Request, task taskRow, wallet walletSnapshot) map[string]any {
	return (&Server{}).publicTask(r, task, wallet)
}

func (s *Server) publicTask(r *http.Request, task taskRow, wallet walletSnapshot) map[string]any {
	base := s.publicBaseURL(r)
	outputs := decodeOutputs(task.OutputsJSON)
	publicOutputs := make([]map[string]any, 0, len(outputs))
	for i, output := range outputs {
		outputURL := fmt.Sprintf("%s/api/tasks/%s/images/%d", base, task.ID, i)
		if walletAddress := requestWalletAddress(r); isWalletAddress(walletAddress) {
			outputURL += "?walletAddress=" + url.QueryEscape(walletAddress)
		}
		publicOutputs = append(publicOutputs, map[string]any{
			"url":         outputURL,
			"contentType": output.ContentType,
		})
	}
	walletAddress := wallet.Wallet.Address
	if walletAddress == "" {
		walletAddress = requestWalletAddress(r)
	}
	payload := map[string]any{
		"id":             task.ID,
		"status":         task.Status,
		"walletId":       task.WalletID,
		"walletAddress":  walletAddress,
		"serviceCode":    task.ServiceCode,
		"creditReserved": task.CreditReserved,
		"creditCharged":  task.CreditCharged,
		"error":          nullableString(task.Error),
		"requestedSize":  task.RequestedSize,
		"serviceProfile": task.ServiceProfile,
		"createdAt":      task.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt":      task.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"finishedAt":     nullableTime(task.FinishedAt),
		"outputs":        publicOutputs,
		"actualParams":   jsonField(task.ActualParamsJSON),
		"revisedPrompts": jsonField(task.RevisedPromptsJSON),
	}
	if isWalletAddress(wallet.Wallet.Address) {
		payload["wallet"] = publicWallet(wallet)
	}
	return payload
}

func requestWalletAddress(r *http.Request) string {
	if r == nil {
		return ""
	}
	address := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("walletAddress")))
	if address == "" {
		address = strings.ToLower(strings.TrimSpace(r.Header.Get("X-Wallet-Address")))
	}
	return address
}

func (s *Server) taskByIDForWallet(ctx context.Context, taskID, walletID string) (taskRow, error) {
	query, args := walletTaskByIDQuery(taskID, walletID)
	return scanTaskRow(s.db.QueryRow(ctx, query, args...))
}

func walletTaskByIDQuery(taskID, walletID string) (string, []any) {
	return `
		SELECT id, COALESCE(wallet_id, ''), COALESCE(service_code, ''),
			credit_reserved, credit_charged, status, requested_size,
			service_profile, request_json, outputs_json, actual_params_json,
			revised_prompts_json, error, created_at, updated_at, finished_at
		FROM tasks
		WHERE id = $1 AND wallet_id = $2
	`, []any{taskID, walletID}
}

func scanTaskRow(row scanner) (taskRow, error) {
	var task taskRow
	err := row.Scan(
		&task.ID,
		&task.WalletID,
		&task.ServiceCode,
		&task.CreditReserved,
		&task.CreditCharged,
		&task.Status,
		&task.RequestedSize,
		&task.ServiceProfile,
		&task.RequestJSON,
		&task.OutputsJSON,
		&task.ActualParamsJSON,
		&task.RevisedPromptsJSON,
		&task.Error,
		&task.CreatedAt,
		&task.UpdatedAt,
		&task.FinishedAt,
	)
	return task, err
}

func (s *Server) selectServiceProfileForWallet(ctx context.Context, serviceCode, size, sizeTier string, exclude []string) (serviceProfileRow, error) {
	buckets, err := candidateBucketsForService(serviceCode, sizeTier)
	if err != nil {
		return serviceProfileRow{}, err
	}
	return s.selectServiceProfileFromBuckets(ctx, buckets, exclude)
}

func (s *Server) selectServiceProfileFromBuckets(ctx context.Context, buckets []string, exclude []string) (serviceProfileRow, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, label, tier_bucket, api_base_url, api_key_ciphertext, model, api_mode,
			codex_cli, priority, max_concurrent, status, selection_count, success_count, failure_count,
			last_selected_at, last_failed_at, disabled_until, created_at, updated_at
		FROM service_profiles
		WHERE tier_bucket = ANY($1)
			AND status = 'active'
			AND api_key_ciphertext <> ''
			AND (disabled_until IS NULL OR disabled_until <= $2)
		ORDER BY
			array_position($1, tier_bucket),
			priority ASC,
			last_selected_at ASC NULLS FIRST,
			selection_count ASC,
			id ASC
		LIMIT 50
	`, buckets, time.Now().UTC())
	if err != nil {
		return serviceProfileRow{}, err
	}
	defer rows.Close()
	excluded := map[string]struct{}{}
	for _, id := range exclude {
		excluded[id] = struct{}{}
	}
	for rows.Next() {
		profile, err := scanServiceProfile(rows)
		if err != nil {
			return serviceProfileRow{}, err
		}
		if _, ok := excluded[profile.ID]; ok {
			continue
		}
		_, _ = s.db.Exec(ctx, `
			UPDATE service_profiles
			SET selection_count = selection_count + 1,
				last_selected_at = $1,
				updated_at = $1
			WHERE id = $2
		`, time.Now().UTC(), profile.ID)
		return profile, nil
	}
	if err := rows.Err(); err != nil {
		return serviceProfileRow{}, err
	}
	if len(buckets) == 1 && buckets[0] == "hd" {
		return serviceProfileRow{}, fmt.Errorf("高清生成服务暂时不可用")
	}
	return serviceProfileRow{}, fmt.Errorf("普通生成服务暂时不可用")
}

func (s *Server) recordServiceProfileSuccess(ctx context.Context, id string) error {
	_, err := s.db.Exec(ctx, `
		UPDATE service_profiles
		SET success_count = success_count + 1,
			failure_count = 0,
			disabled_until = NULL,
			updated_at = $1
		WHERE id = $2
	`, time.Now().UTC(), id)
	return err
}

func (s *Server) recordServiceProfileFailure(ctx context.Context, profile serviceProfileRow) error {
	nextFailureCount := profile.FailureCount + 1
	var disabledUntil any
	if nextFailureCount >= serviceProfileCooldownFailures {
		disabledUntil = time.Now().UTC().Add(serviceProfileCooldown)
	}
	_, err := s.db.Exec(ctx, `
		UPDATE service_profiles
		SET failure_count = $1,
			last_failed_at = $2,
			disabled_until = $3,
			updated_at = $2
		WHERE id = $4
	`, nextFailureCount, time.Now().UTC(), disabledUntil, profile.ID)
	return err
}

func (s *Server) failTaskAndRefund(ctx context.Context, task taskRow, message string) {
	now := time.Now().UTC()
	_, _ = s.db.Exec(ctx, `
		UPDATE tasks
		SET status = 'error', error = $1, updated_at = $2, finished_at = $2
		WHERE id = $3 AND status = 'running'
	`, message, now, task.ID)
	if task.WalletID != "" && task.ServiceCode != "" && task.CreditReserved && !task.CreditCharged {
		_, _ = s.db.Exec(ctx, `
			UPDATE wallet_entitlements
			SET remaining = remaining + 1, updated_at = $1
			WHERE wallet_id = $2 AND service_code = $3
		`, now, task.WalletID, task.ServiceCode)
		_, _ = s.db.Exec(ctx, `
			UPDATE tasks
			SET credit_reserved = false, updated_at = $1
			WHERE id = $2
		`, now, task.ID)
		return
	}
}

func endpointURL(profile serviceProfileRow, path string) string {
	return strings.TrimRight(profile.APIBaseURL, "/") + "/" + strings.TrimLeft(path, "/")
}

func normalizeTaskSize(params map[string]any) (string, string) {
	size := strings.TrimSpace(stringParam(params, "size"))
	if size == "" || size == "auto" {
		size = defaultImageSize
	}
	sizeTier := strings.ToUpper(strings.TrimSpace(stringParam(params, "size_tier")))
	if sizeTier != "1K" && sizeTier != "2K" && sizeTier != "4K" {
		if size == defaultImageSize {
			sizeTier = defaultSizeTier
		} else {
			sizeTier = ""
		}
	}
	return size, sizeTier
}

func selectWalletServiceForGeneration(entitlements []walletEntitlement, sizeTier string) (walletEntitlement, error) {
	sizeTier = strings.ToUpper(strings.TrimSpace(sizeTier))
	if sizeTier == "2K" || sizeTier == "4K" {
		if entitlement, ok := walletEntitlementWithBalance(entitlements, serviceCodeImage2HD); ok {
			return entitlement, nil
		}
		return walletEntitlement{}, fmt.Errorf("当前钱包没有可用的 HD 生成权益")
	}
	if entitlement, ok := walletEntitlementWithBalance(entitlements, serviceCodeImage2SD); ok {
		return entitlement, nil
	}
	if entitlement, ok := walletEntitlementWithBalance(entitlements, serviceCodeImage2HD); ok {
		return entitlement, nil
	}
	return walletEntitlement{}, fmt.Errorf("当前钱包没有可用的图片生成权益")
}

func walletEntitlementWithBalance(entitlements []walletEntitlement, serviceCode string) (walletEntitlement, bool) {
	for _, entitlement := range entitlements {
		if entitlement.ServiceCode == serviceCode && entitlement.Remaining > 0 {
			if entitlement.MaxConcurrent <= 0 {
				entitlement.MaxConcurrent = defaultMaxConcurrent
			}
			return entitlement, true
		}
	}
	return walletEntitlement{}, false
}

func candidateBucketsForService(serviceCode, sizeTier string) ([]string, error) {
	sizeTier = strings.ToUpper(strings.TrimSpace(sizeTier))
	if sizeTier == "2K" || sizeTier == "4K" {
		if serviceCode != serviceCodeImage2HD {
			return nil, fmt.Errorf("当前钱包权益不支持 2K/4K 高清图片")
		}
		return []string{"hd"}, nil
	}
	if serviceCode == serviceCodeImage2HD {
		return []string{"1k", "hd"}, nil
	}
	return []string{"1k"}, nil
}

func replaceEntitlementRemaining(entitlements []walletEntitlement, serviceCode string, remaining int) []walletEntitlement {
	result := append([]walletEntitlement(nil), entitlements...)
	for i := range result {
		if result[i].ServiceCode == serviceCode {
			result[i].Remaining = maxInt(0, remaining)
			return result
		}
	}
	return result
}

func aspectRatioFromSize(size string) string {
	parts := strings.FieldsFunc(size, func(r rune) bool { return r == 'x' || r == 'X' || r == '×' })
	if len(parts) != 2 {
		return ""
	}
	width, errW := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, errH := strconv.Atoi(strings.TrimSpace(parts[1]))
	if errW != nil || errH != nil || width <= 0 || height <= 0 {
		return ""
	}
	presets := []struct {
		name  string
		value float64
	}{
		{"9:16", 9.0 / 16.0},
		{"16:9", 16.0 / 9.0},
		{"3:4", 3.0 / 4.0},
		{"4:3", 4.0 / 3.0},
		{"1:1", 1.0},
	}
	actual := float64(width) / float64(height)
	bestName := ""
	bestDelta := 10.0
	for _, preset := range presets {
		delta := absFloat(actual-preset.value) / preset.value
		if delta < bestDelta {
			bestDelta = delta
			bestName = preset.name
		}
	}
	if bestDelta < 0.03 {
		return bestName
	}
	divisor := gcd(width, height)
	return fmt.Sprintf("%d:%d", width/divisor, height/divisor)
}

func dataURLBytes(value string) ([]byte, string, error) {
	if !strings.HasPrefix(value, "data:") {
		bytes, err := base64.StdEncoding.DecodeString(value)
		return bytes, "image/png", err
	}
	comma := strings.Index(value, ",")
	if comma < 0 {
		return nil, "", fmt.Errorf("invalid image data URL")
	}
	meta := value[5:comma]
	payload := value[comma+1:]
	contentType := "application/octet-stream"
	if meta != "" {
		contentType = strings.Split(meta, ";")[0]
	}
	if strings.Contains(meta, ";base64") {
		bytes, err := base64.StdEncoding.DecodeString(payload)
		return bytes, contentType, err
	}
	decoded, err := url.QueryUnescape(payload)
	if err != nil {
		return nil, "", err
	}
	return []byte(decoded), contentType, nil
}

func downloadImage(ctx context.Context, client *http.Client, imageURL, fallbackMime string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Cache-Control", "no-store")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("Image URL download failed with HTTP %d", resp.StatusCode)
	}
	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = fallbackMime
	}
	return bytes, contentType, nil
}

func imageMime(format string) string {
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	default:
		return "image/png"
	}
}

func imageExt(contentType string) string {
	contentType = strings.ToLower(contentType)
	if strings.Contains(contentType, "jpeg") || strings.Contains(contentType, "jpg") {
		return "jpg"
	}
	if strings.Contains(contentType, "webp") {
		return "webp"
	}
	return "png"
}

func decodeOutputs(value sql.NullString) []taskOutput {
	if !value.Valid || value.String == "" {
		return []taskOutput{}
	}
	var outputs []taskOutput
	if err := json.Unmarshal([]byte(value.String), &outputs); err != nil {
		return []taskOutput{}
	}
	return outputs
}

func jsonField(value sql.NullString) any {
	if !value.Valid || value.String == "" {
		return nil
	}
	var decoded any
	if err := json.Unmarshal([]byte(value.String), &decoded); err != nil {
		return nil
	}
	return decoded
}

func nullableString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}

func stringParam(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func stringParamDefault(params map[string]any, key, fallback string) string {
	value := strings.TrimSpace(stringParam(params, key))
	if value == "" {
		return fallback
	}
	return value
}

func intParam(params map[string]any, key string) int {
	if params == nil {
		return 0
	}
	switch value := params[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	case string:
		parsed, _ := strconv.Atoi(value)
		return parsed
	default:
		return 0
	}
}

func errorsIsNoRows(err error) bool {
	return err == pgx.ErrNoRows
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
