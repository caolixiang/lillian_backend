ALTER TABLE license_keys
  ADD COLUMN IF NOT EXISTS service_code TEXT,
  ADD COLUMN IF NOT EXISTS credits INTEGER,
  ADD COLUMN IF NOT EXISTS redeemed_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS redeemed_wallet_id TEXT;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_name = 'license_keys' AND column_name = 'tier'
  ) THEN
    UPDATE license_keys
    SET service_code = CASE WHEN tier = 'hd' THEN 'image-2-hd' ELSE 'image-2-sd' END
    WHERE service_code IS NULL OR service_code = '';
  END IF;

  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_name = 'license_keys' AND column_name = 'total_credits'
  ) THEN
    UPDATE license_keys
    SET credits = total_credits
    WHERE credits IS NULL OR credits <= 0;
  END IF;

  UPDATE license_keys
  SET service_code = 'image-2-sd'
  WHERE service_code IS NULL OR service_code = '';

  UPDATE license_keys
  SET credits = 5
  WHERE credits IS NULL OR credits <= 0;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'license_keys_redeemed_wallet_id_fkey'
  ) THEN
    ALTER TABLE license_keys
      ADD CONSTRAINT license_keys_redeemed_wallet_id_fkey
      FOREIGN KEY (redeemed_wallet_id) REFERENCES wallets(id) ON DELETE SET NULL;
  END IF;
END $$;

ALTER TABLE license_keys
  ALTER COLUMN service_code SET DEFAULT 'image-2-sd',
  ALTER COLUMN credits SET DEFAULT 5,
  ALTER COLUMN service_code SET NOT NULL,
  ALTER COLUMN credits SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_license_keys_redeemed_wallet
  ON license_keys (redeemed_wallet_id)
  WHERE redeemed_wallet_id IS NOT NULL;
