package httpapi

import (
	"context"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	serviceCodeImage2SD = "image-2-sd"
	serviceCodeImage2HD = "image-2-hd"
)

var (
	errWalletNotFound        = errors.New("wallet not found")
	errWalletLicenseNotFound = errors.New("wallet license not found")
	errWalletLicenseExpired  = errors.New("wallet license expired")
	errWalletLicenseRedeemed = errors.New("wallet license redeemed")
	walletAddressRE          = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
)

type walletStore interface {
	CreateWallet(ctx context.Context, params createWalletParams) (walletSnapshot, error)
	WalletByRecoveryHash(ctx context.Context, recoveryHash string) (walletSnapshot, error)
	WalletByAddress(ctx context.Context, address string) (walletSnapshot, error)
	RedeemLicenseToWallet(ctx context.Context, params redeemWalletParams) (walletSnapshot, error)
}

type createWalletParams struct {
	ID           string
	Address      string
	RecoveryHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type walletRecord struct {
	ID           string
	Address      string
	RecoveryHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type walletEntitlement struct {
	ServiceCode   string
	Label         string
	Remaining     int
	MaxConcurrent int
}

type walletLicense struct {
	ID               string
	ServiceCode      string
	Credits          int
	MaxConcurrent    int
	Status           string
	ExpiresAt        sql.NullTime
	Redeemed         bool
	RedeemedWalletID string
}

type redeemWalletParams struct {
	WalletAddress string
	CodeHash      string
	Now           time.Time
}

type walletSnapshot struct {
	Wallet       walletRecord
	Entitlements []walletEntitlement
}

type walletResponse struct {
	Address      string                      `json:"address"`
	Entitlements []walletEntitlementResponse `json:"entitlements"`
}

type walletEntitlementResponse struct {
	ServiceCode   string `json:"serviceCode"`
	Label         string `json:"label"`
	Remaining     int    `json:"remaining"`
	MaxConcurrent int    `json:"maxConcurrent"`
}

type postgresWalletStore struct {
	db *pgxpool.Pool
}

func (s postgresWalletStore) CreateWallet(ctx context.Context, params createWalletParams) (walletSnapshot, error) {
	_, err := s.db.Exec(ctx, `
		INSERT INTO wallets (id, address, recovery_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, params.ID, params.Address, params.RecoveryHash, params.CreatedAt, params.UpdatedAt)
	if err != nil {
		return walletSnapshot{}, err
	}
	return s.WalletByAddress(ctx, params.Address)
}

func (s postgresWalletStore) WalletByRecoveryHash(ctx context.Context, recoveryHash string) (walletSnapshot, error) {
	var wallet walletRecord
	err := s.db.QueryRow(ctx, `
		SELECT id, address, recovery_hash, created_at, updated_at
		FROM wallets
		WHERE recovery_hash = $1
	`, recoveryHash).Scan(&wallet.ID, &wallet.Address, &wallet.RecoveryHash, &wallet.CreatedAt, &wallet.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return walletSnapshot{}, errWalletNotFound
	}
	if err != nil {
		return walletSnapshot{}, err
	}
	return s.snapshot(ctx, wallet)
}

func (s postgresWalletStore) WalletByAddress(ctx context.Context, address string) (walletSnapshot, error) {
	var wallet walletRecord
	err := s.db.QueryRow(ctx, `
		SELECT id, address, recovery_hash, created_at, updated_at
		FROM wallets
		WHERE address = $1
	`, strings.ToLower(strings.TrimSpace(address))).Scan(&wallet.ID, &wallet.Address, &wallet.RecoveryHash, &wallet.CreatedAt, &wallet.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return walletSnapshot{}, errWalletNotFound
	}
	if err != nil {
		return walletSnapshot{}, err
	}
	return s.snapshot(ctx, wallet)
}

func (s postgresWalletStore) RedeemLicenseToWallet(ctx context.Context, params redeemWalletParams) (walletSnapshot, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return walletSnapshot{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	wallet, err := walletByAddressForUpdate(ctx, tx, params.WalletAddress)
	if err != nil {
		return walletSnapshot{}, err
	}
	license, err := licenseByHashForWalletRedeem(ctx, tx, params.CodeHash)
	if err != nil {
		return walletSnapshot{}, err
	}
	if license.ExpiresAt.Valid && !license.ExpiresAt.Time.After(params.Now) {
		return walletSnapshot{}, errWalletLicenseExpired
	}
	if license.Redeemed {
		return walletSnapshot{}, errWalletLicenseRedeemed
	}
	if license.Credits <= 0 {
		return walletSnapshot{}, errWalletLicenseNotFound
	}
	if license.MaxConcurrent <= 0 {
		license.MaxConcurrent = defaultMaxConcurrent
	}

	entitlementID, err := randomUUID()
	if err != nil {
		return walletSnapshot{}, err
	}
	redemptionID, err := randomUUID()
	if err != nil {
		return walletSnapshot{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO wallet_entitlements (
			id, wallet_id, service_code, remaining, max_concurrent, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (wallet_id, service_code) DO UPDATE SET
			remaining = wallet_entitlements.remaining + excluded.remaining,
			max_concurrent = GREATEST(wallet_entitlements.max_concurrent, excluded.max_concurrent),
			updated_at = excluded.updated_at
	`, entitlementID, wallet.ID, license.ServiceCode, license.Credits, license.MaxConcurrent, params.Now, params.Now); err != nil {
		return walletSnapshot{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE license_keys
		SET redeemed_at = $1,
			redeemed_wallet_id = $2,
			updated_at = $3
		WHERE id = $4
	`, params.Now, wallet.ID, params.Now, license.ID); err != nil {
		return walletSnapshot{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO wallet_redemptions (
			id, wallet_id, license_key_id, service_code, credits_added, created_at
		) VALUES ($1, $2, $3, $4, $5, $6)
	`, redemptionID, wallet.ID, license.ID, license.ServiceCode, license.Credits, params.Now); err != nil {
		return walletSnapshot{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return walletSnapshot{}, err
	}
	return s.WalletByAddress(ctx, wallet.Address)
}

func (s postgresWalletStore) snapshot(ctx context.Context, wallet walletRecord) (walletSnapshot, error) {
	rows, err := s.db.Query(ctx, `
		SELECT service_code, remaining, max_concurrent
		FROM wallet_entitlements
		WHERE wallet_id = $1
		ORDER BY service_code
	`, wallet.ID)
	if err != nil {
		return walletSnapshot{}, err
	}
	defer rows.Close()

	entitlements := []walletEntitlement{}
	for rows.Next() {
		var entitlement walletEntitlement
		if err := rows.Scan(&entitlement.ServiceCode, &entitlement.Remaining, &entitlement.MaxConcurrent); err != nil {
			return walletSnapshot{}, err
		}
		entitlement.Label = serviceLabel(entitlement.ServiceCode)
		entitlements = append(entitlements, entitlement)
	}
	if err := rows.Err(); err != nil {
		return walletSnapshot{}, err
	}
	return walletSnapshot{Wallet: wallet, Entitlements: entitlements}, nil
}

type pgxTx interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func walletByAddressForUpdate(ctx context.Context, tx pgxTx, address string) (walletRecord, error) {
	var wallet walletRecord
	err := tx.QueryRow(ctx, `
		SELECT id, address, recovery_hash, created_at, updated_at
		FROM wallets
		WHERE address = $1
		FOR UPDATE
	`, strings.ToLower(strings.TrimSpace(address))).Scan(&wallet.ID, &wallet.Address, &wallet.RecoveryHash, &wallet.CreatedAt, &wallet.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return walletRecord{}, errWalletNotFound
	}
	return wallet, err
}

func licenseByHashForWalletRedeem(ctx context.Context, tx pgxTx, codeHash string) (walletLicense, error) {
	var license walletLicense
	var redeemedAt sql.NullTime
	var redeemedWalletID sql.NullString
	err := tx.QueryRow(ctx, `
		SELECT id, service_code, credits, max_concurrent, status, expires_at, redeemed_at, redeemed_wallet_id
		FROM license_keys
		WHERE key_hash = $1
			AND status = 'active'
		FOR UPDATE
	`, codeHash).Scan(
		&license.ID,
		&license.ServiceCode,
		&license.Credits,
		&license.MaxConcurrent,
		&license.Status,
		&license.ExpiresAt,
		&redeemedAt,
		&redeemedWalletID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return walletLicense{}, errWalletLicenseNotFound
	}
	if err != nil {
		return walletLicense{}, err
	}
	license.Redeemed = redeemedAt.Valid || redeemedWalletID.Valid
	if redeemedWalletID.Valid {
		license.RedeemedWalletID = redeemedWalletID.String
	}
	if license.ServiceCode == "" {
		license.ServiceCode = serviceCodeFromTier("")
	}
	return license, nil
}

func (s *Server) handleCreateWallet(w http.ResponseWriter, r *http.Request) {
	if !s.requireWalletStore(w) {
		return
	}
	if r.Body != nil && r.ContentLength != 0 {
		var body map[string]any
		if !readJSON(w, r, &body) {
			return
		}
	}

	recoveryCode, err := randomRecoveryCode()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	id, err := randomUUID()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	address, err := randomWalletAddress()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	snapshot, err := s.wallets.CreateWallet(ctx, createWalletParams{
		ID:           id,
		Address:      address,
		RecoveryHash: s.hashSecret(recoveryCode),
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"wallet":       publicWallet(snapshot),
		"recoveryCode": recoveryCode,
	})
}

func (s *Server) handleRedeemWallet(w http.ResponseWriter, r *http.Request) {
	if !s.requireWalletStore(w) {
		return
	}
	var body struct {
		WalletAddress string `json:"walletAddress"`
		Code          string `json:"code"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	walletAddress := strings.ToLower(strings.TrimSpace(body.WalletAddress))
	code := strings.TrimSpace(body.Code)
	if !isWalletAddress(walletAddress) {
		errorJSON(w, http.StatusBadRequest, "钱包地址格式无效")
		return
	}
	if code == "" {
		errorJSON(w, http.StatusBadRequest, "请输入兑换码")
		return
	}

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	snapshot, err := s.wallets.RedeemLicenseToWallet(ctx, redeemWalletParams{
		WalletAddress: walletAddress,
		CodeHash:      s.hashSecret(code),
		Now:           time.Now().UTC(),
	})
	switch {
	case errors.Is(err, errWalletNotFound):
		errorJSON(w, http.StatusNotFound, "钱包不存在")
		return
	case errors.Is(err, errWalletLicenseNotFound):
		errorJSON(w, http.StatusNotFound, "兑换码无效或已失效")
		return
	case errors.Is(err, errWalletLicenseExpired):
		errorJSON(w, http.StatusGone, "兑换码已过期")
		return
	case errors.Is(err, errWalletLicenseRedeemed):
		errorJSON(w, http.StatusConflict, "兑换码已被使用")
		return
	case err != nil:
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"wallet": publicWallet(snapshot)})
}

func (s *Server) handleRestoreWallet(w http.ResponseWriter, r *http.Request) {
	if !s.requireWalletStore(w) {
		return
	}
	var body struct {
		RecoveryCode string `json:"recoveryCode"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	recoveryCode := strings.TrimSpace(body.RecoveryCode)
	if recoveryCode == "" {
		errorJSON(w, http.StatusBadRequest, "请输入恢复密码")
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	snapshot, err := s.wallets.WalletByRecoveryHash(ctx, s.hashSecret(recoveryCode))
	if errors.Is(err, errWalletNotFound) {
		errorJSON(w, http.StatusNotFound, "钱包不存在或恢复密码无效")
		return
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"wallet": publicWallet(snapshot)})
}

func (s *Server) handleGetWallet(w http.ResponseWriter, r *http.Request) {
	if !s.requireWalletStore(w) {
		return
	}
	address := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "address")))
	if !isWalletAddress(address) {
		errorJSON(w, http.StatusBadRequest, "钱包地址格式无效")
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	snapshot, err := s.wallets.WalletByAddress(ctx, address)
	if errors.Is(err, errWalletNotFound) {
		errorJSON(w, http.StatusNotFound, "钱包不存在")
		return
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"wallet": publicWallet(snapshot)})
}

func (s *Server) requireWalletStore(w http.ResponseWriter) bool {
	if s.wallets == nil {
		errorJSON(w, http.StatusServiceUnavailable, "database is not configured")
		return false
	}
	return true
}

func publicWallet(snapshot walletSnapshot) walletResponse {
	entitlements := make([]walletEntitlementResponse, 0, len(snapshot.Entitlements))
	for _, entitlement := range snapshot.Entitlements {
		label := entitlement.Label
		if label == "" {
			label = serviceLabel(entitlement.ServiceCode)
		}
		entitlements = append(entitlements, walletEntitlementResponse{
			ServiceCode:   entitlement.ServiceCode,
			Label:         label,
			Remaining:     entitlement.Remaining,
			MaxConcurrent: entitlement.MaxConcurrent,
		})
	}
	return walletResponse{
		Address:      snapshot.Wallet.Address,
		Entitlements: entitlements,
	}
}

func randomRecoveryCode() (string, error) {
	bytes, err := randomBytes(20)
	if err != nil {
		return "", err
	}
	raw := base32NoPadding(bytes)
	if len(raw) < 25 {
		return "", fmt.Errorf("random recovery entropy was too short")
	}
	raw = raw[:25]
	return fmt.Sprintf("LIL-WAL-%s-%s-%s-%s-%s", raw[:5], raw[5:10], raw[10:15], raw[15:20], raw[20:25]), nil
}

func randomWalletAddress() (string, error) {
	bytes, err := randomBytes(20)
	if err != nil {
		return "", err
	}
	return "0x" + fmt.Sprintf("%x", bytes), nil
}

func isWalletAddress(address string) bool {
	return walletAddressRE.MatchString(strings.ToLower(strings.TrimSpace(address)))
}

func serviceLabel(serviceCode string) string {
	switch serviceCode {
	case serviceCodeImage2SD:
		return "标清"
	case serviceCodeImage2HD:
		return "HD 2K/4K"
	default:
		return serviceCode
	}
}

func serviceCodeFromTier(tier string) string {
	if strings.EqualFold(strings.TrimSpace(tier), "hd") {
		return serviceCodeImage2HD
	}
	return serviceCodeImage2SD
}

func base32NoPadding(bytes []byte) string {
	return strings.TrimRight(base32.StdEncoding.EncodeToString(bytes), "=")
}
