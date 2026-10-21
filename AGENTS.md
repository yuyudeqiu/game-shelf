# Game Shelf — working rules

<!-- postman-cli:installation:start -->
Postman CLI is installed on this device, and can be used for API Engineering work.
<!-- postman-cli:installation:end -->

## Start here

Read [README](README.md), [design decisions](docs/PROJECT.md) and [TODO](TODO.md)
before changes. After meaningful work, update TODO's current handoff in place;
keep pending work, current state and verification, not a growing session diary.

## Scope

- Personal, local-only, read-only dashboard; Go + SQLite and React / TypeScript /
  Vite / Tailwind / shadcn-style components. One account per platform.
- Steam first, PSN second, Epic next. No cross-platform deduplication, game editing,
  user system, recommendations, AI, cloud deployment, complex permissions or
  elaborate domain architecture.
- Use standard-library HTTP/routing and a small SQL repository. No ORM, job system,
  provider plugin framework or authentication system.
- No agent delegation unless explicitly requested by the user.

## Data and credentials

- Unknown remains NULL, never silently zero; totals expose coverage. Platform/title
  entries stay separate; shared trophy sets count once. No deletion on partial lists
  or failed syncs; preserve prior successful data and complete detail snapshots.
- Bind verified accounts before game writes; include retained hidden app rows in
  isolation checks. Exact title/API IDs only; no name-based mapping or name-based
  app filtering.
- Never return stored secrets to the frontend, log/commit them, or request credentials
  in chat. This applies to both Steam and PSN.
- Steam settings: local JSON POST, atomic ignored settings.json (0600), hot apply,
  saved values override initial env, empty Key retains the old Key.
- PSN: transient NPSSO exchange only; atomic ignored psn-settings.json (0600) for
  renewable tokens. If reauthentication is needed, tell the user.
- Keep hidden details collapsed until revealed and unknown unlock times NULL.
  PSN uses zh-Hans; group metadata freshness stays separate from personal snapshots.
  Application category rules and failure semantics are specified in PROJECT.md.
- Demo data must be labeled, isolated from the real database and free of invented details.

## Verification

Use meaningful tests for persistence, NULL/zero semantics, idempotence and failure
preservation. Run `go test ./...` and `npm --prefix web run build` before handoff.
Live account checks need locally configured credentials; distinguish fixture checks
from live verification. Current known gaps belong in TODO, not these working rules.
