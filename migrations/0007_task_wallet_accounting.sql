ALTER TABLE tasks
  ALTER COLUMN license_key_id DROP NOT NULL,
  ALTER COLUMN activation_id DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS wallet_id TEXT REFERENCES wallets(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS service_code TEXT,
  ADD COLUMN IF NOT EXISTS credit_reserved BOOLEAN NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS credit_charged BOOLEAN NOT NULL DEFAULT false;

CREATE INDEX IF NOT EXISTS idx_tasks_wallet_service_status
  ON tasks (wallet_id, service_code, status)
  WHERE wallet_id IS NOT NULL;
