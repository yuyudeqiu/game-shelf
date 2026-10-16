# Project decisions

Last updated: 2026-10-16 (Asia/Shanghai).

## Goal

A personal gaming dashboard worth opening daily: sync games from Steam, PSN and
Epic where possible, store locally, browse a polished read-only web dashboard.
The user approved starting implementation after research. Keep the scope small.

## Interface

- Top: per-platform sync status and last successful update.
- Summary: synced platform entries, known total playtime with coverage, earned
  achievements/trophies with platform breakdown and coverage.
- Recently played: cover cards ordered by actual last-played timestamps.
- Two charts: platform game counts and top five games by known playtime.
- Library: cover, name, platform/version, playtime, earned/total, last played.
  Name search, platform filter, sorting and pagination.
- Dark default, restrained platform colors, responsive layout and empty/error states.
- No fabricated daily activity charts; there is no historical snapshot table.

## Architecture

Go sync commands → SQLite ← read-only Go HTTP API ← React/Vite frontend.
Go serves built static files for local use; Vite proxies the API in development.
Listen on loopback by default. Cover images are cached locally when possible.
No cloud hosting, external metadata subscriptions, or OpenAI API.

The user subsequently requested browser-based Steam configuration. Platform
settings accept Key and SteamID64, store them atomically in ignored
data/settings.json (0600), and trigger background sync without restart. Read APIs
expose only has_key, SteamID and status. Empty Key retains the saved value. Initial
env configuration remains a fallback; saved UI settings take priority. JSON write
endpoints require a local Host/Origin and custom header, with no CORS allowance.
Existing Steam data prevents changing to another account in the same DATA_DIR.

Four tables: games, achievement_sets, sync_runs, achievement_details. Platform IDs are strings.
Schema v2 adds one per-game JSON snapshot table for the read-only achievement archive;
v1 data is migrated without resetting the library. Counts and detail snapshots are
saved atomically on complete responses. The API-name join must be complete before
replacing previous details; count-only fallback may still update summary counts.
Details contain localized names/descriptions, icons, hidden flags, personal unlock
state and optional unlock timestamps. No inferred partial progress or rarity.
Click recent/library games to open an accessible modal, filter/search achievements,
and explicitly reveal locked hidden content. Demo mode has no fabricated details.
Times are UTC Unix seconds, durations are integer seconds. SQLite NULL means
unknown; known zero is distinct. Shared PSN trophy sets must be counted once.
Upsert on (platform, platform_game_id); never merge by name or across platforms.

## Platform research and next integration choices

### Steam / v0.1

Use the public `https://api.steampowered.com` host with a personal Web API key and
SteamID64. `GetOwnedGames` supplies library and playtime; include app info and
played free games. `rtime_last_played` is optional. `GetPlayerAchievements`
supplies per-game counts; schema lookup can distinguish known no-achievements.
Library/privacy errors are not an empty library. Achievements are best effort,
lower concurrency, and must preserve previous counts on transient failures.
Do not expose API request URLs in errors because keys are query parameters.

Artwork: fixed CDN paths are only an initial guess. On failure, resolve the
header_image from public Steam store appdetails (no API key), since newer artwork
may include a hash directory. Cache it and retain the resolved source across syncs.
`npm run covers` backfills missing cached covers without syncing personal metrics.

References:
- https://partner.steamgames.com/doc/webapi_overview
- https://partner.steamgames.com/doc/webapi/IPlayerService
- https://partner.steamgames.com/doc/webapi/ISteamUserStats

### PSN / v0.2

Unofficial endpoints documented by achievements-app/psn-api. NPSSO bootstraps
access/refresh tokens; expiry can require local reauthentication. Start with
played games and trophies. `getUserPlayedGames` includes title ID, artwork,
ISO-8601 duration and last-played time. `getUserTitles` provides trophy sets.
`getPurchasedGames` also exists, but purchase/subscription/played list semantics
must be checked against the account. Use exact platform mappings between title
IDs and npCommunicationId; no name matching. Trophy update time is not play time.

References:
- https://psn-api.achievements.app/api-docs/users
- https://psn-api.achievements.app/api-docs/user-trophies
- https://psn-api.achievements.app/authentication/authenticating-manually

### Epic / v0.3

Legendary implements library pagination, catalog artwork, authentication and some
achievement queries. Start with library/name/artwork only; time and achievements
may remain null. Filter DLC/editor/tool entries. On macOS Legendary defaults to
Mac entries: check Windows and non-installable third-party entries too.
Native Go integration or an explicit Legendary JSON import can be chosen later.

Reference: https://github.com/legendary-gl/legendary

## Definition of done for the first usable release

Real Steam data survives restarts and failed syncs; repeat sync has no duplicates;
missing fields render as unknown; all overview/list/chart interactions work; one
documented local startup command; no credentials exposed. Account-level accuracy
must be verified after the owner configures credentials locally.
