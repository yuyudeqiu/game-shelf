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

- [x] Implement NPSSO → tokens and local refresh; verify with simulated Sony endpoints.
- [x] Owner configured PSN locally; first real library/trophy sync and group UI verified.
- [ ] Verify real refresh after token expiry and broader account data accuracy.
- [x] Implement paginated played games, ISO duration, recent time, edition and local cover caching.
- [x] Map exact title IDs to trophy sets; count shared sets once and persist trophy details.
- [x] Add PSN settings tab, local token storage, account binding and sync status.
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
  PSN has completed a first live sync and trophy group verification; Epic remains future work.
  No daily-history chart or cloud deployment.
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

### PSN integration (2026-10-16)

- Implemented Go-native unofficial PSN auth, token refresh, identity binding, played
  list pagination, trophy mapping/counts/details, PS4/PS5 edition and cover caching.
  No new database migration or runtime dependency. See README for exact scope.
- `PSNSettings` stores only verified account ID and renewable tokens in ignored
  psn-settings.json (0600). NPSSO is exchanged once, not saved. Local-only API
  routes are /api/settings/psn and /api/sync/psn. Never read/print real tokens.
- Frontend: platform settings has Steam / PlayStation tabs, Sony sign-in and NPSSO
  page links, password input and connect/sync controls. Common game modal supports
  PSN trophy grades, filtering, hidden reveal and unlock dates.
- CLI: `npm run sync:psn`. Independent per-platform workers prevent PSN failures
  from delaying Steam. Steam command/settings behavior preserved.
- Tests pass: Go race suite, go vet, TS/Vite build. Fixtures cover auth grant,
  automatic refresh/persistence, null vs known zero, pagination interruption and
  duplicate pages, exact trophy joins, shared-set counts, stale detail refresh,
  invalid replacement preservation, account isolation and local-write redaction.
- UI verified live with existing Steam data and empty PSN settings. Service :8080
  restarted and remains running. Private screenshot: data/qa/psn-settings.png.
- No real PSN credentials were available or used. NEXT: owner opens platform
  settings → PlayStation, signs in to Sony in their browser, gets NPSSO and pastes
  locally. Check real auth, several PS4/PS5 titles, counts/details, playtime and
  pagination. Do not claim real-account sync verified until that succeeds.
- No complete purchased/subscription library and no validated PS3/Vita coverage.
  Absent/ambiguous trophy mappings stay unknown. API changes may require follow-up.
- Last committed baseline: 599d765. PSN changes are included in the follow-up feature snapshot.

### PSN trophy groups and grades (2026-10-16)

- Owner confirmed PSN connected successfully. Local data has 9 PSN entries and
  5 complete trophy detail snapshots. Initial "no credentials available" notes
  above describe the earlier handoff and are superseded by this follow-up.
- Added native getTitleTrophyGroups metadata lookup, cached once per shared set
  per run. SQLite v3 adds groups_json/groups_updated_at to achievement_details.
  Migration preserves old games/items/counts; backup is ignored
  data/backups/before-trophy-groups.db. Metadata failure preserves prior names;
  metadata success leaves personal unlock snapshot timestamps unchanged.
- Added PSNTrophyBrowser: content groups (base/default, named additional content),
  per-group unlocked/total counts and percentages, platinum/gold/silver/bronze
  counts and filters, composed with unlock state and search. Lists also display
  group headings. Missing names fall back to numbered groups; unknown ID/type
  stays unknown. Percentages use trophy counts, not inferred platform weighting.
- Real backfill: 16 content groups across 5 games. Ghost of Tsushima has 6 groups:
  main 41/52, Legends 0/6, New Game+ 0/2, Iki Story 4/7, Iki Exploration 1/6,
  Rivals/Mastery 0/4. Overall 46/77. UI verified selecting Iki Story then Gold
  yields one locked trophy and filter counters 1/0/1. No credentials were printed.
- Checks passed: Go race tests, go vet and TypeScript/Vite build. Tests cover v2→v3
  migration, metadata persistence/failure preservation and separate freshness.
- Live :8080 server restarted and remains running; triggered one normal PSN sync.
  All 5 mapped titles got group metadata; 4 unmapped entries remain unchanged.
  Existing library currently includes non-game PSN apps and two Genshin entries;
  filtering apps / assessing title-scope semantics remains a separate follow-up.
- Private screenshot: data/qa/psn-trophy-groups.png. Updated README/PROJECT/AGENTS.
  PSN feature changes are included in the follow-up snapshot after baseline 599d765.

### PSN Chinese localization fix (2026-10-16)

- Root cause: zh-CN is not the working Sony trophy locale. An opt-in transient
  probe with the configured account confirmed zh-CN/zh-TW returned English,
  zh-Hans Simplified, zh-Hant Traditional. Probe logged only sample trophy text,
  no credentials; its temporary test file was removed.
- Changed default Accept-Language to zh-Hans (Steam already uses schinese).
  Added RefreshDetails + CLI `npm run sync:psn -- -refresh-details` to bypass
  fresh cached texts without falsifying old timestamps or losing failure protection.
- Ran the forced sync locally, then restarted the updated :8080 service. All 301
  trophy names across the 5 mapped games now contain Chinese; descriptions and
  additional group names were refreshed too. Some proper game titles stay English
  exactly as Sony returns them. Four unmapped PSN entries remain unchanged.
- Compared every trophy's ID, unlock state/date, grade and group ID before/after:
  unchanged. Baseline is ignored data/qa/psn-states-before-language.json.
- Go race tests, go vet and frontend build passed. New regression checks explicit
  refresh versus normal caching and failure preservation; fixtures enforce zh-Hans.
  Browser verified Chinese names/descriptions/groups. Screenshot is ignored
  data/qa/psn-chinese-trophies.png. Service remains running; included in the follow-up feature snapshot.

### Bulk hidden-content reveal (2026-10-16)

- Added a game-wide show/hide-all control below the detail progress bar, shared by
  Steam and PSN. Only displayed when the game has locked hidden achievements.
- Individual reveal remains supported. Reveal IDs now live in the parent dialog,
  so group/state filters do not reset them. Revealed names/descriptions participate
  in search; hide-all clears individual reveals too. Closing resets the dialog.
- Frontend-only state, no persistence or sync changes. Existing service serves the
  rebuilt assets immediately; no restart needed. Go tests and frontend build passed.
- Browser verified all 7 hidden Ghost of Tsushima trophies reveal together, remain
  visible across group changes, collapse together, and can be searched by actual
  names (e.g. 猴子). Private screenshot: data/qa/bulk-hidden-content.png.
- README/PROJECT updated. Included in the follow-up snapshot with the PSN work.

### Commit and cleanup (2026-10-16)

- User requested committing the current implementation and closing test processes.
  Snapshot includes PSN integration, grouped/graded trophies, Chinese localization
  and the bulk hidden-content toggle, plus tests and updated handoff documentation.
- Latest checks already passed: Go tests/race, go vet, frontend TS/Vite build,
  browser checks for grouping, Chinese text and bulk reveal/search/collapse.
- Process/listener audit found no remaining test Go processes, Vite servers or
  temporary :8081/:5173 servers. Task-owned in-app browser test tabs are closed.
  Only the user-facing ./bin/gameshelf serve on 127.0.0.1:8080 remains running.
  Codex-managed tool infrastructure is not an application test server.
- Credentials, local databases, backups, cached media, QA screenshots, builds and
  dependencies remain ignored. No remote push was requested or performed.
