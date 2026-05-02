package db

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalletMigrationDefinesWalletEntitlementAndRedemptionTables(t *testing.T) {
	sql := readMigration(t, "0005_wallets.sql")
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS wallets",
		"address TEXT NOT NULL UNIQUE",
		"recovery_hash TEXT NOT NULL UNIQUE",
		"CREATE TABLE IF NOT EXISTS wallet_entitlements",
		"UNIQUE (wallet_id, service_code)",
		"CREATE TABLE IF NOT EXISTS wallet_redemptions",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
	if strings.Contains(strings.ToLower(sql), "recovery_code") || strings.Contains(strings.ToLower(sql), "recovery_plaintext") {
		t.Fatalf("migration appears to store plaintext recovery material")
	}
}

func readMigration(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	return string(data)
}
