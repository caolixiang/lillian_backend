CREATE TABLE IF NOT EXISTS license_keys (
  id TEXT PRIMARY KEY,
  key_hash TEXT NOT NULL UNIQUE,
  key_ciphertext TEXT,
  service_code TEXT NOT NULL DEFAULT 'image-2-sd',
  credits INTEGER NOT NULL DEFAULT 5,
  max_concurrent INTEGER NOT NULL DEFAULT 6,
  status TEXT NOT NULL DEFAULT 'active',
  expires_at TIMESTAMPTZ,
  redeemed_at TIMESTAMPTZ,
  redeemed_wallet_id TEXT,
  note TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS service_profiles (
  id TEXT PRIMARY KEY,
  label TEXT NOT NULL,
  tier_bucket TEXT NOT NULL CHECK (tier_bucket IN ('1k', 'hd')),
  api_base_url TEXT NOT NULL,
  api_key_ciphertext TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT 'gpt-image-2',
  api_mode TEXT NOT NULL DEFAULT 'images',
  priority INTEGER NOT NULL DEFAULT 100,
  status TEXT NOT NULL DEFAULT 'active',
  selection_count INTEGER NOT NULL DEFAULT 0,
  success_count INTEGER NOT NULL DEFAULT 0,
  failure_count INTEGER NOT NULL DEFAULT 0,
  last_selected_at TIMESTAMPTZ,
  last_failed_at TIMESTAMPTZ,
  disabled_until TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tasks (
  id TEXT PRIMARY KEY,
  wallet_id TEXT,
  service_code TEXT,
  credit_reserved BOOLEAN NOT NULL DEFAULT false,
  credit_charged BOOLEAN NOT NULL DEFAULT false,
  status TEXT NOT NULL DEFAULT 'queued',
  requested_size TEXT NOT NULL,
  service_profile TEXT NOT NULL REFERENCES service_profiles(id),
  request_json JSONB NOT NULL,
  outputs_json JSONB,
  actual_params_json JSONB,
  revised_prompts_json JSONB,
  error TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_created ON tasks(status, created_at);
CREATE INDEX IF NOT EXISTS idx_service_profiles_bucket ON service_profiles(tier_bucket, status, priority);
