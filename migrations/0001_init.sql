CREATE TABLE IF NOT EXISTS license_keys (
  id TEXT PRIMARY KEY,
  key_hash TEXT NOT NULL UNIQUE,
  key_ciphertext TEXT,
  tier TEXT NOT NULL DEFAULT 'basic',
  total_credits INTEGER NOT NULL,
  remaining_credits INTEGER NOT NULL,
  max_concurrent INTEGER NOT NULL DEFAULT 6,
  status TEXT NOT NULL DEFAULT 'active',
  expires_at TIMESTAMPTZ,
  note TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS activations (
  id TEXT PRIMARY KEY,
  license_key_id TEXT NOT NULL REFERENCES license_keys(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  label TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ
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
  license_key_id TEXT NOT NULL REFERENCES license_keys(id) ON DELETE CASCADE,
  activation_id TEXT NOT NULL REFERENCES activations(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT 'queued',
  tier TEXT NOT NULL,
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

CREATE TABLE IF NOT EXISTS credit_ledger (
  id TEXT PRIMARY KEY,
  license_key_id TEXT NOT NULL REFERENCES license_keys(id) ON DELETE CASCADE,
  task_id TEXT REFERENCES tasks(id) ON DELETE SET NULL,
  type TEXT NOT NULL,
  amount INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  note TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_activations_token_hash ON activations(token_hash);
CREATE INDEX IF NOT EXISTS idx_tasks_license_status ON tasks(license_key_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_status_created ON tasks(status, created_at);
CREATE INDEX IF NOT EXISTS idx_service_profiles_bucket ON service_profiles(tier_bucket, status, priority);
