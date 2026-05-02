ALTER TABLE license_keys
  ADD COLUMN IF NOT EXISTS service_code TEXT,
  ADD COLUMN IF NOT EXISTS credits INTEGER,
  ADD COLUMN IF NOT EXISTS redeemed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS redeemed_wallet_id TEXT REFERENCES wallets(id) ON DELETE SET NULL;

UPDATE license_keys
SET service_code = CASE WHEN tier = 'hd' THEN 'image-2-hd' ELSE 'image-2-sd' END
WHERE service_code IS NULL OR service_code = '';

UPDATE license_keys
SET credits = total_credits
WHERE credits IS NULL OR credits <= 0;

ALTER TABLE license_keys
  ALTER COLUMN service_code SET DEFAULT 'image-2-sd',
  ALTER COLUMN credits SET DEFAULT 5;

CREATE INDEX IF NOT EXISTS idx_license_keys_redeemed_wallet
  ON license_keys (redeemed_wallet_id)
  WHERE redeemed_wallet_id IS NOT NULL;
