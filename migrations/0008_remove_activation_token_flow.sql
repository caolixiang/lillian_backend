DROP INDEX IF EXISTS idx_tasks_license_status;
DROP INDEX IF EXISTS idx_activations_token_hash;

ALTER TABLE tasks
  DROP COLUMN IF EXISTS activation_id,
  DROP COLUMN IF EXISTS license_key_id,
  DROP COLUMN IF EXISTS tier;

ALTER TABLE license_keys
  DROP COLUMN IF EXISTS tier,
  DROP COLUMN IF EXISTS total_credits,
  DROP COLUMN IF EXISTS remaining_credits;

DROP TABLE IF EXISTS credit_ledger;
DROP TABLE IF EXISTS activations;
