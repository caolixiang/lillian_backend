CREATE TABLE IF NOT EXISTS wallets (
  id TEXT PRIMARY KEY,
  address TEXT NOT NULL UNIQUE,
  recovery_hash TEXT NOT NULL UNIQUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS wallet_entitlements (
  id TEXT PRIMARY KEY,
  wallet_id TEXT NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
  service_code TEXT NOT NULL,
  remaining INTEGER NOT NULL DEFAULT 0,
  max_concurrent INTEGER NOT NULL DEFAULT 6,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (wallet_id, service_code)
);

CREATE TABLE IF NOT EXISTS wallet_redemptions (
  id TEXT PRIMARY KEY,
  wallet_id TEXT NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
  license_key_id TEXT NOT NULL REFERENCES license_keys(id) ON DELETE CASCADE,
  service_code TEXT NOT NULL,
  credits_added INTEGER NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wallets_address ON wallets(address);
CREATE INDEX IF NOT EXISTS idx_wallets_recovery_hash ON wallets(recovery_hash);
CREATE INDEX IF NOT EXISTS idx_wallet_entitlements_wallet ON wallet_entitlements(wallet_id);
CREATE INDEX IF NOT EXISTS idx_wallet_redemptions_wallet ON wallet_redemptions(wallet_id, created_at DESC);
