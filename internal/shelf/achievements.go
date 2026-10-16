package shelf

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"time"
)

// A complete snapshot, joined by Steam API name, never by display name or order.
type Achievement struct {
	APIName     string `json:"api_name"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	IconGray    string `json:"icon_gray"`
	Hidden      bool   `json:"hidden"`
	Achieved    bool   `json:"achieved"`
	UnlockedAt  *int64 `json:"unlocked_at"`
}
type GameDetail struct {
	Game             Game          `json:"game"`
	Achievements     []Achievement `json:"achievements"`
	DetailsUpdatedAt *int64        `json:"details_updated_at"`
}

func (s *Steam) AchievementDetails(ctx context.Context, appID string) ([]Achievement, error) {
	var schema struct {
		Game *struct {
			Stats *struct {
				Achievements []struct {
					Name        string `json:"name"`
					DisplayName string `json:"displayName"`
					Description string `json:"description"`
					Icon        string `json:"icon"`
					IconGray    string `json:"icongray"`
					Hidden      int    `json:"hidden"`
				} `json:"achievements"`
			} `json:"availableGameStats"`
		} `json:"game"`
	}
	if err := s.get(ctx, "/ISteamUserStats/GetSchemaForGame/v2/", url.Values{"appid": {appID}, "l": {"schinese"}}, &schema); err != nil {
		return nil, err
	}
	if schema.Game == nil || schema.Game.Stats == nil {
		return nil, errors.New("achievement definitions unavailable")
	}
	items := []Achievement{}
	if len(schema.Game.Stats.Achievements) == 0 {
		return items, nil
	}
	var player struct {
		Stats struct {
			Success      bool `json:"success"`
			Achievements *[]struct {
				Name       string `json:"apiname"`
				Achieved   *int   `json:"achieved"`
				UnlockTime int64  `json:"unlocktime"`
			} `json:"achievements"`
		} `json:"playerstats"`
	}
	if err := s.get(ctx, "/ISteamUserStats/GetPlayerAchievements/v1/", url.Values{"appid": {appID}, "l": {"schinese"}}, &player); err != nil {
		return nil, err
	}
	if !player.Stats.Success || player.Stats.Achievements == nil {
		return nil, errors.New("personal achievement data unavailable")
	}
	states := map[string]Achievement{}
	for _, a := range *player.Stats.Achievements {
		if a.Name == "" || a.Achieved == nil || (*a.Achieved != 0 && *a.Achieved != 1) {
			return nil, errors.New("invalid achievement state")
		}
		if _, exists := states[a.Name]; exists {
			return nil, errors.New("duplicate achievement state")
		}
		item := Achievement{Achieved: *a.Achieved == 1}
		if item.Achieved && a.UnlockTime > 0 {
			t := a.UnlockTime
			item.UnlockedAt = &t
		}
		states[a.Name] = item
	}
	seen := map[string]bool{}
	for _, a := range schema.Game.Stats.Achievements {
		state, ok := states[a.Name]
		if !ok || a.Name == "" || seen[a.Name] {
			return nil, errors.New("incomplete achievement snapshot")
		}
		seen[a.Name] = true
		state.APIName = a.Name
		state.Name = a.DisplayName
		if state.Name == "" {
			state.Name = a.Name
		}
		state.Description = a.Description
		state.Icon = a.Icon
		state.IconGray = a.IconGray
		state.Hidden = a.Hidden == 1
		items = append(items, state)
	}
	if len(items) != len(states) {
		return nil, errors.New("achievement definitions out of date")
	}
	return items, nil
}

func (s *Store) SaveAchievementDetails(ctx context.Context, g Game, items []Achievement) error {
	if items == nil {
		items = []Achievement{}
	}
	body, err := json.Marshal(items)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err = tx.QueryRowContext(ctx, `SELECT id FROM games WHERE platform=? AND platform_game_id=?`, g.Platform, g.PlatformGameID).Scan(&id); err != nil {
		return err
	}
	var earned int64
	for _, a := range items {
		if a.Achieved {
			earned++
		}
	}
	now := time.Now().Unix()
	_, err = tx.ExecContext(ctx, `INSERT INTO achievement_sets(platform,platform_set_id,earned_count,total_count,updated_at) VALUES(?,?,?,?,?)
 ON CONFLICT(platform,platform_set_id) DO UPDATE SET earned_count=excluded.earned_count,total_count=excluded.total_count,updated_at=excluded.updated_at`, g.Platform, g.PlatformGameID, earned, len(items), now)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE games SET achievement_set_id=(SELECT id FROM achievement_sets WHERE platform=? AND platform_set_id=?) WHERE id=?`, g.Platform, g.PlatformGameID, id)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO achievement_details(game_id,items_json,updated_at) VALUES(?,?,?) ON CONFLICT(game_id) DO UPDATE SET items_json=excluded.items_json,updated_at=excluded.updated_at`, id, string(body), now)
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) GameDetail(ctx context.Context, id int64) (*GameDetail, error) {
	games, err := s.Games(ctx)
	if err != nil {
		return nil, err
	}
	var game *Game
	for _, g := range games {
		if g.ID == id {
			copy := g
			game = &copy
			break
		}
	}
	if game == nil {
		return nil, sql.ErrNoRows
	}
	result := &GameDetail{Game: *game, Achievements: []Achievement{}}
	var body string
	var updated int64
	err = s.DB.QueryRowContext(ctx, `SELECT items_json,updated_at FROM achievement_details WHERE game_id=?`, id).Scan(&body, &updated)
	if err == sql.ErrNoRows {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(body), &result.Achievements); err != nil {
		return nil, err
	}
	result.DetailsUpdatedAt = &updated
	return result, nil
}
func (s *Store) HasAchievementDetails(ctx context.Context, g Game) bool {
	var found int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM achievement_details d JOIN games g ON g.id=d.game_id WHERE g.platform=? AND g.platform_game_id=?`, g.Platform, g.PlatformGameID).Scan(&found)
	return err == nil
}
