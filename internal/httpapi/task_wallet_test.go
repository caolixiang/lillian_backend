package httpapi

import (
	"context"
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CookSleep/lillian_backend/internal/config"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSelectWalletServiceForGenerationPrefersSDFor1KAndFallsBackToHD(t *testing.T) {
	service, err := selectWalletServiceForGeneration([]walletEntitlement{
		{ServiceCode: serviceCodeImage2SD, Remaining: 3, MaxConcurrent: 6},
		{ServiceCode: serviceCodeImage2HD, Remaining: 9, MaxConcurrent: 6},
	}, "1K")
	if err != nil {
		t.Fatalf("select service: %v", err)
	}
	if service.ServiceCode != serviceCodeImage2SD {
		t.Fatalf("service = %q", service.ServiceCode)
	}

	service, err = selectWalletServiceForGeneration([]walletEntitlement{
		{ServiceCode: serviceCodeImage2SD, Remaining: 0, MaxConcurrent: 6},
		{ServiceCode: serviceCodeImage2HD, Remaining: 9, MaxConcurrent: 6},
	}, "1K")
	if err != nil {
		t.Fatalf("select fallback service: %v", err)
	}
	if service.ServiceCode != serviceCodeImage2HD {
		t.Fatalf("fallback service = %q", service.ServiceCode)
	}
}

func TestSelectWalletServiceForGenerationRequiresHDFor2KAnd4K(t *testing.T) {
	_, err := selectWalletServiceForGeneration([]walletEntitlement{
		{ServiceCode: serviceCodeImage2SD, Remaining: 3, MaxConcurrent: 6},
	}, "2K")
	if err == nil {
		t.Fatalf("expected 2K without HD to fail")
	}

	for _, sizeTier := range []string{"2K", "4K"} {
		service, err := selectWalletServiceForGeneration([]walletEntitlement{
			{ServiceCode: serviceCodeImage2SD, Remaining: 3, MaxConcurrent: 6},
			{ServiceCode: serviceCodeImage2HD, Remaining: 1, MaxConcurrent: 6},
		}, sizeTier)
		if err != nil {
			t.Fatalf("select %s service: %v", sizeTier, err)
		}
		if service.ServiceCode != serviceCodeImage2HD {
			t.Fatalf("%s service = %q", sizeTier, service.ServiceCode)
		}
	}
}

func TestCreditBillingForImageGenerationUsesConfiguredPrices(t *testing.T) {
	server := &Server{
		db: nil,
	}
	for _, sizeTier := range []string{"2K", "4K"} {
		if got := billingKeyForImageGeneration(sizeTier); got != "HD" {
			t.Fatalf("billing key for %s = %q", sizeTier, got)
		}
	}
	if got := creditServiceCandidatesForImageGeneration("1K"); len(got) != 1 || got[0] != serviceCodeImage2SD {
		t.Fatalf("1K candidates = %#v", got)
	}
	if _, err := server.creditBillingForImageGeneration(context.Background(), 10, "4K"); err == nil {
		t.Fatalf("expected missing DB pricing to fail")
	}
}

func TestCandidateBucketsAllow1KGenerationOnHDProvider(t *testing.T) {
	for _, serviceCode := range []string{serviceCodeImage2SD, serviceCodeImage2HD} {
		buckets, err := candidateBucketsForService(serviceCode, "1K")
		if err != nil {
			t.Fatalf("candidate buckets for %s: %v", serviceCode, err)
		}
		if len(buckets) != 2 || buckets[0] != "1k" || buckets[1] != "hd" {
			t.Fatalf("candidate buckets for %s = %#v", serviceCode, buckets)
		}
	}
}

func TestPublicTaskIncludesWalletSnapshotForFrontendRefresh(t *testing.T) {
	address := "0x4444444444444444444444444444444444444444"
	req := httptest.NewRequest("GET", "/api/tasks/task-1?walletAddress="+address, nil)
	task := taskRow{
		ID:             "task-1",
		WalletID:       "wallet-1",
		ServiceCode:    serviceCodeImage2SD,
		CreditReserved: true,
		CreditCharged:  true,
		Status:         "done",
		RequestedSize:  "1024x1824",
		ServiceProfile: "provider-1",
		CreatedAt:      time.Date(2026, 5, 2, 4, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 5, 2, 4, 1, 0, 0, time.UTC),
		FinishedAt:     sql.NullTime{Time: time.Date(2026, 5, 2, 4, 1, 0, 0, time.UTC), Valid: true},
	}
	wallet := walletSnapshot{
		Wallet: walletRecord{ID: "wallet-1", Address: address},
		Entitlements: []walletEntitlement{
			{ServiceCode: serviceCodeImage2SD, Label: "标清", Remaining: 2, MaxConcurrent: 6},
		},
	}

	payload := publicTask(req, task, wallet)

	if payload["walletAddress"] != address {
		t.Fatalf("walletAddress = %#v", payload["walletAddress"])
	}
	responseWallet, ok := payload["wallet"].(walletResponse)
	if !ok {
		t.Fatalf("wallet = %#v", payload["wallet"])
	}
	if responseWallet.Address != address {
		t.Fatalf("wallet.address = %q", responseWallet.Address)
	}
	if len(responseWallet.Entitlements) != 1 || responseWallet.Entitlements[0].Remaining != 2 {
		t.Fatalf("wallet.entitlements = %#v", responseWallet.Entitlements)
	}
}

