ALTER TABLE tasks
  ADD COLUMN IF NOT EXISTS billing_key TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS service_credit_prices (
  id TEXT PRIMARY KEY,
  service_code TEXT NOT NULL,
  billing_key TEXT NOT NULL,
  credit_units INTEGER NOT NULL DEFAULT 1,
  enabled BOOLEAN NOT NULL DEFAULT true,
  note TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (service_code, billing_key)
);

CREATE INDEX IF NOT EXISTS idx_service_credit_prices_enabled
  ON service_credit_prices (service_code, enabled, billing_key);

CREATE TABLE IF NOT EXISTS credit_topup_plans (
  id TEXT PRIMARY KEY,
  label TEXT NOT NULL,
  amount_usdt NUMERIC(12, 2) NOT NULL,
  credits INTEGER NOT NULL,
  is_default BOOLEAN NOT NULL DEFAULT false,
  enabled BOOLEAN NOT NULL DEFAULT true,
  sort_order INTEGER NOT NULL DEFAULT 100,
  note TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_credit_topup_plans_enabled
  ON credit_topup_plans (enabled, is_default, sort_order);

ALTER TABLE payment_orders
  ADD COLUMN IF NOT EXISTS plan_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_payment_orders_plan_created
  ON payment_orders (plan_id, created_at DESC);

INSERT INTO service_credit_prices (id, service_code, billing_key, credit_units, enabled, note, created_at, updated_at)
VALUES
  ('service-credit-image-2-hd-1k', 'image-2-hd', '1K', 1, true, '默认 1K credits 价格', NOW(), NOW()),
  ('service-credit-image-2-hd-2k', 'image-2-hd', '2K', 2, true, '默认 2K credits 价格', NOW(), NOW()),
  ('service-credit-image-2-hd-4k', 'image-2-hd', '4K', 2, true, '默认 4K credits 价格', NOW(), NOW()),
  ('service-credit-image-2-sd-1k', 'image-2-sd', '1K', 1, true, '默认标清 1K credits 价格', NOW(), NOW())
ON CONFLICT (service_code, billing_key) DO UPDATE SET
  credit_units = excluded.credit_units,
  enabled = excluded.enabled,
  note = excluded.note,
  updated_at = excluded.updated_at;

INSERT INTO credit_topup_plans (id, label, amount_usdt, credits, is_default, enabled, sort_order, note, created_at, updated_at)
VALUES
  ('topup-usdt-10-credits-200', '10 USDT = 200 credits', 10, 200, true, true, 10, '默认充值套餐', NOW(), NOW())
ON CONFLICT (id) DO UPDATE SET
  label = excluded.label,
  amount_usdt = excluded.amount_usdt,
  credits = excluded.credits,
  is_default = excluded.is_default,
  enabled = excluded.enabled,
  sort_order = excluded.sort_order,
  note = excluded.note,
  updated_at = excluded.updated_at;
