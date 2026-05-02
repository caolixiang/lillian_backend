package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type adminWalletStore interface {
	AdminWalletSummary(ctx context.Context, address string) (adminWalletSummary, error)
}

type adminWalletSummary struct {
	Wallet       walletRecord
	Entitlements []walletEntitlement
	Redemptions  []adminWalletRedemption
	Tasks        []adminWalletTask
}

type adminWalletRedemption struct {
	ID            string    `json:"id"`
	LicenseKeyID  string    `json:"licenseKeyId"`
	ServiceCode   string    `json:"serviceCode"`
	CreditsAdded  int       `json:"creditsAdded"`
	LicenseNote   string    `json:"licenseNote,omitempty"`
	LicenseStatus string    `json:"licenseStatus,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

type adminWalletTask struct {
	ID                  string
	ServiceCode         string
	Status              string
	RequestedSize       string
	ServiceProfile      string
	ServiceProfileLabel string
	CreditReserved      bool
	CreditCharged       bool
	Error               sql.NullString
	CreatedAt           time.Time
	UpdatedAt           time.Time
	FinishedAt          sql.NullTime
}

type adminWalletTaskResponse struct {
	ID                  string `json:"id"`
	ServiceCode         string `json:"serviceCode"`
	Status              string `json:"status"`
	RequestedSize       string `json:"requestedSize"`
	ServiceProfile      string `json:"serviceProfile"`
	ServiceProfileLabel string `json:"serviceProfileLabel"`
	CreditReserved      bool   `json:"creditReserved"`
	CreditCharged       bool   `json:"creditCharged"`
	Error               any    `json:"error"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
	FinishedAt          any    `json:"finishedAt"`
}

func (s postgresWalletStore) AdminWalletSummary(ctx context.Context, address string) (adminWalletSummary, error) {
	snapshot, err := s.WalletByAddress(ctx, address)
	if err != nil {
		return adminWalletSummary{}, err
	}
	redemptions, err := s.adminWalletRedemptions(ctx, snapshot.Wallet.ID)
	if err != nil {
		return adminWalletSummary{}, err
	}
	tasks, err := s.adminWalletTasks(ctx, snapshot.Wallet.ID)
	if err != nil {
		return adminWalletSummary{}, err
	}
	return adminWalletSummary{
		Wallet:       snapshot.Wallet,
		Entitlements: snapshot.Entitlements,
		Redemptions:  redemptions,
		Tasks:        tasks,
	}, nil
}

func (s postgresWalletStore) adminWalletRedemptions(ctx context.Context, walletID string) ([]adminWalletRedemption, error) {
	rows, err := s.db.Query(ctx, `
		SELECT r.id, r.license_key_id, r.service_code, r.credits_added,
			COALESCE(l.note, ''), COALESCE(l.status, ''), r.created_at
		FROM wallet_redemptions r
		LEFT JOIN license_keys l ON l.id = r.license_key_id
		WHERE r.wallet_id = $1
		ORDER BY r.created_at DESC
		LIMIT 50
	`, walletID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	redemptions := []adminWalletRedemption{}
	for rows.Next() {
		var redemption adminWalletRedemption
		if err := rows.Scan(
			&redemption.ID,
			&redemption.LicenseKeyID,
			&redemption.ServiceCode,
			&redemption.CreditsAdded,
			&redemption.LicenseNote,
			&redemption.LicenseStatus,
			&redemption.CreatedAt,
		); err != nil {
			return nil, err
		}
		redemptions = append(redemptions, redemption)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return redemptions, nil
}

func (s postgresWalletStore) adminWalletTasks(ctx context.Context, walletID string) ([]adminWalletTask, error) {
	rows, err := s.db.Query(ctx, `
		SELECT t.id, COALESCE(t.service_code, ''), t.status, t.requested_size, t.service_profile,
			COALESCE(sp.label, ''), t.credit_reserved, t.credit_charged, t.error,
			t.created_at, t.updated_at, t.finished_at
		FROM tasks t
		LEFT JOIN service_profiles sp ON sp.id = t.service_profile
		WHERE t.wallet_id = $1
		ORDER BY t.created_at DESC
		LIMIT 50
	`, walletID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := []adminWalletTask{}
	for rows.Next() {
		var task adminWalletTask
		if err := rows.Scan(
			&task.ID,
			&task.ServiceCode,
			&task.Status,
			&task.RequestedSize,
			&task.ServiceProfile,
			&task.ServiceProfileLabel,
			&task.CreditReserved,
			&task.CreditCharged,
			&task.Error,
			&task.CreatedAt,
			&task.UpdatedAt,
			&task.FinishedAt,
		); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *Server) handleAdminGetWallet(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireAdminWalletStore(w) {
		return
	}
	address := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "address")))
	if !isWalletAddress(address) {
		errorJSON(w, http.StatusBadRequest, "钱包地址格式无效")
		return
	}

	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	summary, err := s.adminWallets.AdminWalletSummary(ctx, address)
	if errors.Is(err, errWalletNotFound) {
		errorJSON(w, http.StatusNotFound, "钱包不存在")
		return
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"wallet":      publicWallet(walletSnapshot{Wallet: summary.Wallet, Entitlements: summary.Entitlements}),
		"redemptions": summary.Redemptions,
		"tasks":       publicAdminWalletTasks(summary.Tasks),
	})
}

func (s *Server) requireAdminWalletStore(w http.ResponseWriter) bool {
	if s.adminWallets == nil {
		errorJSON(w, http.StatusServiceUnavailable, "database is not configured")
		return false
	}
	return true
}

func publicAdminWalletTasks(tasks []adminWalletTask) []adminWalletTaskResponse {
	responses := make([]adminWalletTaskResponse, 0, len(tasks))
	for _, task := range tasks {
		responses = append(responses, adminWalletTaskResponse{
			ID:                  task.ID,
			ServiceCode:         task.ServiceCode,
			Status:              task.Status,
			RequestedSize:       task.RequestedSize,
			ServiceProfile:      task.ServiceProfile,
			ServiceProfileLabel: task.ServiceProfileLabel,
			CreditReserved:      task.CreditReserved,
			CreditCharged:       task.CreditCharged,
			Error:               nullableString(task.Error),
			CreatedAt:           task.CreatedAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:           task.UpdatedAt.UTC().Format(time.RFC3339Nano),
			FinishedAt:          nullableTime(task.FinishedAt),
		})
	}
	return responses
}
