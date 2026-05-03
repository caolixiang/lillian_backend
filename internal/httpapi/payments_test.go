package httpapi

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CookSleep/lillian_backend/internal/config"
)

func TestEPUSDTSignatureMatchesSortedParamsAndSkipsEmptyValues(t *testing.T) {
	params := map[string]any{
		"pid":          "1000",
		"order_id":     "ORD20260424001",
		"currency":     "cny",
		"token":        "usdt",
		"network":      "tron",
		"amount":       100,
		"notify_url":   "https://merchant.example.com/notify",
		"redirect_url": "",
		"signature":    "ignored",
	}
	got := epusdtSignature(params, "secret")
	want := md5Hex("amount=100&currency=cny&network=tron&notify_url=https://merchant.example.com/notify&order_id=ORD20260424001&pid=1000&token=usdt" + "secret")
	if got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestCreateEPUSDTTransactionDecodesPaymentURL(t *testing.T) {
	var received map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payments/gmpay/v1/order/create-transaction" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status_code": 200,
			"message":     "success",
			"data": map[string]any{
				"trade_id":    "T2026041612345678",
				"payment_url": "https://pay.example.com/pay/checkout-counter/T2026041612345678",
			},
		})
	}))
	defer upstream.Close()

	server := New(config.Config{
		EPUSDT: config.EPUSDTConfig{BaseURL: upstream.URL},
	}, nil, nil, nil)
	response, err := server.createEPUSDTTransaction(context.Background(), map[string]any{"order_id": "order-1"})
	if err != nil {
		t.Fatalf("create transaction: %v", err)
	}
	if received["order_id"] != "order-1" {
		t.Fatalf("upstream request = %#v", received)
	}
	if response.TradeID != "T2026041612345678" {
		t.Fatalf("trade id = %q", response.TradeID)
	}
	if response.PaymentURL != "https://pay.example.com/pay/checkout-counter/T2026041612345678" {
		t.Fatalf("payment url = %q", response.PaymentURL)
	}
}

func TestValidateEPUSDTPaidCallback(t *testing.T) {
	order := paymentOrder{
		ProviderTradeID: "T202605031234",
		AmountUSDT:      "10.00",
	}
	cfg := config.EPUSDTConfig{
		PID:      "merchant-1",
		Currency: "USDT",
		Token:    "USDT",
		Network:  "TRON",
	}
	tests := []struct {
		name    string
		payload map[string]any
		wantErr bool
	}{
		{
			name: "valid amount and asset",
			payload: map[string]any{
				"status":   2,
				"pid":      "merchant-1",
				"amount":   10,
				"currency": "usdt",
				"token":    "usdt",
				"network":  "tron",
				"trade_id": "T202605031234",
			},
		},
		{
			name: "valid actual amount",
			payload: map[string]any{
				"status":        2,
				"actual_amount": "10.0",
			},
		},
		{
			name: "reject unpaid status",
			payload: map[string]any{
				"status": 1,
				"amount": "10.00",
			},
			wantErr: true,
		},
		{
			name: "reject mismatched amount",
			payload: map[string]any{
				"status": 2,
				"amount": "9.99",
			},
			wantErr: true,
		},
		{
			name: "ignore mismatched actual amount when order amount matches",
			payload: map[string]any{
				"status":        2,
				"amount":        "10.00",
				"actual_amount": "9.99",
			},
		},
		{
			name: "accept trc20 network alias",
			payload: map[string]any{
				"status":  2,
				"amount":  "10.00",
				"network": "TRC20",
			},
		},
		{
			name: "reject missing amount",
			payload: map[string]any{
				"status": 2,
			},
			wantErr: true,
		},
		{
			name: "reject mismatched token",
			payload: map[string]any{
				"status": 2,
				"amount": "10.00",
				"token":  "trx",
			},
			wantErr: true,
		},
		{
			name: "reject mismatched network",
			payload: map[string]any{
				"status":  2,
				"amount":  "10.00",
				"network": "bsc",
			},
			wantErr: true,
		},
		{
			name: "reject mismatched trade id",
			payload: map[string]any{
				"status":   2,
				"amount":   "10.00",
				"trade_id": "T-other",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEPUSDTPaidCallback(order, tt.payload, cfg)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestDecimalStringEqual(t *testing.T) {
	if !decimalStringEqual("10", "10.00") {
		t.Fatalf("expected equivalent decimal strings")
	}
	if decimalStringEqual("10.01", "10.00") {
		t.Fatalf("expected different decimal strings")
	}
	if decimalStringEqual("", "10.00") {
		t.Fatalf("expected empty string to be invalid")
	}
}

func TestSupportedServiceCreditPriceOnlyAllowsBusinessBackedImageRows(t *testing.T) {
	tests := []struct {
		serviceCode string
		billingKey  string
		want        bool
	}{
		{serviceCodeImage2SD, "1K", true},
		{serviceCodeImage2HD, "1K", false},
		{serviceCodeImage2HD, "HD", true},
		{serviceCodeImage2HD, "2K", false},
		{serviceCodeImage2HD, "4K", false},
		{"tts-standard", "STANDARD", false},
	}

	for _, tt := range tests {
		if got := supportedServiceCreditPrice(tt.serviceCode, tt.billingKey); got != tt.want {
			t.Fatalf("supportedServiceCreditPrice(%q, %q) = %v", tt.serviceCode, tt.billingKey, got)
		}
	}
}

func TestPublicTopupPlansRouteExistsForFrontendSelection(t *testing.T) {
	server := New(config.Config{}, nil, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/topup-plans", nil)
	rec := httptest.NewRecorder()

	server.Handler().ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Fatalf("public topup plans route is not registered")
	}
}

func TestPublicCreditTopupPlansExposeFrontendFieldsOnly(t *testing.T) {
	plans := publicCreditTopupPlans([]creditTopupPlan{
		{ID: "plan-10", Label: "10 USDT", AmountUSDT: "10.00", Credits: 200, IsDefault: true, Enabled: true, SortOrder: 10, Note: "internal"},
	})
	if len(plans) != 1 {
		t.Fatalf("plans = %#v", plans)
	}
	plan := plans[0]
	for _, key := range []string{"id", "label", "amountUsdt", "credits", "isDefault"} {
		if _, ok := plan[key]; !ok {
			t.Fatalf("public plan missing %q: %#v", key, plan)
		}
	}
	for _, key := range []string{"enabled", "sortOrder", "note", "createdAt", "updatedAt"} {
		if _, ok := plan[key]; ok {
			t.Fatalf("public plan leaked admin field %q: %#v", key, plan)
		}
	}
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
