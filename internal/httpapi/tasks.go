package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
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
)

const (
	defaultImageSize               = "1024x1824"
	defaultSizeTier                = "1K"
	serviceProfileCooldownFailures = 2
	serviceProfileCooldown         = 5 * time.Minute
	serviceProfileFallbackRetries  = 1
)

type taskPayload struct {
	Prompt             string         `json:"prompt"`
	Params             map[string]any `json:"params"`
	InputImageDataURLs []string       `json:"inputImageDataUrls"`
	MaskDataURL        string         `json:"maskDataUrl,omitempty"`
}

type taskRow struct {
	ID                 string
	LicenseKeyID       string
	ActivationID       string
	Status             string
	Tier               string
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
	concurrency := s.cfg.TaskWorkerConcurrency
	if concurrency <= 0 {
		concurrency = 1
	}
	for i := 0; i < concurrency; i++ {
		go s.taskWorkerLoop(ctx)
	}
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

func (s *Server) processNextQueuedTask(ctx context.Context) bool {
	taskCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var taskID string
	err := s.db.QueryRow(taskCtx, `
		SELECT id
		FROM tasks
		WHERE status = 'queued'
		ORDER BY created_at ASC
		LIMIT 1
	`).Scan(&taskID)
	if errorsIsNoRows(err) {
		return false
	}
	if err != nil {
		if s.logger != nil {
			s.logger.Printf("task worker query failed: %v", err)
		}
		return false
	}
	if err := s.processTask(ctx, taskID); err != nil && s.logger != nil {
		s.logger.Printf("task %s failed: %v", taskID, err)
	}
	return true
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	auth, ok := s.verifyActivation(w, r)
	if !ok {
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
	requestedSize, requestedSizeTier := normalizeTaskSize(body.Params)
	body.Params["size"] = requestedSize
	body.Params["output_format"] = "png"
	body.Params["output_compression"] = nil
	if requestedSizeTier != "" {
		body.Params["size_tier"] = requestedSizeTier
	}

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	profile, err := s.selectServiceProfile(ctx, auth.Tier, requestedSize, requestedSizeTier, nil)
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

	var activeTasks int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM tasks
		WHERE license_key_id = $1 AND status IN ('queued', 'running')
	`, auth.LicenseKeyID).Scan(&activeTasks); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if activeTasks >= auth.MaxConcurrent {
		errorJSON(w, http.StatusTooManyRequests, "并发任务已达上限")
		return
	}
	tag, err := tx.Exec(ctx, `
		UPDATE license_keys
		SET remaining_credits = remaining_credits - 1, updated_at = $1
		WHERE id = $2 AND remaining_credits > 0 AND status = 'active'
	`, now, auth.LicenseKeyID)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() != 1 {
		errorJSON(w, http.StatusForbidden, "兑换密匙次数已用完")
		return
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO tasks (
			id, license_key_id, activation_id, status, tier, requested_size,
			service_profile, request_json, created_at, updated_at
		) VALUES ($1, $2, $3, 'queued', $4, $5, $6, $7, $8, $9)
	`, taskID, auth.LicenseKeyID, auth.ActivationID, auth.Tier, requestedSize, profile.ID, requestJSON, now, now)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	ledgerID, err := randomUUID()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO credit_ledger (id, license_key_id, task_id, type, amount, created_at, note)
		VALUES ($1, $2, $3, 'reserve', -1, $4, 'task created')
	`, ledgerID, auth.LicenseKeyID, taskID, now)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), s.cfg.UpstreamTimeout+time.Minute)
		defer cancel()
		if err := s.processTask(bgCtx, taskID); err != nil && s.logger != nil {
			s.logger.Printf("task %s failed: %v", taskID, err)
		}
	}()

	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":               taskID,
		"status":           "queued",
		"remainingCredits": maxInt(0, auth.RemainingCredits-1),
	})
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	auth, ok := s.verifyActivation(w, r)
	if !ok {
		return
	}
	task, ok := s.taskForLicense(w, r, chi.URLParam(r, "id"), auth.LicenseKeyID)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.publicTask(r, task))
}

func (s *Server) handleGetTaskImage(w http.ResponseWriter, r *http.Request) {
	auth, ok := s.verifyActivation(w, r)
	if !ok {
		return
	}
	if s.store == nil {
		errorJSON(w, http.StatusServiceUnavailable, "object store is not configured")
		return
	}
	task, ok := s.taskForLicense(w, r, chi.URLParam(r, "id"), auth.LicenseKeyID)
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

func (s *Server) taskForLicense(w http.ResponseWriter, r *http.Request, taskID, licenseKeyID string) (taskRow, bool) {
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	task, err := s.taskByID(ctx, taskID)
	if errorsIsNoRows(err) || task.LicenseKeyID != licenseKeyID {
		errorJSON(w, http.StatusNotFound, "任务不存在")
		return taskRow{}, false
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return taskRow{}, false
	}
	return task, true
}

func (s *Server) processTask(ctx context.Context, taskID string) error {
	task, err := s.taskByID(ctx, taskID)
	if errorsIsNoRows(err) || task.Status != "queued" {
		return nil
	}
	if err != nil {
		return err
	}

	startedAt := time.Now().UTC()
	tag, err := s.db.Exec(ctx, `
		UPDATE tasks
		SET status = 'running', updated_at = $1
		WHERE id = $2 AND status = 'queued'
	`, startedAt, taskID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
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
		next, selectErr := s.selectServiceProfile(ctx, task.Tier, task.RequestedSize, stringParam(payload.Params, "size_tier"), tried)
		if selectErr != nil {
			break
		}
		profile = &next
		_, _ = s.db.Exec(ctx, `UPDATE tasks SET service_profile = $1, updated_at = $2 WHERE id = $3`, profile.ID, time.Now().UTC(), taskID)
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
		key := fmt.Sprintf("tasks/%s/outputs/%d.%s", taskID, i, imageExt(image.ContentType))
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
			outputs_json = $1,
			actual_params_json = $2,
			revised_prompts_json = $3,
			updated_at = $4,
			finished_at = $5
		WHERE id = $6
	`, string(outputsJSON), string(actualParamsJSON), string(revisedPromptsJSON), finishedAt, finishedAt, taskID)
	if err != nil {
		return err
	}
	ledgerID, _ := randomUUID()
	_, _ = s.db.Exec(ctx, `
		INSERT INTO credit_ledger (id, license_key_id, task_id, type, amount, created_at, note)
		VALUES ($1, $2, $3, 'consume', 0, $4, 'task completed')
	`, ledgerID, task.LicenseKeyID, taskID, finishedAt)
	return nil
}

