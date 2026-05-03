package httpapi

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CookSleep/lillian_backend/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const paymentProviderEPUSDT = "epusdt"

type creditTopupPlan struct {
	ID         string
	Label      string
	AmountUSDT string
	Credits    int
	IsDefault  bool
	Enabled    bool
	SortOrder  int
	Note       string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type serviceCreditPrice struct {
	ID          string
	ServiceCode string
	BillingKey  string
	CreditUnits int
	Enabled     bool
	Note        string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func supportedServiceCreditPrice(serviceCode, billingKey string) bool {
	serviceCode = strings.TrimSpace(serviceCode)
	billingKey = strings.ToUpper(strings.TrimSpace(billingKey))
	switch serviceCode {
	case serviceCodeImage2SD:
		return billingKey == "1K"
	case serviceCodeImage2HD:
		return billingKey == "HD"
	default:
		return false
	}
}

type paymentOrder struct {
	ID              string
	WalletID        string
	Provider        string
	ProviderOrderID string
	ProviderTradeID string
	PlanID          string
	AmountUSDT      string
	Credits         int
	Status          string
	CheckoutURL     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	PaidAt          sql.NullTime
}

func (s *Server) handleCreateWalletTopup(w http.ResponseWriter, r *http.Request) {
	if !s.requireWalletStore(w) || !s.requireDatabase(w) {
		return
	}
	address := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "address")))
	if !isWalletAddress(address) {
		errorJSON(w, http.StatusBadRequest, "钱包地址格式无效")
		return
	}
	if err := s.requireEPUSDTConfig(); err != nil {
		errorJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	var body struct {
		PlanID      string `json:"planId"`
		RedirectURL string `json:"redirectUrl"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if !readJSON(w, r, &body) {
			return
		}
	}

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	wallet, err := s.wallets.WalletByAddress(ctx, address)
	if errors.Is(err, errWalletNotFound) {
		errorJSON(w, http.StatusNotFound, "钱包不存在")
		return
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	plan, err := s.creditTopupPlan(ctx, body.PlanID)
	if errors.Is(err, pgx.ErrNoRows) {
		errorJSON(w, http.StatusNotFound, "充值套餐不可用")
		return
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	amount, err := strconv.ParseFloat(plan.AmountUSDT, 64)
	if err != nil || amount <= 0 {
		errorJSON(w, http.StatusInternalServerError, "充值套餐金额配置无效")
		return
	}

	orderID, err := randomPaymentOrderID()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	now := time.Now().UTC()
	notifyURL := s.publicBaseURL(r) + "/api/payments/epusdt/callback"
	reqBody := map[string]any{
		"pid":          s.cfg.EPUSDT.PID,
		"order_id":     orderID,
		"currency":     strings.ToLower(s.cfg.EPUSDT.Currency),
		"token":        strings.ToLower(s.cfg.EPUSDT.Token),
		"network":      strings.ToLower(s.cfg.EPUSDT.Network),
		"amount":       amount,
		"notify_url":   notifyURL,
		"redirect_url": strings.TrimSpace(body.RedirectURL),
		"name":         plan.Label,
		"payment_type": "lillian-credits",
	}
	reqBody["signature"] = epusdtSignature(reqBody, s.cfg.EPUSDT.SecretKey)
	rawRequest, _ := json.Marshal(reqBody)

	response, err := s.createEPUSDTTransaction(ctx, reqBody)
	if err != nil {
		errorJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	rawResponse, _ := json.Marshal(response.Raw)
	providerTradeID := response.TradeID
	checkoutURL := response.PaymentURL
	if checkoutURL == "" && providerTradeID != "" {
		checkoutURL = strings.TrimRight(s.cfg.EPUSDT.BaseURL, "/") + "/pay/checkout-counter/" + providerTradeID
	}
	if checkoutURL == "" {
		errorJSON(w, http.StatusBadGateway, "EPUSDT 未返回支付地址")
		return
	}

	_, err = s.db.Exec(ctx, `
		INSERT INTO payment_orders (
			id, wallet_id, provider, provider_order_id, provider_trade_id, plan_id,
			amount_usdt, credits, status, checkout_url, raw_request, raw_response,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'created', $9, $10, $11, $12, $13)
	`, orderID, wallet.Wallet.ID, paymentProviderEPUSDT, orderID, providerTradeID, plan.ID,
		plan.AmountUSDT, plan.Credits, checkoutURL, string(rawRequest), string(rawResponse), now, now)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"order": map[string]any{
			"id":              orderID,
			"provider":        paymentProviderEPUSDT,
			"providerTradeId": providerTradeID,
			"planId":          plan.ID,
			"amountUsdt":      plan.AmountUSDT,
			"credits":         plan.Credits,
			"status":          "created",
			"checkoutUrl":     checkoutURL,
			"createdAt":       now.UTC().Format(time.RFC3339Nano),
		},
		"checkoutUrl": checkoutURL,
		"wallet":      publicWallet(wallet),
	})
}

func (s *Server) handleEPUSDTCallback(w http.ResponseWriter, r *http.Request) {
	if !s.requireDatabase(w) {
		return
	}
	if strings.TrimSpace(s.cfg.EPUSDT.SecretKey) == "" {
		errorJSON(w, http.StatusServiceUnavailable, "EPUSDT secret key is not configured")
		return
	}
	var payload map[string]any
	if !readJSON(w, r, &payload) {
		return
	}
	signature := strings.TrimSpace(stringValue(payload["signature"]))
	if signature == "" {
		errorJSON(w, http.StatusBadRequest, "EPUSDT 回调缺少签名")
		return
	}
	expected := epusdtSignature(payload, s.cfg.EPUSDT.SecretKey)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		errorJSON(w, http.StatusUnauthorized, "EPUSDT 回调签名无效")
		return
	}
	orderID := strings.TrimSpace(stringValue(payload["order_id"]))
	if orderID == "" {
		errorJSON(w, http.StatusBadRequest, "EPUSDT 回调缺少订单号")
		return
	}
	status := intValue(payload["status"])
	rawCallback, _ := json.Marshal(payload)

	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx)

	var order paymentOrder
	err = tx.QueryRow(ctx, `
		SELECT id, wallet_id, provider, provider_order_id, provider_trade_id, plan_id,
			amount_usdt::text, credits, status, checkout_url, created_at, updated_at, paid_at
		FROM payment_orders
		WHERE provider = $1 AND provider_order_id = $2
		FOR UPDATE
	`, paymentProviderEPUSDT, orderID).Scan(
		&order.ID,
		&order.WalletID,
		&order.Provider,
		&order.ProviderOrderID,
		&order.ProviderTradeID,
		&order.PlanID,
		&order.AmountUSDT,
		&order.Credits,
		&order.Status,
		&order.CheckoutURL,
		&order.CreatedAt,
		&order.UpdatedAt,
		&order.PaidAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		errorJSON(w, http.StatusNotFound, "充值订单不存在")
		return
	}
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	now := time.Now().UTC()
	providerTradeID := strings.TrimSpace(stringValue(payload["trade_id"]))
	if status != 2 {
		_, err = tx.Exec(ctx, `
			UPDATE payment_orders
			SET provider_trade_id = COALESCE(NULLIF($1, ''), provider_trade_id),
				status = $2,
				raw_callback = $3,
				updated_at = $4
			WHERE id = $5
		`, providerTradeID, epusdtPaymentStatusName(status), string(rawCallback), now, order.ID)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := tx.Commit(ctx); err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writePlainOK(w)
		return
	}

	if err := validateEPUSDTPaidCallback(order, payload, s.cfg.EPUSDT); err != nil {
		errorJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if order.Status != "paid" {
		if _, err := tx.Exec(ctx, `
			UPDATE wallets
			SET credits = credits + $1, updated_at = $2
			WHERE id = $3
		`, order.Credits, now, order.WalletID); err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	_, err = tx.Exec(ctx, `
		UPDATE payment_orders
		SET provider_trade_id = COALESCE(NULLIF($1, ''), provider_trade_id),
			status = 'paid',
			raw_callback = $2,
			updated_at = $3,
			paid_at = COALESCE(paid_at, $3)
		WHERE id = $4
	`, providerTradeID, string(rawCallback), now, order.ID)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writePlainOK(w)
}

func (s *Server) handleAdminListBillingSettings(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	prices, err := s.serviceCreditPrices(ctx)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	plans, err := s.creditTopupPlans(ctx)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"creditPrices": publicServiceCreditPrices(prices),
		"topupPlans":   publicCreditTopupPlans(plans),
	})
}

func (s *Server) handleAdminUpsertCreditPrice(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}
	var body struct {
		ID          string `json:"id"`
		ServiceCode string `json:"serviceCode"`
		BillingKey  string `json:"billingKey"`
		CreditUnits int    `json:"creditUnits"`
		Enabled     *bool  `json:"enabled"`
		Note        string `json:"note"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	serviceCode := strings.TrimSpace(body.ServiceCode)
	billingKey := strings.ToUpper(strings.TrimSpace(body.BillingKey))
	if serviceCode == "" || billingKey == "" {
		errorJSON(w, http.StatusBadRequest, "服务代码和计费键为必填")
		return
	}
	if !supportedServiceCreditPrice(serviceCode, billingKey) {
		errorJSON(w, http.StatusBadRequest, "当前仅支持配置标清 1K 和高清 2K/4K 的 credits 价格")
		return
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		id = "service-credit-" + safeIDPart(serviceCode) + "-" + safeIDPart(billingKey)
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	cost := positiveOr(body.CreditUnits, 1)
	now := time.Now().UTC()
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	var price serviceCreditPrice
	err := s.db.QueryRow(ctx, `
		INSERT INTO service_credit_prices (id, service_code, billing_key, credit_units, enabled, note, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (service_code, billing_key) DO UPDATE SET
			credit_units = excluded.credit_units,
			enabled = excluded.enabled,
			note = excluded.note,
			updated_at = excluded.updated_at
		RETURNING id, service_code, billing_key, credit_units, enabled, note, created_at, updated_at
	`, id, serviceCode, billingKey, cost, enabled, strings.TrimSpace(body.Note), now, now).Scan(
		&price.ID,
		&price.ServiceCode,
		&price.BillingKey,
		&price.CreditUnits,
		&price.Enabled,
		&price.Note,
		&price.CreatedAt,
		&price.UpdatedAt,
	)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicServiceCreditPrice(price))
}

func (s *Server) handleAdminUpsertTopupPlan(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) || !s.requireDatabase(w) {
		return
	}
	var body struct {
		ID         string `json:"id"`
		Label      string `json:"label"`
		AmountUSDT string `json:"amountUsdt"`
		Credits    int    `json:"credits"`
		IsDefault  *bool  `json:"isDefault"`
		Enabled    *bool  `json:"enabled"`
		SortOrder  int    `json:"sortOrder"`
		Note       string `json:"note"`
	}
	if !readJSON(w, r, &body) {
		return
	}
	amount, err := strconv.ParseFloat(strings.TrimSpace(body.AmountUSDT), 64)
	if err != nil || amount <= 0 {
		errorJSON(w, http.StatusBadRequest, "充值金额无效")
		return
	}
	credits := positiveOr(body.Credits, 1)
	label := strings.TrimSpace(body.Label)
	if label == "" {
		label = fmt.Sprintf("%s USDT = %d credits", strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", amount), "0"), "."), credits)
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		id = "topup-usdt-" + safeIDPart(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", amount), "0"), ".")) + "-credits-" + strconv.Itoa(credits)
	}
	isDefault := false
	if body.IsDefault != nil {
		isDefault = *body.IsDefault
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	sortOrder := positiveOr(body.SortOrder, 100)
	now := time.Now().UTC()
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx)
	if isDefault {
		if _, err := tx.Exec(ctx, `UPDATE credit_topup_plans SET is_default = false WHERE id <> $1`, id); err != nil {
			errorJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	var plan creditTopupPlan
	err = tx.QueryRow(ctx, `
		INSERT INTO credit_topup_plans (id, label, amount_usdt, credits, is_default, enabled, sort_order, note, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			label = excluded.label,
			amount_usdt = excluded.amount_usdt,
			credits = excluded.credits,
			is_default = excluded.is_default,
			enabled = excluded.enabled,
			sort_order = excluded.sort_order,
			note = excluded.note,
			updated_at = excluded.updated_at
		RETURNING id, label, amount_usdt::text, credits, is_default, enabled, sort_order, note, created_at, updated_at
	`, id, label, amount, credits, isDefault, enabled, sortOrder, strings.TrimSpace(body.Note), now, now).Scan(
		&plan.ID,
		&plan.Label,
		&plan.AmountUSDT,
		&plan.Credits,
		&plan.IsDefault,
		&plan.Enabled,
		&plan.SortOrder,
		&plan.Note,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(ctx); err != nil {
		errorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, publicCreditTopupPlan(plan))
}

type epusdtTransactionResponse struct {
	TradeID    string
	PaymentURL string
	Raw        map[string]any
}

func (s *Server) createEPUSDTTransaction(ctx context.Context, payload map[string]any) (epusdtTransactionResponse, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.EPUSDT.BaseURL, "/")+"/payments/gmpay/v1/order/create-transaction", bytes.NewReader(body))
	if err != nil {
		return epusdtTransactionResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := s.upstreamClient
	if client == nil {
		client = newUpstreamHTTPClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return epusdtTransactionResponse{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return epusdtTransactionResponse{}, fmt.Errorf("EPUSDT 创建订单失败：HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded map[string]any
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return epusdtTransactionResponse{}, err
	}
	if code := intValue(decoded["status_code"]); code != 0 && code != http.StatusOK {
		message := strings.TrimSpace(stringValue(decoded["message"]))
		if message == "" {
			message = "EPUSDT 创建订单失败"
		}
		return epusdtTransactionResponse{}, errors.New(message)
	}
	data := decoded
	if nested, ok := decoded["data"].(map[string]any); ok {
		data = nested
	}
	return epusdtTransactionResponse{
		TradeID: strings.TrimSpace(stringValue(data["trade_id"])),
		PaymentURL: firstNonEmpty(
			strings.TrimSpace(stringValue(data["payment_url"])),
			strings.TrimSpace(stringValue(data["pay_url"])),
			strings.TrimSpace(stringValue(data["checkout_url"])),
			strings.TrimSpace(stringValue(data["url"])),
			strings.TrimSpace(stringValue(data["code_url"])),
		),
		Raw: decoded,
	}, nil
}

func (s *Server) creditTopupPlan(ctx context.Context, id string) (creditTopupPlan, error) {
	id = strings.TrimSpace(id)
	var plan creditTopupPlan
	query := `
		SELECT id, label, amount_usdt::text, credits, is_default, enabled, sort_order, note, created_at, updated_at
		FROM credit_topup_plans
		WHERE enabled = true
	`
	args := []any{}
	if id != "" {
		query += ` AND id = $1 ORDER BY sort_order, id LIMIT 1`
		args = append(args, id)
	} else {
		query += ` ORDER BY is_default DESC, sort_order, id LIMIT 1`
	}
	err := s.db.QueryRow(ctx, query, args...).Scan(
		&plan.ID,
		&plan.Label,
		&plan.AmountUSDT,
		&plan.Credits,
		&plan.IsDefault,
		&plan.Enabled,
		&plan.SortOrder,
		&plan.Note,
		&plan.CreatedAt,
		&plan.UpdatedAt,
	)
	return plan, err
}

func (s *Server) creditTopupPlans(ctx context.Context) ([]creditTopupPlan, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, label, amount_usdt::text, credits, is_default, enabled, sort_order, note, created_at, updated_at
		FROM credit_topup_plans
		ORDER BY enabled DESC, is_default DESC, sort_order, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plans := []creditTopupPlan{}
	for rows.Next() {
		var plan creditTopupPlan
		if err := rows.Scan(
			&plan.ID,
			&plan.Label,
			&plan.AmountUSDT,
			&plan.Credits,
			&plan.IsDefault,
			&plan.Enabled,
			&plan.SortOrder,
			&plan.Note,
			&plan.CreatedAt,
			&plan.UpdatedAt,
		); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}

func (s *Server) serviceCreditPrices(ctx context.Context) ([]serviceCreditPrice, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, service_code, billing_key, credit_units, enabled, note, created_at, updated_at
		FROM service_credit_prices
		WHERE (service_code = 'image-2-sd' AND billing_key = '1K')
			OR (service_code = 'image-2-hd' AND billing_key = 'HD')
		ORDER BY service_code, billing_key
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prices := []serviceCreditPrice{}
	for rows.Next() {
		var price serviceCreditPrice
		if err := rows.Scan(
			&price.ID,
			&price.ServiceCode,
			&price.BillingKey,
			&price.CreditUnits,
			&price.Enabled,
			&price.Note,
			&price.CreatedAt,
			&price.UpdatedAt,
		); err != nil {
			return nil, err
		}
		prices = append(prices, price)
	}
	return prices, rows.Err()
}

func (s *Server) requireEPUSDTConfig() error {
	if strings.TrimSpace(s.cfg.EPUSDT.BaseURL) == "" {
		return fmt.Errorf("EPUSDT_BASE_URL is not configured")
	}
	if strings.TrimSpace(s.cfg.EPUSDT.PID) == "" {
		return fmt.Errorf("EPUSDT_PID is not configured")
	}
	if strings.TrimSpace(s.cfg.EPUSDT.SecretKey) == "" {
		return fmt.Errorf("EPUSDT_SECRET_KEY is not configured")
	}
	return nil
}

func epusdtSignature(params map[string]any, secret string) string {
	pairs := make([]string, 0, len(params))
	for key, raw := range params {
		if key == "signature" || raw == nil {
			continue
		}
		value := strings.TrimSpace(stringValue(raw))
		if value == "" {
			continue
		}
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)
	sum := md5.Sum([]byte(strings.Join(pairs, "&") + secret))
	return hex.EncodeToString(sum[:])
}

func randomPaymentOrderID() (string, error) {
	token, err := randomToken("LILPAY", 12)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(token, "_", ""), nil
}

func writePlainOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func validateEPUSDTPaidCallback(order paymentOrder, payload map[string]any, cfg config.EPUSDTConfig) error {
	if intValue(payload["status"]) != 2 {
		return errors.New("EPUSDT 回调状态不是已支付")
	}
	if err := validateEPUSDTCallbackAmount(order.AmountUSDT, payload); err != nil {
		return err
	}
	if !optionalCallbackFieldMatches(payload, "pid", cfg.PID) {
		return errors.New("EPUSDT 回调商户号与配置不一致")
	}
	if !optionalCallbackFieldMatches(payload, "currency", cfg.Currency) {
		return errors.New("EPUSDT 回调币种与配置不一致")
	}
	if !optionalCallbackFieldMatches(payload, "token", cfg.Token) {
		return errors.New("EPUSDT 回调代币与配置不一致")
	}
	if !optionalCallbackFieldMatches(payload, "network", cfg.Network) {
		return errors.New("EPUSDT 回调网络与配置不一致")
	}
	callbackTradeID := strings.TrimSpace(stringValue(payload["trade_id"]))
	if strings.TrimSpace(order.ProviderTradeID) != "" && callbackTradeID != "" && callbackTradeID != strings.TrimSpace(order.ProviderTradeID) {
		return errors.New("EPUSDT 回调交易号与订单不一致")
	}
	return nil
}

func validateEPUSDTCallbackAmount(orderAmount string, payload map[string]any) error {
	if payloadFieldHasValue(payload, "amount") {
		if !decimalStringEqual(stringValue(payload["amount"]), orderAmount) {
			return errors.New("EPUSDT 回调金额与订单不一致")
		}
		return nil
	}
	if payloadFieldHasValue(payload, "actual_amount") {
		if !decimalStringEqual(stringValue(payload["actual_amount"]), orderAmount) {
			return errors.New("EPUSDT 回调金额与订单不一致")
		}
		return nil
	}
	return errors.New("EPUSDT 回调缺少金额")
}

func optionalCallbackFieldMatches(payload map[string]any, key string, expected string) bool {
	if strings.TrimSpace(expected) == "" || !payloadFieldHasValue(payload, key) {
		return true
	}
	actual := strings.TrimSpace(stringValue(payload[key]))
	if key == "network" {
		return normalizePaymentNetwork(actual) == normalizePaymentNetwork(expected)
	}
	return strings.EqualFold(actual, strings.TrimSpace(expected))
}

func payloadFieldHasValue(payload map[string]any, key string) bool {
	raw, ok := payload[key]
	return ok && strings.TrimSpace(stringValue(raw)) != ""
}

func decimalStringEqual(a string, b string) bool {
	left, ok := decimalRat(a)
	if !ok {
		return false
	}
	right, ok := decimalRat(b)
	if !ok {
		return false
	}
	return left.Cmp(right) == 0
}

func decimalRat(value string) (*big.Rat, bool) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return nil, false
	}
	rat, ok := new(big.Rat).SetString(normalized)
	return rat, ok
}

func normalizePaymentNetwork(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "")
	normalized = strings.ReplaceAll(normalized, "_", "")
	switch normalized {
	case "trc20":
		return "tron"
	case "erc20":
		return "ethereum"
	default:
		return normalized
	}
}

func epusdtPaymentStatusName(status int) string {
	switch status {
	case 1:
		return "pending"
	case 2:
		return "paid"
	case 3:
		return "expired"
	default:
		return "callback_" + strconv.Itoa(status)
	}
}

func publicServiceCreditPrices(prices []serviceCreditPrice) []map[string]any {
	result := make([]map[string]any, 0, len(prices))
	for _, price := range prices {
		result = append(result, publicServiceCreditPrice(price))
	}
	return result
}

func publicServiceCreditPrice(price serviceCreditPrice) map[string]any {
	return map[string]any{
		"id":          price.ID,
		"serviceCode": price.ServiceCode,
		"billingKey":  price.BillingKey,
		"creditUnits": price.CreditUnits,
		"enabled":     price.Enabled,
		"note":        price.Note,
		"createdAt":   price.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt":   price.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func publicCreditTopupPlans(plans []creditTopupPlan) []map[string]any {
	result := make([]map[string]any, 0, len(plans))
	for _, plan := range plans {
		result = append(result, publicCreditTopupPlan(plan))
	}
	return result
}

func publicCreditTopupPlan(plan creditTopupPlan) map[string]any {
	return map[string]any{
		"id":         plan.ID,
		"label":      plan.Label,
		"amountUsdt": plan.AmountUSDT,
		"credits":    plan.Credits,
		"isDefault":  plan.IsDefault,
		"enabled":    plan.Enabled,
		"sortOrder":  plan.SortOrder,
		"note":       plan.Note,
		"createdAt":  plan.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updatedAt":  plan.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func safeIDPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	result := strings.Trim(b.String(), "-")
	if result == "" {
		return "item"
	}
	return result
}

func stringValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	case json.Number:
		return v.String()
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}

func intValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		i, _ := strconv.Atoi(v.String())
		return i
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(v))
		return i
	default:
		return 0
	}
}
