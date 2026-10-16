package shelf

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }

type Game struct {
	ID                   int64   `json:"id"`
	Platform             string  `json:"platform"`
	PlatformGameID       string  `json:"platform_game_id"`
	Name                 string  `json:"name"`
	EditionPlatform      *string `json:"edition_platform"`
	CoverURL             *string `json:"cover_url"`
	CoverPath            *string `json:"cover_path"`
	PlaytimeSeconds      *int64  `json:"playtime_seconds"`
	LastPlayedAt         *int64  `json:"last_played_at"`
	AchievementSetID     *int64  `json:"achievement_set_id"`
	EarnedCount          *int64  `json:"earned_count"`
	TotalCount           *int64  `json:"total_count"`
	SourceScope          string  `json:"source_scope"`
	UpdatedAt            int64   `json:"updated_at"`
	AchievementUpdatedAt *int64  `json:"achievement_updated_at"`
}

type SyncRun struct {
	ID           int64   `json:"id"`
	Platform     string  `json:"platform"`
	StartedAt    int64   `json:"started_at"`
	FinishedAt   *int64  `json:"finished_at"`
	Status       string  `json:"status"`
	ItemsSynced  int     `json:"items_synced"`
	ErrorSummary *string `json:"error_summary"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if _, err = db.Exec(`PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL;`); err != nil {
		db.Close()
		return nil, err
	}
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version > 3 {
		db.Close()
		return nil, fmt.Errorf("database schema %d is newer than this application", version)
	}
	if version == 0 {
		tx, e := db.Begin()
		if e != nil {
			db.Close()
			return nil, e
		}
		_, err = tx.Exec(`
CREATE TABLE achievement_sets (
 id INTEGER PRIMARY KEY, platform TEXT NOT NULL, platform_set_id TEXT NOT NULL,
 earned_count INTEGER CHECK(earned_count>=0), total_count INTEGER CHECK(total_count>=0),
 platform_progress_percent REAL, updated_at INTEGER NOT NULL,
 UNIQUE(platform, platform_set_id), CHECK(earned_count IS NULL OR total_count IS NULL OR earned_count<=total_count)
);
CREATE TABLE games (
 id INTEGER PRIMARY KEY, platform TEXT NOT NULL, platform_game_id TEXT NOT NULL,
 name TEXT NOT NULL, edition_platform TEXT, cover_url TEXT, cover_path TEXT,
 playtime_seconds INTEGER CHECK(playtime_seconds>=0), last_played_at INTEGER,
 achievement_set_id INTEGER REFERENCES achievement_sets(id), source_scope TEXT NOT NULL,
 last_seen_at INTEGER NOT NULL, updated_at INTEGER NOT NULL,
 UNIQUE(platform, platform_game_id)
);
CREATE INDEX games_recent ON games(last_played_at DESC);
CREATE TABLE sync_runs (
 id INTEGER PRIMARY KEY, platform TEXT NOT NULL, started_at INTEGER NOT NULL,
 finished_at INTEGER, status TEXT NOT NULL, items_synced INTEGER NOT NULL DEFAULT 0,
 error_summary TEXT
);
CREATE INDEX sync_runs_platform ON sync_runs(platform,id DESC);
PRAGMA user_version=1;`)
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		if err != nil {
			db.Close()
			return nil, err
		}
	}
	if version < 2 {
		_, err = db.Exec(`BEGIN; CREATE TABLE achievement_details (
 game_id INTEGER PRIMARY KEY REFERENCES games(id), items_json TEXT NOT NULL, updated_at INTEGER NOT NULL
 ); PRAGMA user_version=2; COMMIT;`)
		if err != nil {
			_, _ = db.Exec("ROLLBACK")
			db.Close()
			return nil, err
		}
	}
	if version < 3 {
		_, err = db.Exec(`BEGIN; ALTER TABLE achievement_details ADD COLUMN groups_json TEXT;
 ALTER TABLE achievement_details ADD COLUMN groups_updated_at INTEGER;
 PRAGMA user_version=3; COMMIT;`)
		if err != nil {
			_, _ = db.Exec("ROLLBACK")
			db.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) SaveLibrary(ctx context.Context, games []Game) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	for _, g := range games {
		_, err = tx.ExecContext(ctx, `INSERT INTO games(platform,platform_game_id,name,edition_platform,cover_url,playtime_seconds,last_played_at,source_scope,last_seen_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(platform,platform_game_id) DO UPDATE SET
name=excluded.name,edition_platform=COALESCE(excluded.edition_platform,games.edition_platform),cover_url=COALESCE(games.cover_url,excluded.cover_url),
playtime_seconds=COALESCE(excluded.playtime_seconds,games.playtime_seconds),
last_played_at=COALESCE(excluded.last_played_at,games.last_played_at),
source_scope=excluded.source_scope,last_seen_at=excluded.last_seen_at,updated_at=excluded.updated_at`,
			g.Platform, g.PlatformGameID, g.Name, g.EditionPlatform, g.CoverURL, g.PlaytimeSeconds, g.LastPlayedAt, g.SourceScope, now, now)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SaveAchievements(ctx context.Context, platform, gameID, setID string, earned, total int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO achievement_sets(platform,platform_set_id,earned_count,total_count,updated_at) VALUES(?,?,?,?,?)
ON CONFLICT(platform,platform_set_id) DO UPDATE SET earned_count=excluded.earned_count,total_count=excluded.total_count,updated_at=excluded.updated_at`, platform, setID, earned, total, time.Now().Unix())
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE games SET achievement_set_id=(SELECT id FROM achievement_sets WHERE platform=? AND platform_set_id=?) WHERE platform=? AND platform_game_id=?`, platform, setID, platform, gameID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveCover(ctx context.Context, platform, gameID, path, sourceURL string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE games SET cover_path=?,cover_url=? WHERE platform=? AND platform_game_id=?`, path, sourceURL, platform, gameID)
	return err
}

