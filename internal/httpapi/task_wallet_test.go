package httpapi

import (
	"database/sql"
	"net/http/httptest"
	"testing"
	"time"
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

	for _, tier := range []string{"2K", "4K"} {
		service, err := selectWalletServiceForGeneration([]walletEntitlement{
			{ServiceCode: serviceCodeImage2SD, Remaining: 3, MaxConcurrent: 6},
			{ServiceCode: serviceCodeImage2HD, Remaining: 1, MaxConcurrent: 6},
		}, tier)
		if err != nil {
			t.Fatalf("select %s service: %v", tier, err)
		}
		if service.ServiceCode != serviceCodeImage2HD {
			t.Fatalf("%s service = %q", tier, service.ServiceCode)
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
