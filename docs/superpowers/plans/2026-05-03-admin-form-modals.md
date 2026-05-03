# Admin Form Modals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Move admin create/edit forms for service providers, credit prices, and top-up plans into reusable modal dialogs.

**Architecture:** Keep the existing vanilla TypeScript admin app and API calls. Add one modal shell in `web/admin/src/main.ts`, move existing forms into modal body containers, and use small helper functions to open/close and reset/load each form type.

**Tech Stack:** TypeScript, Vite, CSS.

---

### Task 1: Modal Structure And Styles

**Files:**
- Modify: `web/admin/src/main.ts`
- Modify: `web/admin/src/styles.css`

- [ ] Add a reusable modal shell with title, close button, body wrapper, and overlay.
- [ ] Add hidden form containers for provider, credit price, and top-up plan forms.
- [ ] Add CSS for modal overlay, panel, header, body, responsive sizing, and focusable close affordance.

### Task 2: Move Forms Into Modal

**Files:**
- Modify: `web/admin/src/main.ts`

- [ ] Remove persistent provider form wrapper from the profile tab.
- [ ] Remove persistent credit price and top-up plan forms from settings body.
- [ ] Keep form IDs unchanged so existing submit handlers can be reused.
- [ ] Add toolbar buttons for opening credit price and top-up plan modals.

### Task 3: Wire Modal Behavior

**Files:**
- Modify: `web/admin/src/main.ts`

- [ ] Add `openModal` and `closeModal` helpers.
- [ ] Update provider reset/load helpers to open provider modal.
- [ ] Update credit price and top-up plan edit click handlers to open the corresponding modal.
- [ ] Close modal after successful provider/price/top-up saves.
- [ ] Add overlay click and Escape close behavior.

### Task 4: Verification

**Files:**
- Modify: `docs/plan/20260503.md`
- Modify: `docs/memo/20260503.md`

- [ ] Run `npm run build` in `web/admin`.
- [ ] Run `go test ./...` after admin build.
- [ ] Run `git diff --check`.
- [ ] Commit the phase code changes, excluding today's daily plan/memo.
