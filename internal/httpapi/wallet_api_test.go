package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CookSleep/lillian_backend/internal/config"
)

func TestWalletCreateReturnsAddressRecoveryCodeAndStoresOnlyHash(t *testing.T) {
	store := newFakeWalletStore()
	server := New(config.Config{
		CORSOrigin:       "*",
		LicenseKeyPepper: "test-pepper",
	}, nil, nil, nil)
	server.wallets = store

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/wallets/create", strings.NewReader(`{}`))
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Wallet       walletResponse `json:"wallet"`
		RecoveryCode string         `json:"recoveryCode"`
	}
	decodeJSON(t, rec, &payload)

	if !isWalletAddress(payload.Wallet.Address) {
		t.Fatalf("address = %q", payload.Wallet.Address)
	}
	if !strings.HasPrefix(payload.RecoveryCode, "LIL-WAL-") {
		t.Fatalf("recoveryCode = %q", payload.RecoveryCode)
	}
	if len(payload.Wallet.Entitlements) != 0 {
		t.Fatalf("entitlements = %#v", payload.Wallet.Entitlements)
	}
	if store.lastCreate.RecoveryHash == "" {
		t.Fatalf("store did not receive recovery hash")
	}
	if store.lastCreate.RecoveryHash == payload.RecoveryCode {
		t.Fatalf("stored recovery hash matched plaintext recovery code")
	}
	if strings.Contains(store.lastCreate.RecoveryHash, "LIL-WAL-") {
		t.Fatalf("stored recovery hash contains plaintext code: %q", store.lastCreate.RecoveryHash)
	}
}

func TestWalletRestoreAndGetReturnWalletWithEntitlements(t *testing.T) {
	store := newFakeWalletStore()
	server := New(config.Config{
		CORSOrigin:       "*",
		LicenseKeyPepper: "test-pepper",
	}, nil, nil, nil)
	server.wallets = store

	createRec := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/api/wallets/create", strings.NewReader(`{}`))
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Wallet       walletResponse `json:"wallet"`
		RecoveryCode string         `json:"recoveryCode"`
	}
	decodeJSON(t, createRec, &created)

	store.entitlements[created.Wallet.Address] = []walletEntitlement{
		{
			ServiceCode:   serviceCodeImage2SD,
			Label:         "标清",
			Remaining:     5,
			MaxConcurrent: 6,
		},
	}

	restoreRec := httptest.NewRecorder()
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/wallets/restore", strings.NewReader(`{"recoveryCode":"`+created.RecoveryCode+`"}`))
	server.Handler().ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, body = %s", restoreRec.Code, restoreRec.Body.String())
	}
	var restored struct {
		Wallet walletResponse `json:"wallet"`
	}
	decodeJSON(t, restoreRec, &restored)
	assertWalletPayload(t, restored.Wallet, created.Wallet.Address)

	getRec := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/wallets/"+created.Wallet.Address, nil)
	server.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	var fetched struct {
		Wallet walletResponse `json:"wallet"`
	}
	decodeJSON(t, getRec, &fetched)
	assertWalletPayload(t, fetched.Wallet, created.Wallet.Address)
}

func TestWalletRestoreRejectsBlankRecoveryCode(t *testing.T) {
	server := New(config.Config{CORSOrigin: "*"}, nil, nil, nil)
	server.wallets = newFakeWalletStore()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/wallets/restore", strings.NewReader(`{"recoveryCode":" "}`))
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestWalletRedeemAddsCreditsAndRejectsSecondRedemption(t *testing.T) {
	store := newFakeWalletStore()
	wallet := store.addWallet("0x1111111111111111111111111111111111111111")
	code := "LIL-ABCDE-FGHIJ-KLMNO-PQRST"
	store.licensesByHash["hash:"+code] = walletLicense{
		ID:            "license-1",
		ServiceCode:   serviceCodeImage2HD,
		Credits:       7,
		MaxConcurrent: 8,
		Status:        "active",
	}
	server := New(config.Config{
		CORSOrigin:       "*",
		LicenseKeyPepper: "test-pepper",
	}, nil, nil, nil)
	server.wallets = store
	server.hashSecretFunc = func(value string) string { return "hash:" + strings.TrimSpace(value) }

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/wallets/redeem", strings.NewReader(`{"walletAddress":"`+wallet.Address+`","code":"`+code+`"}`))
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Wallet walletResponse `json:"wallet"`
	}
	decodeJSON(t, rec, &payload)
	if payload.Wallet.Address != wallet.Address {
		t.Fatalf("address = %q", payload.Wallet.Address)
	}
	if len(payload.Wallet.Entitlements) != 1 {
		t.Fatalf("entitlements = %#v", payload.Wallet.Entitlements)
	}
	entitlement := payload.Wallet.Entitlements[0]
	if entitlement.ServiceCode != serviceCodeImage2HD {
		t.Fatalf("serviceCode = %q", entitlement.ServiceCode)
	}
	if entitlement.Remaining != 7 {
		t.Fatalf("remaining = %d", entitlement.Remaining)
	}
	if entitlement.MaxConcurrent != 8 {
		t.Fatalf("maxConcurrent = %d", entitlement.MaxConcurrent)
	}
	if !store.licensesByHash["hash:"+code].Redeemed {
		t.Fatalf("license was not marked redeemed")
	}
	if len(store.redemptions) != 1 {
		t.Fatalf("redemptions = %#v", store.redemptions)
	}

	secondRec := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "/api/wallets/redeem", strings.NewReader(`{"walletAddress":"`+wallet.Address+`","code":"`+code+`"}`))
	server.Handler().ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("second status = %d, body = %s", secondRec.Code, secondRec.Body.String())
	}
}

