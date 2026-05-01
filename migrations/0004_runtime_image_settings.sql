ALTER TABLE service_profiles
  ADD COLUMN IF NOT EXISTS max_concurrent INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS app_settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO app_settings (key, value, description)
VALUES
  ('image_global_concurrency', '6', 'Maximum number of image generation tasks running upstream at the same time.'),
  ('image_provider_default_concurrency', '2', 'Default per-provider upstream concurrency when a provider does not override it.'),
  ('upstream_timeout_seconds', '600', 'Timeout in seconds for one synchronous upstream image call and image retrieval.')
ON CONFLICT (key) DO NOTHING;

