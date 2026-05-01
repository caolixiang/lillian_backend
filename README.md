# lillian_backend

Standalone Go backend for Lillian's Canvas.

The Cloudflare side should only serve the SPA and `/config.json`. This backend owns licenses, activations, image tasks, provider routing, and S3-compatible image storage.

## Current Scope

This first backend scaffold includes:

- Go HTTP server with `/health`, `/ready`, and `/config.json`
- Vite-built admin frontend embedded into the Go binary at `/admin`
- Postgres connection plumbing
- S3-compatible object storage client for R2, AWS S3, MinIO, Wasabi, Backblaze, and similar providers
- Initial Postgres schema for licenses, activations, tasks, credit ledger, and service profiles
- Lightweight Dockerfile, backend-only Docker Compose, and GitHub Actions build flow

Implemented admin/runtime endpoints:

- `GET /admin`
- `POST /admin/licenses`
- `GET /admin/licenses`
- `PATCH /admin/licenses/:id`
- `POST /admin/licenses/delete`
- `GET /admin/service-profiles`
- `POST /admin/service-profiles`
- `DELETE /admin/service-profiles/:id`
- `GET /admin/runtime-settings`
- `POST /admin/runtime-settings`
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
- `S3_ENDPOINT` - S3-compatible endpoint, for example an R2 or MinIO endpoint
- `S3_REGION` - S3 region, use `auto` for R2
- `S3_BUCKET` - image bucket
- `S3_ACCESS_KEY_ID` / `S3_SECRET_ACCESS_KEY` - S3 credentials
- `S3_PUBLIC_BASE_URL` - public image base URL, usually a custom domain or public bucket URL
- `S3_FORCE_PATH_STYLE` - set `true` for MinIO and many S3-compatible providers

Optional deployment overrides:

- `CORS_ORIGIN` - browser origin allowlist; defaults to `*`.
- `PUBLIC_API_BASE_URL` - public backend URL override; defaults to the current request's forwarded host/protocol.

Runtime image settings are stored in Postgres and edited from `/admin`, not in `.env`:

- Global image concurrency: total number of upstream synchronous image tasks allowed to run at once.
- Default provider concurrency: per-provider limit when the provider does not override it.
- Upstream timeout seconds: timeout for one synchronous generation call plus image retrieval.

## Local

```bash
cp .env.example .env
npm --prefix web/admin install
npm --prefix web/admin run build
docker compose up --build
```

The default Compose file starts only the backend container. Postgres and S3/R2 are external dependencies configured through `.env`; they are not bundled into the backend deployment.

For local development, point `DATABASE_URL` at a local Postgres, Railway Postgres, Neon/Supabase, or any reachable Postgres instance. Point `S3_*` at R2 or another S3-compatible bucket.

For backend-only local runs outside Docker, build the admin frontend once before `go run`:

```bash
npm --prefix web/admin run build
go run ./cmd/backend
```

Health:

```bash
curl http://127.0.0.1:8787/health
curl http://127.0.0.1:8787/ready
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

Railway can build the `Dockerfile` directly. The backend service is a single container; add Railway Postgres or another managed Postgres separately, set `DATABASE_URL`, and configure the S3 variables for R2 or another provider.

Use [.env.production.example](.env.production.example) as the minimal Railway variable template. See [docs/deployment.md](docs/deployment.md) for Railway, VPS Docker Compose, R2/S3, and Cloudflare SPA wiring.
