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

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}
