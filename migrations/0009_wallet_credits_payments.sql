ALTER TABLE wallets
  ADD COLUMN IF NOT EXISTS credits INTEGER NOT NULL DEFAULT 0;

ALTER TABLE tasks
  ADD COLUMN IF NOT EXISTS credit_units INTEGER NOT NULL DEFAULT 1,
  ADD COLUMN IF NOT EXISTS billing_source TEXT NOT NULL DEFAULT 'entitlement';

CREATE TABLE IF NOT EXISTS payment_orders (
  id TEXT PRIMARY KEY,
  wallet_id TEXT NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
  provider TEXT NOT NULL DEFAULT 'epusdt',
  provider_order_id TEXT NOT NULL UNIQUE,
  provider_trade_id TEXT NOT NULL DEFAULT '',
  amount_usdt NUMERIC(12, 2) NOT NULL,
  credits INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'created',
  checkout_url TEXT NOT NULL DEFAULT '',
  raw_request JSONB,
  raw_response JSONB,
  raw_callback JSONB,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  paid_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_payment_orders_wallet_created
  ON payment_orders (wallet_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_payment_orders_status
  ON payment_orders (status, created_at);
