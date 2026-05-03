# Image Credit Pricing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Simplify image credit pricing so 2K/4K share one HD price and admin operators edit only supported image price rows.

**Architecture:** Keep the generic Postgres pricing table and admin API, but constrain the current UI to a preset set of supported image price keys. Backend credit billing maps requested HD sizes to one `HD` billing key, while migrations seed and migrate existing data into that key.

**Tech Stack:** Go HTTP API, pgx/Postgres SQL migrations, TypeScript/Vite admin frontend.

---

### Task 1: Backend Billing Key And Migration Tests

**Files:**
- Modify: `internal/httpapi/task_wallet_test.go`
- Modify: `internal/db/migrations_test.go`

- [ ] Update `TestCreditBillingForImageGenerationUsesConfiguredPrices` to expect `billingKeyForImageGeneration("2K") == "HD"` and `billingKeyForImageGeneration("4K") == "HD"`.
- [ ] Update migration content assertions to require an `image-2-hd / HD` seed and a migration that disables legacy `2K`/`4K` rows.
- [ ] Run `go test ./internal/httpapi -run TestCreditBillingForImageGenerationUsesConfiguredPrices` and confirm it fails before implementation.
- [ ] Run `go test ./internal/db` and confirm it fails before implementation.

### Task 2: Backend Billing And Migrations

**Files:**
- Modify: `internal/httpapi/tasks.go`
- Modify: `migrations/0010_credit_pricing_and_topup.sql`
- Create: `migrations/0011_unify_hd_credit_pricing.sql`
- Modify if needed: `README.md`, `docs/deployment.md`

- [ ] Change `billingKeyForImageGeneration` so `2K` and `4K` return `HD`.
- [ ] Update fresh seed rows to include `image-2-hd / HD` and stop seeding separate `2K`/`4K` rows.
- [ ] Add follow-up migration that inserts/upserts `image-2-hd / HD` from existing 4K/2K cost and disables legacy `1K`/`2K`/`4K` HD rows.
- [ ] Update docs that mention `1K/2K/4K` pricing to the new supported key shape.
- [ ] Run targeted Go tests and verify they pass.

### Task 3: Admin UI Preset Pricing Editor

**Files:**
- Modify: `web/admin/src/main.ts`

- [ ] Replace free-form service-code/billing-key inputs with a preset selector for supported image price rows.
- [ ] Keep the submitted API payload using the selected row's `serviceCode` and `billingKey`.
- [ ] Render service rows with user-facing labels and hide disabled legacy `2K`/`4K` rows from the normal table.
- [ ] Update edit behavior so clicking a row loads its preset, credits, status, and note.
- [ ] Run `npm run build` in `web/admin`.

### Task 4: Full Verification And Commit

**Files:**
- Modify: `docs/plan/20260503.md`
- Modify: `docs/memo/20260503.md`

- [ ] Run `go test ./...`.
- [ ] Run `git diff --check`.
- [ ] Update today's plan and memo with test evidence.
- [ ] Commit the phase code changes, excluding today's plan/memo from the commit.
