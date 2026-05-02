# Lillian Backend Wallet And Entitlement Plan

## Goal

Convert the backend from local activation-token based redemption into a server-side wallet and entitlement system.

The frontend is already moving toward this model:

- Users do not create accounts or log in.
- Users bind a local browser workspace to a wallet.
- The wallet has a public `0x...` address.
- A one-time recovery password can restore the wallet on another browser/device.
- Redemption codes add service entitlements to the wallet.
- Image generation consumes wallet entitlements on the backend, not local browser state.

## Current Product Model

### Wallet

A wallet is the anonymous account container.

- Public address format: `0x` + 40 hex chars.
- Recovery password is shown only once by the client/backend during creation.
- Backend must never store the recovery password in plaintext.
- A user may restore a wallet by submitting the recovery password.
- The wallet can hold multiple service entitlements.

### Service Entitlements

Initial services:

- `image-2-sd`: 标清 image generation, used for 1K.
- `image-2-hd`: HD image generation, used for 2K/4K and may also be consumed for 1K fallback.

Future services should be possible without changing the core wallet model:

- TTS
- STT
- Other image/video/text services

## Data Model

### `wallets`

Purpose: Stores anonymous wallet identity.

Suggested columns:

- `id` UUID primary key
- `address` text unique not null
- `recovery_hash` text unique not null
- `created_at` timestamptz not null
- `updated_at` timestamptz not null

Notes:

- Do not store recovery password plaintext.
- Use a server-side pepper from env for hashing recovery password.
- Address may be derived from recovery hash or generated independently.

### `wallet_entitlements`

Purpose: Stores service balances per wallet.

Suggested columns:

- `id` UUID primary key
- `wallet_id` UUID references `wallets(id)`
- `service_code` text not null
- `remaining` integer not null default 0
- `max_concurrent` integer not null default 6
- `created_at` timestamptz not null
- `updated_at` timestamptz not null

Unique key:

- `(wallet_id, service_code)`

Initial service codes:

- `image-2-sd`
- `image-2-hd`

### `license_keys`

Purpose: Existing redemption code table should be adapted to grant wallet entitlements.

Required semantics:

- Code format remains `LIL-...`.
- Redemption code has a service type.
- Redemption code has credit amount.
- Redemption code has max concurrent setting.
- Redemption code has expiry.
- Redemption code can be redeemed only once unless intentionally configured otherwise.
- Redemption code can have admin remark/nickname.

Suggested fields to ensure exist:

- `id`
- `code_hash`
- `code_plaintext` or encrypted plaintext if admin list must show original code
- `service_code`
- `credits`
- `max_concurrent`
- `expires_at`
- `redeemed_at`
- `redeemed_wallet_id`
- `remark`
- `created_at`
- `updated_at`

Security note:

- If the admin UI must display original redemption codes later, store encrypted plaintext using the existing provider credential encryption pattern.
- Otherwise store only hash and show the code only once after generation.

### `wallet_redemptions`

Purpose: Audits which wallet redeemed which code.

Suggested columns:

- `id` UUID primary key
- `wallet_id` UUID references `wallets(id)`
- `license_key_id` UUID references `license_keys(id)`
- `service_code` text not null
- `credits_added` integer not null
- `created_at` timestamptz not null

### `generation_tasks`

Purpose: Existing task tracking should carry wallet/service accounting state.

Suggested fields if not already present:

- `wallet_id`
- `service_code`
- `credit_reserved` boolean
- `credit_charged` boolean
- `status`
- `provider_id`
- `provider_request_id`
- `created_at`
- `started_at`
- `finished_at`
- `error`

## API Design

### `POST /api/wallets/create`

Creates a new wallet.

Request:

```json
{}
```

Response:

```json
{
  "wallet": {
    "address": "0x...",
    "entitlements": []
  },
  "recoveryCode": "LIL-WAL-...."
}
```

Rules:

- `recoveryCode` is returned once.
- Backend stores only `recovery_hash`.

### `POST /api/wallets/restore`

Restores a wallet by recovery password.

Request:

```json
{
  "recoveryCode": "LIL-WAL-...."
}
```

Response:

```json
{
  "wallet": {
    "address": "0x...",
    "entitlements": [
      {
        "serviceCode": "image-2-sd",
        "label": "标清",
        "remaining": 5,
        "maxConcurrent": 6
      }
    ]
  }
}
```

### `GET /api/wallets/:address`

Fetches wallet balance and entitlement state.

Used by frontend:

- When opening wallet modal.
- Before each generation request.
- After generation completion/failure.

Response shape should match restore response without `recoveryCode`.

### `POST /api/wallets/redeem`

Redeems a `LIL-...` code into a wallet.

Request:

```json
{
  "walletAddress": "0x...",
  "code": "LIL-..."
}
```

Response:

```json
{
  "wallet": {
    "address": "0x...",
    "entitlements": [
      {
        "serviceCode": "image-2-sd",
        "label": "标清",
        "remaining": 10,
        "maxConcurrent": 6
      }
    ]
  }
}
```

Rules:

- Validate code exists.
- Validate not expired.
- Validate not already redeemed.
- Add credits to `(wallet_id, service_code)`.
- Set or raise max concurrent to the license key's max concurrent, depending on desired product semantics.
- Record redemption audit.

### Image Generation Request Changes

Generation requests should carry wallet identity, not local activation token.

Suggested request fields:

```json
{
  "walletAddress": "0x...",
  "prompt": "...",
  "size": "1024x1824",
  "sizeTier": "1K",
  "quality": "auto",
  "outputFormat": "png"
}
```

Backend chooses service:

