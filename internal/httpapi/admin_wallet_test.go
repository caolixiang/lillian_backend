package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/CookSleep/lillian_backend/internal/config"
)

func TestAdminWalletLookupReturnsWalletRedemptionsAndRecentTasks(t *testing.T) {
	address := "0x3333333333333333333333333333333333333333"
	server := New(config.Config{
		CORSOrigin: "*",
		AdminToken: "secret",
	}, nil, nil, nil)
	server.adminWallets = fakeAdminWalletStore{
		summary: adminWalletSummary{
			Wallet: walletRecord{
				ID:        "wallet-1",
				Address:   address,
				CreatedAt: time.Date(2026, 5, 2, 1, 2, 3, 0, time.UTC),
				UpdatedAt: time.Date(2026, 5, 2, 1, 2, 3, 0, time.UTC),
			},
			Entitlements: []walletEntitlement{
				{ServiceCode: serviceCodeImage2SD, Label: "标清", Remaining: 4, MaxConcurrent: 6},
			},
			Redemptions: []adminWalletRedemption{
				{
					ID:           "redemption-1",
					LicenseKeyID: "license-1",
					ServiceCode:  serviceCodeImage2SD,
					CreditsAdded: 5,
					CreatedAt:    time.Date(2026, 5, 2, 2, 0, 0, 0, time.UTC),
				},
			},
			Tasks: []adminWalletTask{
				{
					ID:            "task-1",
					ServiceCode:   serviceCodeImage2SD,
					Status:        "done",
					RequestedSize: "1024x1824",
					CreatedAt:     time.Date(2026, 5, 2, 3, 0, 0, 0, time.UTC),
				},
			},
		},
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/wallets/"+address, nil)
	req.Header.Set("Authorization", "Bearer secret")
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Wallet      walletResponse            `json:"wallet"`
		Redemptions []adminWalletRedemption   `json:"redemptions"`
		Tasks       []adminWalletTaskResponse `json:"tasks"`
	}
	decodeJSON(t, rec, &payload)
	if payload.Wallet.Address != address {
		t.Fatalf("address = %q", payload.Wallet.Address)
	}
	if len(payload.Wallet.Entitlements) != 1 {
		t.Fatalf("entitlements = %#v", payload.Wallet.Entitlements)
	}
	if len(payload.Redemptions) != 1 || payload.Redemptions[0].CreditsAdded != 5 {
		t.Fatalf("redemptions = %#v", payload.Redemptions)
	}
	if len(payload.Tasks) != 1 || payload.Tasks[0].ID != "task-1" {
		t.Fatalf("tasks = %#v", payload.Tasks)
	}
}

type fakeAdminWalletStore struct {
	summary adminWalletSummary
}

func (s fakeAdminWalletStore) AdminWalletSummary(_ context.Context, address string) (adminWalletSummary, error) {
	if address != s.summary.Wallet.Address {
		return adminWalletSummary{}, errWalletNotFound
	}
	return s.summary, nil
}
