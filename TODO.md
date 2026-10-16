# Development checklist

## v0.1 — Steam daily dashboard

- [x] Save project goals, research, constraints and agent handoff instructions.
- [x] Set up Go + SQLite and three tables with schema versioning.
- [x] Implement Steam library sync, complete-response validation and idempotent upserts.
- [x] Add best-effort achievement sync and local artwork caching.
- [x] Preserve data on privacy errors, network errors and partial failures.
- [x] Expose read-only dashboard and library API with coverage and sync status.
- [x] Build responsive dashboard, recent covers, charts, search/filter/sort/paging.
- [x] Provide an explicitly separate demo mode and a useful real empty state.
- [x] Add local startup, environment example, stale-data refresh and sync command.
- [x] Verify backend behavior and frontend production build.
- [x] Verify layout in a browser (desktop and 390px iframe viewport).
- [x] Add in-page Steam Key / SteamID64 settings, atomic local persistence and immediate sync.
- [x] Verify no saved Key is returned, blank-key retention, account isolation and local write protection.
- [x] Fix Steam cover URLs that require a hashed asset directory; backfill missing covers.
- [x] Owner configured Steam locally; synced 64 games and checked NBA cover / achievement details.
- [x] Add persisted per-game achievement details, click-to-open archive, status filters and search.
- [ ] Complete broader account-level comparison of playtime, library scope and achievement gaps.

## v0.2 — PSN

- [ ] Validate NPSSO → tokens and local refresh behavior.
- [ ] Fetch played games with pagination, duration and recent-played timestamps.
- [ ] Fetch trophy sets; map exact title IDs and avoid shared-set double counting.
- [ ] Verify purchased-game list and subscription semantics separately.

## v0.3 — Epic

- [ ] Choose native integration or Legendary JSON import after an account probe.
- [ ] Import library, names and artwork; check Windows/Mac coverage.
- [ ] Filter DLC/tools; leave unsupported personal metrics null.

## Current handoff

- Updated 2026-10-16. Initial local snapshot includes the Steam dashboard, settings,
  artwork repair and per-game achievement archive; no remote push. Go 1.25.14,
  Node 22.23.0; dependencies pinned. Read README and docs/PROJECT.md first.
- Steam is configured locally by the owner. Do not read/print saved keys, return
  them via API or commit anything in ignored data/. Demo data stays frontend-only.
- User-facing service runs on http://127.0.0.1:8080. `npm start` builds + serves;
  `npm run sync` syncs Steam; `npm run covers` repairs only missing artwork.
- Steam dashboard, settings, artwork and achievement details are implemented.
  PSN/Epic remain future work. No daily-history chart or cloud deployment.
- Verification includes Go tests (also race), go vet, TS/Vite production build,
  real Steam sync and browser checks. Build has the known ~655KB chart bundle warning.
- Next work can continue from the owner's requests. Broader manual comparison of
  library/playtime/count semantics is still pending; artwork and detail checks do
  not constitute a full account audit.

### Cover repair follow-up (2026-10-16)

- Owner has configured Steam locally; Chrome shows 64 synced Steam entries.
- NBA 2K26 (3472040) had no cached cover. The guessed /apps/3472040/header.jpg
  returned 404; Steam store appdetails returned a working header_image containing
  an additional asset-hash directory. No credentials were needed for this lookup.
- Added store-metadata fallback only when a cover download fails. Cache source URL
  is persisted and no longer overwritten by guessed URLs on subsequent syncs.
- Added `npm run covers` to repair only missing cached artwork without touching
  game stats or reading saved Steam keys. Ran it locally: 3 repaired, 0 unavailable.
- Regression tests cover hashed URLs, persistent cache/source, lost cache retry,
  unavailable metadata preservation and trusted image-host handling. Go tests,
  go vet and frontend production build passed. Confirmed NBA artwork in Chrome.
- Remaining account verification: compare game stats/achievement gaps separately;
  successful cover repair does not establish all metric accuracy.
- Restarted only the Game Shelf IPv4 loopback service on :8080 to load the fix;
  left the unrelated IPv6 listener untouched. User-facing service remains running.
  Private verification screenshot is under ignored data/qa/covers-repaired.png.

### Per-game achievement archive (2026-10-16)

- Added schema v2: `achievement_details` stores a complete JSON snapshot per game.
  Database backup before migration is ignored `data/backups/before-achievement-details.db`.
- Steam schema + personal states are joined strictly by API name, using schinese.
  Stores name, description, color/gray icon, hidden flag, achieved and unlock time.
  Missing/invalid/mismatched personal states reject the snapshot; failures preserve
  previous details. Count-only fallback remains available. Unknown time stays null.
- Sync enriches games missing details even when count data is still fresh. Existing
  24h/playtime-change refresh applies after details exist. Read-only GET details
  uses local SQLite only; UI polls it while open, with a Steam-sync action if missing.
- Click recent cards or library names. Modal supports all/unlocked/locked filters,
  search, unlock dates, progress and explicit hidden-content reveal. Escape closes,
  keyboard focus stays inside and returns to the trigger. Demo has no invented details.
- Real sync: 62/64 games, 4,194 detail records. Half-Life and RPG Maker XP did not
  yield usable detail schemas, retained without inferred zero. NBA 2K26: 9/48,
  39 locked; Chinese names/descriptions and dates verified in browser along with
  filtering/search/hidden reveal. Icons are remote with fallback; text/state is local.
- Tests cover API-name joining despite different ordering, partial/missing/duplicate
  states, persistence/idempotence, v1 migration, null versus confirmed empty, HTTP
  404 and key redaction. See achievements_test.go.
- Final verification: `go test -race ./...`, `go vet ./...`, frontend TypeScript +
  production build passed. Browser checked desktop filters/search/reveal and 390px
  iframe layout. Private screenshot: ignored data/qa/achievement-details.png.
- The live :8080 service was restarted with the updated build and remains running.
