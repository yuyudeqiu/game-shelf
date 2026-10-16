export type Platform = "steam" | "psn" | "epic";
export interface Game {
  id: number;
  platform: Platform;
  platform_game_id: string;
  name: string;
  edition_platform: string | null;
  cover_url: string | null;
  cover_path: string | null;
  playtime_seconds: number | null;
  last_played_at: number | null;
  achievement_set_id: number | null;
  earned_count: number | null;
  total_count: number | null;
  source_scope: string;
  updated_at: number;
  achievement_updated_at: number | null;
}
export interface SyncRun {
  id: number;
  platform: Platform;
  started_at: number;
  finished_at: number | null;
  status: "running" | "success" | "partial" | "failed";
  items_synced: number;
  error_summary: string | null;
}
export interface PlatformStatus {
  platform: Platform;
  configured: boolean;
  supported: boolean;
  scope: string;
  count: number;
  earned: number;
  run: SyncRun | null;
  last_library_sync: number | null;
}
export interface Dashboard {
  metrics: {
    games: number;
    playtime_seconds: number;
    playtime_known: number;
    earned: number;
    achievements_known: number;
  };
  platforms: PlatformStatus[];
  recent: Game[];
  top_played: Game[];
}
export interface Library {
  games: Game[];
  total: number;
  page: number;
  pages: number;
  page_size: number;
}
export interface Achievement {
  api_name: string;
  name: string;
  description: string;
  icon: string;
  icon_gray: string;
  hidden: boolean;
  achieved: boolean;
  unlocked_at: number | null;
  trophy_type?: string;
  group_id?: string;
}
export interface AchievementGroup {
  id: string;
  name: string;
  icon: string;
}
export interface GameDetail {
  game: Game;
  achievements: Achievement[];
  details_updated_at: number | null;
  achievement_groups: AchievementGroup[];
  groups_updated_at: number | null;
}
