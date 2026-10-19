# Game Shelf — agent handoff

<!-- postman-cli:installation:start -->
Postman CLI is installed on this device, and can be used for API Engineering work.
<!-- postman-cli:installation:end -->

## Start here

Read `README.md`, `docs/PROJECT.md`, and `TODO.md` before making changes. Update the
TODO and its handoff section after each meaningful work session so a new chat can
continue without prior conversation history.

## Agreed scope

- Personal, local-only, read-only gaming dashboard. Go + SQLite backend.
- React + TypeScript + Vite + Tailwind + shadcn-style components + Recharts frontend.
- Steam first, PSN second, Epic library third. One account per platform.
- No cross-platform deduplication, user system, editing, recommendations, AI, cloud
  deployment, complex permissions, or elaborate domain architecture.
- Unknown values must remain null, never silently become zero. Totals include
  known values only and must expose coverage. Platform entries count separately.
- Keep previous successful data on sync failures; never delete on a partial list.
- The user requested in-page Steam settings. Accept user-entered credentials via
  local-only JSON POST, persist atomically to ignored data/settings.json (0600),
  and apply without restarting. Saved settings override initial env credentials.
  Never return stored keys to the frontend, print them, commit them, or request
  them in chat. Empty key on save preserves the existing key. Game data stays read-only.
- PSN uses an unofficial Sony API. NPSSO is exchanged once and never persisted;
  renewable tokens are saved atomically to ignored psn-settings.json (0600), never
  returned to the frontend or logged. Bind verified account ID before game writes.
  Keep played-list scope explicit, use exact title/trophy IDs, and count shared
  service:npCommunicationId sets once. Owner confirmed first real PSN sync; broader account accuracy and real token expiry
  refresh are still pending.
- PSN schema v4 retains source category and hides the live-confirmed media categories
  ps5_web_based_media_app / ps5_native_media_app from all game views/statistics and
  enrichment. Keep raw rows/snapshots and account isolation; missing category retains
  prior classification, unknown categories stay visible. Never filter by game name.
- Achievement details are complete per-game snapshots, joined by platform API name.
  Preserve previous details on missing/invalid personal states or failed schema requests.
  Keep unknown unlock times null; locked hidden content stays collapsed until revealed.
  PSN uses Accept-Language zh-Hans for Simplified Chinese; zh-CN falls back to English.
  PSN group metadata has separate timestamps (schema v3); updating group names must
  not make personal unlock snapshots appear fresher. Preserve names on failed fetches.
- Demo data must be explicitly labeled and must not enter the real database.
- No agent delegation unless explicitly requested by the user.

## Working rules

Use the Go standard library for HTTP/routing and a small SQL repository. Do not add
an ORM, job system, provider plugin framework, or authentication system to v0.1.
Verify persistence, null/zero semantics, idempotent sync and failure preservation
with meaningful tests. Run `go test ./...` and `npm --prefix web run build` before
handoff. Live account checks require locally configured credentials; clearly report
when only fixture-based checks were possible.
