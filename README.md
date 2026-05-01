# lillian_backend

Standalone Go backend for Lillian's Canvas.

The Cloudflare side should only serve the SPA and `/config.json`. This backend owns licenses, activations, image tasks, provider routing, and S3-compatible image storage.

## Current Scope

This first backend scaffold includes:

- Go HTTP server with `/health`, `/ready`, and `/config.json`
- Postgres connection plumbing
- S3-compatible object storage client for R2, AWS S3, MinIO, Wasabi, Backblaze, and similar providers
- Initial Postgres schema for licenses, activations, tasks, credit ledger, and service profiles
- Dockerfile, Docker Compose, and GitHub Actions build flow

Implemented admin/runtime endpoints:

- `GET /admin`
- `POST /admin/licenses`
- `GET /admin/licenses`
- `PATCH /admin/licenses/:id`
- `POST /admin/licenses/delete`
- `GET /admin/service-profiles`
- `POST /admin/service-profiles`
- `DELETE /admin/service-profiles/:id`
- `POST /api/keys/activate`
- `GET /api/me/credits`
- `POST /api/tasks`
- `GET /api/tasks/:id`
- `GET /api/tasks/:id/images/:index`

## Environment

- `PORT` - listen port, default `8787`
- `DATABASE_URL` - Postgres DSN
- `AUTO_MIGRATE` - run bundled SQL migrations on startup, default `true`
- `MIGRATIONS_DIR` - migrations directory, default `migrations`
- `ADMIN_TOKEN` - admin bearer token
- `LICENSE_KEY_PEPPER` - stable secret used for license key hashing
- `PROVIDER_CREDENTIAL_SECRET` - stable secret used to encrypt provider credentials
- `UPSTREAM_TIMEOUT_SECONDS` - image provider request timeout, default `600`
- `TASK_POLL_INTERVAL_SECONDS` - future task worker poll interval, default `2`
- `TASK_WORKER_CONCURRENCY` - future task worker concurrency, default `2`
- `S3_ENDPOINT` - S3-compatible endpoint, for example an R2 or MinIO endpoint
- `S3_REGION` - S3 region, use `auto` for R2
- `S3_BUCKET` - image bucket
- `S3_ACCESS_KEY_ID` / `S3_SECRET_ACCESS_KEY` - S3 credentials
- `S3_PUBLIC_BASE_URL` - public image base URL, usually a custom domain or public bucket URL
- `S3_FORCE_PATH_STYLE` - set `true` for MinIO and many S3-compatible providers

## Local

```bash
cp .env.example .env
docker compose up --build
```

Compose publishes Postgres on host port `15432` by default to avoid colliding with a local Postgres on `5432`. Override `POSTGRES_PORT` if needed.

Health:

```bash
curl http://127.0.0.1:8787/health
curl http://127.0.0.1:8787/ready
```

MinIO console:

```text
http://127.0.0.1:9001
```

## R2 Example

```env
S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
S3_REGION=auto
S3_BUCKET=lillian-canvas-images
S3_ACCESS_KEY_ID=...
S3_SECRET_ACCESS_KEY=...
S3_PUBLIC_BASE_URL=https://images.example.com
S3_FORCE_PATH_STYLE=true
```

## Railway

Railway can build the `Dockerfile` directly. Add Postgres as a Railway service, set `DATABASE_URL`, and configure the S3 variables for R2 or another provider.

Use [.env.production.example](.env.production.example) as the minimal Railway variable template. See [docs/deployment.md](docs/deployment.md) for Railway, VPS Docker Compose, R2/S3, and Cloudflare SPA wiring.
