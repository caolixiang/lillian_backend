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

CREATE INDEX IF NOT EXISTS idx_tasks_status_created_queued
  ON tasks (status, created_at)
  WHERE status = 'queued';

CREATE INDEX IF NOT EXISTS idx_tasks_running_global
  ON tasks (status)
  WHERE status = 'running';

CREATE INDEX IF NOT EXISTS idx_tasks_service_profile_running
  ON tasks (service_profile, status)
  WHERE status = 'running';

CREATE INDEX IF NOT EXISTS idx_tasks_wallet_recent
  ON tasks (wallet_id, created_at DESC)
  WHERE wallet_id IS NOT NULL;
