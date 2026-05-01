# Lillian Backend Deployment

This backend replaces the Cloudflare Worker backend. Cloudflare should serve only the SPA and `/config.json`; the SPA points to this backend through its configured API base URL.

## Deployment Order

1. Deploy the Go backend first.
2. Bind a backend domain, for example `https://api.example.com`.
3. Verify:
   - `GET /health`
   - `GET /ready`
   - `GET /admin`
4. Configure the SPA Worker/Pages `API_BASE_URL` or equivalent config value to the backend domain.
5. Redeploy the SPA.

## Deployment Model

The Docker image runs only the Lillian backend. It contains:

- the Go backend binary
- the built `/admin` frontend assets embedded in the binary
- SQL migration files

It does not start or bundle Postgres, MinIO, R2, or any queue service. Postgres and S3/R2 are external dependencies connected through environment variables. This keeps Railway, VPS, and other container platforms using the same simple deployment shape.

## Railway Required Variables

Railway should be the default target. Add Railway Postgres or use any managed Postgres connection string, then set the backend service variables below.

Railway usually injects `PORT` automatically. If Railway provides it, keep Railway's value; the `PORT=8787` line is mainly for VPS/Docker runs and keeps the env templates aligned.

Required:

```env
APP_ENV=production
PORT=8787

DATABASE_URL=${{Postgres.DATABASE_URL}}
AUTO_MIGRATE=true
MIGRATIONS_DIR=migrations

ADMIN_TOKEN=replace-with-long-random-admin-password
LICENSE_KEY_PEPPER=replace-with-long-random-stable-secret
PROVIDER_CREDENTIAL_SECRET=replace-with-long-random-stable-secret

S3_ENDPOINT=https://ACCOUNT_ID.r2.cloudflarestorage.com
S3_REGION=auto
S3_BUCKET=lillian-canvas-images
S3_ACCESS_KEY_ID=replace-with-access-key-id
S3_SECRET_ACCESS_KEY=replace-with-secret-access-key
S3_PUBLIC_BASE_URL=https://images.example.com
S3_FORCE_PATH_STYLE=true
```

These values have safe defaults in code but stay in the env templates so all three env files use the same shape:

- `AUTO_MIGRATE=true`
- `MIGRATIONS_DIR=migrations`
- `S3_REGION=auto`
- `S3_FORCE_PATH_STYLE=true`

These values are optional overrides and usually do not need to be set:

- `CORS_ORIGIN`: defaults to `*`. Set it only when you want to restrict browser callers to a specific frontend origin.
- `PUBLIC_API_BASE_URL`: defaults to the current request host/protocol. Set it only when a reverse proxy hides the real public backend URL and does not send standard forwarded headers.

`LICENSE_KEY_PEPPER` and `PROVIDER_CREDENTIAL_SECRET` must remain stable. Changing either one breaks lookup/decryption for existing exchange codes, activations, or service provider credentials.

## Railway

Railway can build this repository from the `Dockerfile`. The Docker build compiles the Vite admin frontend first, embeds the generated files into the Go binary, and then builds the backend service.

1. Create a Railway project from this repo.
2. Add Railway Postgres, or prepare another managed Postgres database.
3. Set `DATABASE_URL` to `${{Postgres.DATABASE_URL}}` or your managed Postgres connection string.
4. Set the required variables above.
5. Configure R2 or another S3-compatible provider with the `S3_*` variables.
6. Deploy.
7. Open `/ready`; it should return `{"ok":true,...}`.
8. Open `/admin` and log in with `ADMIN_TOKEN`.

Notes:

- You do not need to know the Railway/custom domain before the first deploy. `/config.json` and task image URLs derive their base URL from the incoming request when `PUBLIC_API_BASE_URL` is unset.
- CORS defaults to `*`, which fits this app because browser auth is sent through explicit bearer tokens rather than cookies. Restrict `CORS_ORIGIN` later only if you want a tighter browser origin policy.
- The backend runs migrations on startup by default. Set `AUTO_MIGRATE=false` only after you have a separate migration process.

## VPS With Docker Compose

For a VPS, use Docker Compose for the backend container only. Run Postgres separately, use a managed Postgres, or point at a database you already operate. Use R2 or another external S3-compatible bucket for images.

Example:

```yaml
services:
  backend:
    image: ghcr.io/YOUR_ORG/lillian-backend:0.1.0
    restart: unless-stopped
    env_file: .env
    ports:
      - "8787:8787"
```

Use a reverse proxy such as Caddy, Nginx, or Traefik for HTTPS and the public domain.

If Postgres runs on the same VPS, manage it as its own service outside this compose file and set `DATABASE_URL` to that service's reachable address. If the VPS is only for backend compute, do not store images on local disk. Keep image output in S3/R2 so the backend can be moved without migrating generated images.

## Cloudflare SPA Wiring

After the backend domain works:

1. Set the SPA API base URL to the backend domain, for example `https://api.example.com`.
2. Redeploy the SPA.
3. Open the SPA `/config.json` and confirm it returns the same backend URL.
4. Activate a test exchange code in the SPA.
5. Generate a 1K test image.
6. Generate a 2K/4K test image after configuring an HD provider.

The old Cloudflare backend Worker should not be on the image generation path once the SPA points to this Go backend.

## Admin Setup

Open:

```text
https://api.example.com/admin
```

Use `ADMIN_TOKEN` as the initial password.

Recommended first actions:

1. Create one 1K service provider.
2. Create one HD service provider.
3. Open "运行设置" and confirm the database-backed image settings:
   - 全局生图并发: total upstream synchronous image tasks allowed to run at the same time.
   - 默认服务商并发: per-provider concurrency when that provider's "最大并发" is `0`.
   - 上游超时秒数: timeout for one synchronous upstream generation and returned image retrieval.
4. Create a short-lived test exchange code.
5. Activate the test code in the SPA.
6. Generate one 1K image and one HD image.

## Operational Notes

- Startup ENV should be limited to infrastructure and secrets: Postgres, S3/R2, public URLs, admin password, and encryption/hash secrets.
- Image runtime knobs live in Postgres and are edited from `/admin`, so changing concurrency or timeout does not require changing `.env`.
- The backend starts a fixed worker pool internally, but a task can only be claimed when the database-backed global and provider concurrency limits allow it.
- Per-license concurrency is enforced by active queued/running task count and the exchange code's `max_concurrent`.
- Provider concurrency is enforced when queued tasks are claimed. A provider's "最大并发" of `0` means "use 默认服务商并发".
- 1K generation prefers 1K providers. HD keys can fall back to HD providers for 1K only when no active 1K provider is selected by priority/fallback rules.
- 2K/4K requires an HD license and an HD provider.
- If upstream generation fails before output is stored, the backend marks the task `error` and refunds the reserved credit.
- If upstream succeeds but the backend cannot download/store the output, the task is also marked `error` and credit is refunded. Provider-side billing may still happen; use reliable backend hosting and S3 storage.
