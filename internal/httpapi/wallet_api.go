package httpapi

import (
	"context"
	"encoding/base32"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	serviceCodeImage2SD = "image-2-sd"
	serviceCodeImage2HD = "image-2-hd"
)

var (
	errWalletNotFound = errors.New("wallet not found")
	walletAddressRE   = regexp.MustCompile(`^0x[0-9a-f]{40}$`)
)

type walletStore interface {
	CreateWallet(ctx context.Context, params createWalletParams) (walletSnapshot, error)
	WalletByRecoveryHash(ctx context.Context, recoveryHash string) (walletSnapshot, error)
	WalletByAddress(ctx context.Context, address string) (walletSnapshot, error)
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
		return "HD"
	default:
		return serviceCode
	}
}

func base32NoPadding(bytes []byte) string {
	return strings.TrimRight(base32.StdEncoding.EncodeToString(bytes), "=")
}
