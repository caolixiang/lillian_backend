# Image Credit Pricing Design

## Goal

Simplify image generation credit pricing so admin operators edit only price values that the backend actually consumes.

## Current Problem

The database uses a generic `service_credit_prices(service_code, billing_key)` table. That is useful internally, but the admin UI exposes the raw keys as free-form inputs. Operators can enter arbitrary service codes that no backend code uses. The migration also seeds separate `image-2-hd / 2K` and `image-2-hd / 4K` rows even though the current business intent is one HD price for both 2K and 4K.

## Design

- Keep the generic table and admin API shape so future services can still add real keys deliberately.
- For current image generation billing, use these supported rows:
  - `image-2-sd / 1K`: standard 1K image credit price.
  - `image-2-hd / HD`: shared HD price for both 2K and 4K image generation.
- Map requested `2K` and `4K` tasks to the `HD` billing key when billing credits.
- Change the admin UI to a preset selector plus editable `credits/status/note` fields instead of free-form `serviceCode` and `billingKey` inputs.
- Hide obsolete `image-2-hd / 1K`, `image-2-hd / 2K`, and `image-2-hd / 4K` rows from the normal admin list after migration by disabling them. They can remain in the table for historical compatibility, but active billing and normal UI focus on the supported rows.

## Data Migration

Update the existing credit-pricing migration so a fresh database seeds the supported rows. Add a follow-up migration for existing deployments that inserts/upserts `image-2-hd / HD` using the existing 4K price when available, otherwise 2K, otherwise default `2`, and disables legacy `2K`/`4K` rows.

## Testing

- Unit test that `billingKeyForImageGeneration("2K")` and `billingKeyForImageGeneration("4K")` return `HD`.
- Migration test that the new migration exists and disables legacy HD split rows.
- Admin frontend build to catch TypeScript/template issues.