func (s *Server) callImageService(ctx context.Context, profile serviceProfileRow, payload taskPayload) (imageServiceResult, error) {
	apiKey, err := s.decryptSecret(profile.APIKeyCiphertext)
	if err != nil {
		return imageServiceResult{}, err
	}
	requestSize, _ := normalizeTaskSize(payload.Params)
	outputFormat := "png"
	fallbackMime := imageMime(outputFormat)
	isEdit := len(payload.InputImageDataURLs) > 0
	aspectRatio := aspectRatioFromSize(requestSize)
	client := &http.Client{Timeout: s.cfg.UpstreamTimeout}

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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL(profile, "images/edits"), body)
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL(profile, "images/generations"), bytes.NewReader(body))
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
			bytes, contentType, err := downloadImage(ctx, client, item.URL, fallbackMime)
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

func (s *Server) publicTask(r *http.Request, task taskRow) map[string]any {
	base := strings.TrimRight(s.cfg.PublicAPIBaseURL, "/")
	if base == "" {
		base = "http://" + r.Host
	}
	outputs := decodeOutputs(task.OutputsJSON)
	publicOutputs := make([]map[string]any, 0, len(outputs))
	for i, output := range outputs {
		publicOutputs = append(publicOutputs, map[string]any{
			"url":         fmt.Sprintf("%s/api/tasks/%s/images/%d", base, task.ID, i),
			"contentType": output.ContentType,
		})
	}
	return map[string]any{
		"id":             task.ID,
		"status":         task.Status,
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
}

func (s *Server) taskByID(ctx context.Context, taskID string) (taskRow, error) {
	var task taskRow
	err := s.db.QueryRow(ctx, `
		SELECT id, license_key_id, activation_id, status, tier, requested_size,
			service_profile, request_json, outputs_json, actual_params_json,
			revised_prompts_json, error, created_at, updated_at, finished_at
		FROM tasks
		WHERE id = $1
	`, taskID).Scan(
		&task.ID,
		&task.LicenseKeyID,
		&task.ActivationID,
		&task.Status,
		&task.Tier,
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

func (s *Server) selectServiceProfile(ctx context.Context, tier, size, sizeTier string, exclude []string) (serviceProfileRow, error) {
	buckets, err := candidateBuckets(tier, sizeTier)
	if err != nil {
		return serviceProfileRow{}, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, label, tier_bucket, api_base_url, api_key_ciphertext, model, api_mode,
			codex_cli, priority, status, selection_count, success_count, failure_count,
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
	_, _ = s.db.Exec(ctx, `
		UPDATE license_keys
		SET remaining_credits = remaining_credits + 1, updated_at = $1
		WHERE id = $2
	`, now, task.LicenseKeyID)
	ledgerID, _ := randomUUID()
	_, _ = s.db.Exec(ctx, `
		INSERT INTO credit_ledger (id, license_key_id, task_id, type, amount, created_at, note)
		VALUES ($1, $2, $3, 'refund', 1, $4, 'task failed before producing output')
	`, ledgerID, task.LicenseKeyID, task.ID, now)
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

func candidateBuckets(tier, sizeTier string) ([]string, error) {
	sizeTier = strings.ToUpper(strings.TrimSpace(sizeTier))
	if sizeTier == "2K" || sizeTier == "4K" {
		if tier != "hd" {
			return nil, fmt.Errorf("当前密匙不支持 2K/4K 高清图片")
		}
		return []string{"hd"}, nil
	}
	if tier == "hd" {
		return []string{"1k", "hd"}, nil
	}
	return []string{"1k"}, nil
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