func (s *Store) Games(ctx context.Context) ([]Game, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT g.id,g.platform,g.platform_game_id,g.name,g.edition_platform,g.cover_url,g.cover_path,
g.playtime_seconds,g.last_played_at,g.achievement_set_id,a.earned_count,a.total_count,g.source_scope,g.updated_at,a.updated_at
FROM games g LEFT JOIN achievement_sets a ON g.achievement_set_id=a.id ORDER BY g.last_played_at DESC,g.name COLLATE NOCASE,g.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	games := []Game{}
	for rows.Next() {
		var g Game
		if err := rows.Scan(&g.ID, &g.Platform, &g.PlatformGameID, &g.Name, &g.EditionPlatform, &g.CoverURL, &g.CoverPath, &g.PlaytimeSeconds, &g.LastPlayedAt, &g.AchievementSetID, &g.EarnedCount, &g.TotalCount, &g.SourceScope, &g.UpdatedAt, &g.AchievementUpdatedAt); err != nil {
			return nil, err
		}
		games = append(games, g)
	}
	return games, rows.Err()
}

func (s *Store) StartRun(ctx context.Context, platform string) (int64, error) {
	r, err := s.DB.ExecContext(ctx, `INSERT INTO sync_runs(platform,started_at,status) VALUES(?,?,'running')`, platform, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return r.LastInsertId()
}

func (s *Store) FinishRun(ctx context.Context, id int64, status string, count int, summary *string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE sync_runs SET finished_at=?,status=?,items_synced=?,error_summary=? WHERE id=?`, time.Now().Unix(), status, count, summary, id)
	return err
}

func (s *Store) LatestRun(ctx context.Context, platform string) (*SyncRun, error) {
	var r SyncRun
	err := s.DB.QueryRowContext(ctx, `SELECT id,platform,started_at,finished_at,status,items_synced,error_summary FROM sync_runs WHERE platform=? ORDER BY id DESC LIMIT 1`, platform).Scan(&r.ID, &r.Platform, &r.StartedAt, &r.FinishedAt, &r.Status, &r.ItemsSynced, &r.ErrorSummary)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &r, err
}

// A partial run has still refreshed the library; failures should not erase that timestamp.
func (s *Store) LastLibrarySync(ctx context.Context, platform string) (*int64, error) {
	var result *int64
	err := s.DB.QueryRowContext(ctx, `SELECT MAX(finished_at) FROM sync_runs WHERE platform=? AND status IN ('success','partial')`, platform).Scan(&result)
	return result, err
}