func TestServiceLabelReturnsUserFacingHDLabel(t *testing.T) {
	if got := serviceLabel(serviceCodeImage2HD); got != "HD 2K/4K" {
		t.Fatalf("serviceLabel(%q) = %q", serviceCodeImage2HD, got)
	}
}

func TestRecoverStaleWalletRunningTasksRefundsReservedCredit(t *testing.T) {
	tx := &fakeStaleTaskTx{
		walletID:    "wallet-1",
		serviceCode: serviceCodeImage2SD,
	}

	recovered, err := recoverStaleWalletRunningTasks(context.Background(), tx, time.Date(2026, 5, 2, 6, 0, 0, 0, time.UTC), time.Hour)
	if err != nil {
		t.Fatalf("recover stale tasks: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d", recovered)
	}
	if !tx.refunded {
		t.Fatalf("reserved wallet credit was not refunded")
	}
	if !tx.markedError {
		t.Fatalf("task was not marked error")
	}
}

func TestWalletTaskLookupFiltersByWalletIDInQuery(t *testing.T) {
	query, args := walletTaskByIDQuery("task-1", "wallet-1")
	if !strings.Contains(query, "WHERE id = $1 AND wallet_id = $2") {
		t.Fatalf("query does not filter by task and wallet id: %s", query)
	}
	if len(args) != 2 || args[0] != "task-1" || args[1] != "wallet-1" {
		t.Fatalf("args = %#v", args)
	}
}

func TestTaskWorkerConcurrencyRespectsConfig(t *testing.T) {
	server := New(config.Config{TaskWorkerConcurrency: 2}, nil, nil, nil)
	if got := server.taskWorkerConcurrency(); got != 2 {
		t.Fatalf("taskWorkerConcurrency = %d", got)
	}

	server.cfg.TaskWorkerConcurrency = 0
	if got := server.taskWorkerConcurrency(); got != taskWorkerSlots {
		t.Fatalf("fallback taskWorkerConcurrency = %d", got)
	}
}

func TestImageEditMultipartUsesImageContentTypesForInputsAndMask(t *testing.T) {
	const pngB64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+/p9sAAAAASUVORK5CYII="
	var imageContentType string
	var maskContentType string

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		reader, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		for {
			part, err := reader.NextPart()
			if err != nil {
				break
			}
			switch part.FormName() {
			case "image[]":
				imageContentType = part.Header.Get("Content-Type")
			case "mask":
				maskContentType = part.Header.Get("Content-Type")
			}
			_ = part.Close()
		}
		if imageContentType != "image/png" {
			t.Fatalf("image[] content-type = %q", imageContentType)
		}
		if maskContentType != "image/png" {
			t.Fatalf("mask content-type = %q", maskContentType)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString([]byte("ok"))}},
		})
	}))
	defer upstream.Close()

	server := New(config.Config{ProviderSecret: "test-provider-secret"}, nil, nil, nil)
	apiKeyCiphertext, err := server.encryptSecret("test-key")
	if err != nil {
		t.Fatalf("encrypt api key: %v", err)
	}

	_, err = server.callImageService(context.Background(), serviceProfileRow{
		APIBaseURL:       upstream.URL + "/v1",
		APIKeyCiphertext: apiKeyCiphertext,
		Model:            "gpt-image-2",
		APIMode:          "ohmytoken",
	}, taskPayload{
		Prompt: "edit",
		Params: map[string]any{"size": "1024x1024"},
		InputImageDataURLs: []string{
			"data:application/octet-stream;base64," + pngB64,
		},
		MaskDataURL: "data:application/octet-stream;base64," + pngB64,
	})
	if err != nil {
		t.Fatalf("call image service: %v", err)
	}
}

type fakeStaleTaskTx struct {
	walletID    string
	serviceCode string
	refunded    bool
	markedError bool
}

func (tx *fakeStaleTaskTx) Query(_ context.Context, sql string, _ ...any) (taskRows, error) {
	if !strings.Contains(sql, "UPDATE tasks") {
		return nil, nil
	}
	tx.markedError = true
	return &singleTaskRows{walletID: tx.walletID, serviceCode: tx.serviceCode}, nil
}

func (tx *fakeStaleTaskTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	switch {
	case strings.Contains(sql, "UPDATE wallet_entitlements"):
		tx.refunded = true
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

type singleTaskRows struct {
	walletID    string
	serviceCode string
	read        bool
}

func (r *singleTaskRows) Close() {}

func (r *singleTaskRows) Err() error { return nil }

func (r *singleTaskRows) Next() bool {
	if r.read {
		return false
	}
	r.read = true
	return true
}

func (r *singleTaskRows) Scan(dest ...any) error {
	values := []any{r.walletID, r.serviceCode, billingSourceEntitlement, 1}
	for i := range dest {
		switch target := dest[i].(type) {
		case *string:
			*target = values[i].(string)
		case *int:
			*target = values[i].(int)
		}
	}
	return nil
}
