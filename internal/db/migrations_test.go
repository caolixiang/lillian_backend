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

func TestLicenseWalletRedemptionMigrationAddsServiceAndRedeemColumns(t *testing.T) {
	sql := readMigration(t, "0006_license_wallet_redemption.sql")
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS service_code TEXT",
		"ADD COLUMN IF NOT EXISTS credits INTEGER",
		"ADD COLUMN IF NOT EXISTS redeemed_at TIMESTAMPTZ",
		"ADD COLUMN IF NOT EXISTS redeemed_wallet_id TEXT",
		"SET service_code = CASE WHEN tier = 'hd' THEN 'image-2-hd' ELSE 'image-2-sd' END",
		"SET service_code = 'image-2-sd'",
		"SET credits = total_credits",
		"SET credits = 5",
		"ADD CONSTRAINT license_keys_redeemed_wallet_id_fkey",
		"FOREIGN KEY (redeemed_wallet_id) REFERENCES wallets(id) ON DELETE SET NULL",
		"ALTER COLUMN service_code SET NOT NULL",
		"ALTER COLUMN credits SET NOT NULL",
		"idx_license_keys_redeemed_wallet",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}

func TestTaskWalletAccountingMigrationAddsTaskWalletFields(t *testing.T) {
	sql := readMigration(t, "0007_task_wallet_accounting.sql")
	for _, want := range []string{
		"column_name = 'license_key_id'",
		"ALTER TABLE tasks ALTER COLUMN license_key_id DROP NOT NULL",
		"column_name = 'activation_id'",
		"ALTER TABLE tasks ALTER COLUMN activation_id DROP NOT NULL",
		"ADD COLUMN IF NOT EXISTS wallet_id TEXT",
		"ADD COLUMN IF NOT EXISTS service_code TEXT",
		"ADD COLUMN IF NOT EXISTS credit_reserved BOOLEAN NOT NULL DEFAULT false",
		"ADD COLUMN IF NOT EXISTS credit_charged BOOLEAN NOT NULL DEFAULT false",
		"ADD CONSTRAINT tasks_wallet_id_fkey",
		"FOREIGN KEY (wallet_id) REFERENCES wallets(id) ON DELETE SET NULL",
		"idx_tasks_wallet_service_status",
		"idx_tasks_status_created_queued",
		"idx_tasks_running_global",
		"idx_tasks_service_profile_running",
		"idx_tasks_wallet_recent",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}

func TestInitialMigrationUsesWalletOnlyLicenseAndTaskSchema(t *testing.T) {
	sql := readMigration(t, "0001_init.sql")
	for _, want := range []string{
		"service_code TEXT NOT NULL DEFAULT 'image-2-sd'",
		"credits INTEGER NOT NULL DEFAULT 5",
		"wallet_id TEXT",
		"credit_reserved BOOLEAN NOT NULL DEFAULT false",
		"credit_charged BOOLEAN NOT NULL DEFAULT false",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("initial migration missing %q", want)
		}
	}
	for _, notWant := range []string{
		"CREATE TABLE IF NOT EXISTS activations",
		"CREATE TABLE IF NOT EXISTS credit_ledger",
		"activation_id TEXT",
		"license_key_id TEXT",
		"total_credits",
		"remaining_credits",
	} {
		if strings.Contains(sql, notWant) {
			t.Fatalf("initial migration still contains legacy schema %q", notWant)
		}
	}
}

func TestRemoveActivationTokenFlowMigrationDropsLegacyTablesAndColumns(t *testing.T) {
	sql := readMigration(t, "0008_remove_activation_token_flow.sql")
	for _, want := range []string{
		"DROP INDEX IF EXISTS idx_tasks_license_status",
		"DROP INDEX IF EXISTS idx_activations_token_hash",
		"DROP COLUMN IF EXISTS activation_id",
		"DROP COLUMN IF EXISTS license_key_id",
		"DROP TABLE IF EXISTS credit_ledger",
		"DROP TABLE IF EXISTS activations",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
}

func TestWalletCreditsPaymentMigrationsDefineGenericPricingAndTopups(t *testing.T) {
	sql := readMigration(t, "0009_wallet_credits_payments.sql") + "\n" + readMigration(t, "0010_credit_pricing_and_topup.sql") + "\n" + readMigration(t, "0011_unify_hd_credit_pricing.sql")
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS credits INTEGER NOT NULL DEFAULT 0",
		"ADD COLUMN IF NOT EXISTS credit_units INTEGER NOT NULL DEFAULT 1",
		"ADD COLUMN IF NOT EXISTS billing_source TEXT NOT NULL DEFAULT 'entitlement'",
		"CREATE TABLE IF NOT EXISTS payment_orders",
		"CREATE TABLE IF NOT EXISTS service_credit_prices",
		"UNIQUE (service_code, billing_key)",
		"CREATE TABLE IF NOT EXISTS credit_topup_plans",
		"('service-credit-image-2-hd-hd', 'image-2-hd', 'HD', 2",
		"billing_key IN ('1K', '2K', '4K')",
		"('topup-usdt-10-credits-200', '10 USDT = 200 credits', 10, 200",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
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