- 1K uses `image-2-sd` first.
- 1K may fall back to `image-2-hd` if no SD balance exists.
- 2K/4K requires `image-2-hd`.

## Accounting Semantics

### Reserve, Then Charge

Generation is long-running and upstream can succeed while local transport fails. Backend must be strict:

1. Validate wallet has usable entitlement.
2. Reserve one credit and one concurrency slot.
3. Call upstream provider.
4. Upload returned image to S3/R2-compatible storage.
5. Only after image is persisted and response is ready, mark credit as charged.
6. On upstream error, timeout, provider error, upload error, or internal error, release reservation and do not charge.

Do not charge the user if the backend did not successfully persist and return the image.

### Suggested Task Statuses

- `queued`
- `running`
- `succeeded`
- `failed`
- `released`

For the current synchronous backend, `queued` may not be needed immediately. It is still useful for future async jobs.

### Concurrency Control

Wallet-level rule:

- A wallet can have at most 6 concurrent generation tasks by default.
- Entitlement-level `max_concurrent` can override this per service.

Checks before provider call:

- Wallet has positive remaining credits for selected service.
- Running task count for wallet/service is below limit.
- Provider global concurrency limit is below limit.

Implementation options:

- DB transaction with row locks for entitlement rows.
- Separate task rows counted by `status = running`.
- Prefer DB-backed control so it works across Railway/VPS replicas.

## Provider Routing

Existing provider configuration should remain DB/admin managed.

Required behavior:

- Providers can serve one or more service codes.
- `image-2-sd` providers can be separate from `image-2-hd` providers.
- `image-2-hd` providers may also serve 1K fallback if configured.
- Provider selection should continue using the backend's chosen routing strategy.

Admin provider config should include:

- Enabled/disabled
- Service codes supported
- Base URL
- API key/credential
- Model
- Mode/provider type
- Priority
- Optional weight
- Timeout seconds

## Admin UI Changes

### Generate Redemption Code

Fields:

- Service: `image-2-sd 标清` or `image-2-hd 2k/4k`
- Credits, default 5
- Max concurrent, default 6
- Expiry days, default 30
- Remark/nickname
- Quantity

Generated result:

- Show original `LIL-...` codes.
- Support copy/export.

### Redemption Code List

Columns:

- Code
- Service
- Credits
- Max concurrent
- Expired? yes/no
- Redeemed? yes/no
- Redeemed wallet address
- Remark
- Created time

Actions:

- Search by code
- Edit remark
- Delete
- Multi-select delete

### Wallet Lookup

Add a wallet page or panel:

- Search by wallet address.
- Show entitlements list.
- Show redemption records.
- Show recent generation tasks.

## Frontend Compatibility Plan

Current frontend still has local activation records.

Migration approach:

1. Add wallet APIs to backend.
2. Update frontend wallet creation/restore to call backend.
3. Update frontend redeem flow to call `/wallets/redeem`.
4. Update generation flow to pass `walletAddress`.
5. Before every generation request, frontend calls wallet balance refresh.
6. After generation, frontend writes backend-returned entitlement state back to local IndexedDB.
7. Remove old activation-token local flow once wallet flow is stable.

No need to preserve old local activation compatibility unless explicitly required.

## Implementation Phases

### Phase 1 - Schema And Wallet APIs

- Add migrations for `wallets`, `wallet_entitlements`, `wallet_redemptions`.
- Implement recovery password generation and hashing.
- Implement create/restore/get wallet APIs.
- Add tests for wallet creation, restore, and no plaintext recovery storage.

Acceptance:

- Can create wallet and receive `0x...` address plus one-time recovery code.
- Can restore wallet from recovery code.
- Can fetch empty entitlements.

### Phase 2 - Redemption Into Wallet

- Add `service_code`, `redeemed_wallet_id`, and redemption semantics to license keys.
- Implement `/wallets/redeem`.
- Update admin code generation for service code.
- Update admin code list to show service and redeemed wallet.

Acceptance:

- Admin can create `image-2-sd` and `image-2-hd` codes.
- Wallet can redeem a code once.
- Wallet entitlements increase correctly.

### Phase 3 - Generation Accounting

- Change generation request to accept wallet address.
- Select service based on requested size/tier.
- Reserve credit/concurrency before provider call.
- Charge only after image is uploaded and response is ready.
- Release reservation on failures.

Acceptance:

- 1K consumes `image-2-sd` first.
- 1K can fall back to `image-2-hd`.
- 2K/4K consumes only `image-2-hd`.
- Failed generations do not charge credits.

### Phase 4 - Admin Wallet View

- Add wallet lookup.
- Show entitlements.
- Show redemption history.
- Show recent tasks.

Acceptance:

- Admin can inspect a wallet and understand its balances and usage history.

### Phase 5 - Frontend Integration Support

- Confirm response shapes match frontend needs.
- Add any missing fields for the SPA wallet modal.
- Keep endpoints stable for Cloudflare/Vercel frontend shell.

Acceptance:

- Current SPA can create/restore wallet, redeem codes, refresh balances, and generate images with wallet billing.

## Open Decisions

- Should `max_concurrent` be wallet-global, service-specific, or both?
- Should redeeming another code add credits only, or also override/raise `max_concurrent`?
- Should admin ever be able to recover/show original generated codes after creation?
- Should wallet address be derived deterministically from recovery password or generated independently?
- Should backend support async generation tasks later, even if upstream is synchronous today?

## Non-Goals For First Version

- No user login.
- No blockchain signing.
- No mnemonic phrase.
- No paid checkout integration.
- No separate account profile.
- No public wallet transfer.

