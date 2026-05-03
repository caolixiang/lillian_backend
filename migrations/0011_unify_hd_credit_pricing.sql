WITH hd_credit_price AS (
  SELECT COALESCE(
    (SELECT credit_units FROM service_credit_prices WHERE service_code = 'image-2-hd' AND billing_key = 'HD'),
    (SELECT credit_units FROM service_credit_prices WHERE service_code = 'image-2-hd' AND billing_key = '4K'),
    (SELECT credit_units FROM service_credit_prices WHERE service_code = 'image-2-hd' AND billing_key = '2K'),
    2
  ) AS credit_units
)
INSERT INTO service_credit_prices (id, service_code, billing_key, credit_units, enabled, note, created_at, updated_at)
SELECT
  'service-credit-image-2-hd-hd',
  'image-2-hd',
  'HD',
  credit_units,
  true,
  '默认高清 2K/4K credits 价格',
  NOW(),
  NOW()
FROM hd_credit_price
ON CONFLICT (service_code, billing_key) DO UPDATE SET
  enabled = true,
  updated_at = NOW();

UPDATE service_credit_prices
SET
  enabled = false,
  note = CASE
    WHEN note = '' THEN '已合并到 HD credits 价格'
    ELSE note
  END,
  updated_at = NOW()
WHERE service_code = 'image-2-hd'
  AND billing_key IN ('1K', '2K', '4K');
