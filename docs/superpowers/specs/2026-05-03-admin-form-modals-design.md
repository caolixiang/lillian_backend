# Admin Form Modals Design

## Goal

Reduce persistent admin page clutter by moving service provider, credit price, and top-up plan create/edit forms into modal dialogs.

## Design

- Add one reusable modal shell with title, close button, body slot, and existing message areas inside forms.
- Move the existing provider, credit price, and top-up plan forms into hidden modal bodies in the page markup.
- Keep the existing form IDs and submit handlers where practical so API behavior stays unchanged.
- Replace persistent inline form visibility with modal state:
  - `新增服务商` opens the provider modal with defaults.
  - service provider `编辑` opens the provider modal with the selected row loaded.
  - `配置价格` opens the credit price modal with defaults.
  - credit price `编辑` opens the credit price modal with selected row values.
  - `新增套餐` opens the top-up plan modal with defaults.
  - top-up plan `编辑` opens the top-up plan modal with selected row values.
- Close the modal on successful save, overlay click, close button, or Escape.
- Leave backend APIs unchanged.

## Verification

- `npm run build` in `web/admin`.
- `go test ./...` after admin build to verify embedded admin assets.