func TestWalletRedeemRejectsExpiredCode(t *testing.T) {
	store := newFakeWalletStore()
	wallet := store.addWallet("0x2222222222222222222222222222222222222222")
	code := "LIL-EXPIRED-CODE1-CODE2-CODE3"
	store.licensesByHash["hash:"+code] = walletLicense{
		ID:            "license-expired",
		ServiceCode:   serviceCodeImage2SD,
		Credits:       5,
		MaxConcurrent: 6,
		Status:        "active",
		ExpiresAt: sql.NullTime{
			Time:  time.Now().UTC().Add(-time.Hour),
			Valid: true,
		},
	}
	server := New(config.Config{
		CORSOrigin:       "*",
		LicenseKeyPepper: "test-pepper",
	}, nil, nil, nil)
	server.wallets = store
	server.hashSecretFunc = func(value string) string { return "hash:" + strings.TrimSpace(value) }

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/wallets/redeem", strings.NewReader(`{"walletAddress":"`+wallet.Address+`","code":"`+code+`"}`))
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.entitlements[wallet.Address]) != 0 {
		t.Fatalf("entitlements = %#v", store.entitlements[wallet.Address])
	}
}

func assertWalletPayload(t *testing.T, wallet walletResponse, address string) {
	t.Helper()
	if wallet.Address != address {
		t.Fatalf("address = %q, want %q", wallet.Address, address)
	}
	if len(wallet.Entitlements) != 1 {
		t.Fatalf("entitlements = %#v", wallet.Entitlements)
	}
	entitlement := wallet.Entitlements[0]
	if entitlement.ServiceCode != serviceCodeImage2SD {
		t.Fatalf("serviceCode = %q", entitlement.ServiceCode)
	}
	if entitlement.Remaining != 5 {
		t.Fatalf("remaining = %d", entitlement.Remaining)
	}
	if entitlement.MaxConcurrent != 6 {
		t.Fatalf("maxConcurrent = %d", entitlement.MaxConcurrent)
	}
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, dest any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, rec.Body.String())
	}
}

type fakeWalletStore struct {
	walletsByAddress map[string]walletRecord
	walletsByHash    map[string]walletRecord
	entitlements     map[string][]walletEntitlement
	licensesByHash   map[string]walletLicense
	redemptions      []redeemWalletParams
	lastCreate       createWalletParams
}

func newFakeWalletStore() *fakeWalletStore {
	return &fakeWalletStore{
		walletsByAddress: map[string]walletRecord{},
		walletsByHash:    map[string]walletRecord{},
		entitlements:     map[string][]walletEntitlement{},
		licensesByHash:   map[string]walletLicense{},
	}
}

func (s *fakeWalletStore) addWallet(address string) walletRecord {
	record := walletRecord{
		ID:      "wallet-" + address[len(address)-4:],
		Address: address,
	}
	s.walletsByAddress[address] = record
	return record
}

func (s *fakeWalletStore) CreateWallet(_ context.Context, params createWalletParams) (walletSnapshot, error) {
	s.lastCreate = params
	record := walletRecord{
		ID:           params.ID,
		Address:      params.Address,
		RecoveryHash: params.RecoveryHash,
	}
	s.walletsByAddress[params.Address] = record
	s.walletsByHash[params.RecoveryHash] = record
	return s.snapshot(record), nil
}

func (s *fakeWalletStore) WalletByRecoveryHash(_ context.Context, recoveryHash string) (walletSnapshot, error) {
	record, ok := s.walletsByHash[recoveryHash]
	if !ok {
		return walletSnapshot{}, errWalletNotFound
	}
	return s.snapshot(record), nil
}

func (s *fakeWalletStore) WalletByAddress(_ context.Context, address string) (walletSnapshot, error) {
	record, ok := s.walletsByAddress[address]
	if !ok {
		return walletSnapshot{}, errWalletNotFound
	}
	return s.snapshot(record), nil
}

func (s *fakeWalletStore) RedeemLicenseToWallet(_ context.Context, params redeemWalletParams) (walletSnapshot, error) {
	wallet, ok := s.walletsByAddress[params.WalletAddress]
	if !ok {
		return walletSnapshot{}, errWalletNotFound
	}
	license, ok := s.licensesByHash[params.CodeHash]
	if !ok || license.Status != "active" {
		return walletSnapshot{}, errWalletLicenseNotFound
	}
	if license.ExpiresAt.Valid && !license.ExpiresAt.Time.After(params.Now) {
		return walletSnapshot{}, errWalletLicenseExpired
	}
	if license.Redeemed {
		return walletSnapshot{}, errWalletLicenseRedeemed
	}
	license.Redeemed = true
	license.RedeemedWalletID = wallet.ID
	s.licensesByHash[params.CodeHash] = license
	s.redemptions = append(s.redemptions, params)

	entitlements := s.entitlements[wallet.Address]
	found := false
	for i := range entitlements {
		if entitlements[i].ServiceCode == license.ServiceCode {
			entitlements[i].Remaining += license.Credits
			if license.MaxConcurrent > entitlements[i].MaxConcurrent {
				entitlements[i].MaxConcurrent = license.MaxConcurrent
			}
			found = true
			break
		}
	}
	if !found {
		entitlements = append(entitlements, walletEntitlement{
			ServiceCode:   license.ServiceCode,
			Label:         serviceLabel(license.ServiceCode),
			Remaining:     license.Credits,
			MaxConcurrent: license.MaxConcurrent,
		})
	}
	s.entitlements[wallet.Address] = entitlements
	return s.snapshot(wallet), nil
}

func (s *fakeWalletStore) snapshot(record walletRecord) walletSnapshot {
	return walletSnapshot{
		Wallet:       record,
		Entitlements: append([]walletEntitlement(nil), s.entitlements[record.Address]...),
	}
}
