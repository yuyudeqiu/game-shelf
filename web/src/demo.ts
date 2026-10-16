import type { Dashboard, Game, Library, Platform } from "./types";

const now = Math.floor(Date.now() / 1000);
const entries: [
  string,
  string,
  Platform,
  number | null,
  number | null,
  number | null,
  number | null,
][] = [
  ["1245620", "ELDEN RING", "steam", 146.8, 36, 42, 0.08],
  ["1091500", "Cyberpunk 2077", "steam", 89.2, 38, 57, 1],
  ["1086940", "Baldur’s Gate 3", "steam", 112.4, 29, 54, 2],
  ["1145360", "Hades", "steam", 64.5, 32, 49, 3],
  ["367520", "Hollow Knight", "steam", 42.3, 41, 63, 5],
  ["CUSA34388", "God of War Ragnarök", "psn", 58.2, 28, 36, 6],
  ["413150", "Stardew Valley", "steam", 126.1, 24, 49, 8],
  ["646570", "Slay the Spire", "steam", 78.4, 30, 46, 12],
  ["1174180", "Red Dead Redemption 2", "steam", 94.7, 19, 51, 15],
  ["CUSA07410", "The Last of Us Part II", "psn", 36.5, 17, 26, 25],
  ["alan-wake-2", "Alan Wake 2", "epic", null, null, null, null],
  ["death-stranding", "DEATH STRANDING", "epic", null, null, null, null],
  ["292030", "The Witcher 3: Wild Hunt", "steam", 0, 0, 78, null],
  ["105600", "Terraria", "steam", 0, null, null, null],
];
const art: Record<string, string> = {
  CUSA34388: "2322010",
  CUSA07410: "2531310",
  "alan-wake-2": "",
  "death-stranding": "1850570",
};
export const demoGames: Game[] = entries.map(
  ([id, name, platform, hours, earned, total, days], index) => ({
    id: index + 1,
    platform,
    platform_game_id: id,
    name,
    edition_platform: platform === "psn" ? "PS5" : null,
    cover_url:
      (art[id] ?? id)
        ? `https://shared.fastly.steamstatic.com/store_item_assets/steam/apps/${art[id] ?? id}/header.jpg`
        : null,
    cover_path: null,
    playtime_seconds: hours === null ? null : Math.round(hours * 3600),
    last_played_at: days === null ? null : now - Math.round(days * 86400),
    achievement_set_id: earned === null ? null : index + 1,
    earned_count: earned,
    total_count: total,
    source_scope: platform === "psn" ? "played" : "owned",
    updated_at: now,
    achievement_updated_at: earned === null ? null : now,
  }),
);
export function demoDashboard(): Dashboard {
  return {
    metrics: {
      games: demoGames.length,
      playtime_seconds: demoGames.reduce(
        (sum, g) => sum + (g.playtime_seconds ?? 0),
        0,
      ),
      playtime_known: demoGames.filter((g) => g.playtime_seconds !== null)
        .length,
      earned: demoGames.reduce((sum, g) => sum + (g.earned_count ?? 0), 0),
      achievements_known: demoGames.filter((g) => g.earned_count !== null)
        .length,
    },
    platforms: (["steam", "psn", "epic"] as const).map((platform) => ({
      platform,
      configured: true,
      supported: true,
      scope: platform === "psn" ? "played" : "owned",
      count: demoGames.filter((g) => g.platform === platform).length,
      earned: demoGames
        .filter((g) => g.platform === platform)
        .reduce((sum, g) => sum + (g.earned_count ?? 0), 0),
      run: null,
      last_library_sync: now - 300,
    })),
    recent: demoGames.filter((g) => g.last_played_at !== null).slice(0, 6),
    top_played: [...demoGames]
      .filter((g) => (g.playtime_seconds ?? 0) > 0)
      .sort((a, b) => b.playtime_seconds! - a.playtime_seconds!)
      .slice(0, 5),
  };
}
export function demoLibrary(
  q: string,
  platform: string,
  sort: string,
  page: number,
): Library {
  const games = demoGames.filter(
    (g) =>
      g.name.toLowerCase().includes(q.trim().toLowerCase()) &&
      (platform === "all" || g.platform === platform),
  );
  if (sort === "name") games.sort((a, b) => a.name.localeCompare(b.name));
  if (sort === "playtime")
    games.sort(
      (a, b) => (b.playtime_seconds ?? -1) - (a.playtime_seconds ?? -1),
    );
  if (sort === "achievements") games.sort((a, b) => ratio(b) - ratio(a));
  const pages = Math.max(1, Math.ceil(games.length / 20));
  page = Math.min(page, pages);
  return {
    games: games.slice((page - 1) * 20, page * 20),
    total: games.length,
    page,
    pages,
    page_size: 20,
  };
}
function ratio(g: Game) {
  return g.earned_count !== null && g.total_count
    ? g.earned_count / g.total_count
    : -1;
}
