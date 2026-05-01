CREATE INDEX IF NOT EXISTS idx_license_keys_list_created
  ON license_keys (created_at DESC, id)
  WHERE status <> 'deleted';

