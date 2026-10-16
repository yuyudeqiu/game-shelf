package shelf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Steam struct {
	Key      string
	ID       string
	BaseURL  string
	StoreURL string
	HTTP     *http.Client
}

func NewSteam(key, id string) *Steam {
	return &Steam{Key: key, ID: id, BaseURL: "https://api.steampowered.com", StoreURL: "https://store.steampowered.com", HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// Never forward transport errors: net/http errors can include URLs containing the API key.
func (s *Steam) get(ctx context.Context, endpoint string, params url.Values, out any) error {
	params.Set("key", s.Key)
	params.Set("steamid", s.ID)
	params.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return errors.New("Steam request could not be created")
	}
	res, err := s.HTTP.Do(req)
	if err != nil {
		return errors.New("Steam network request failed or timed out")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Steam returned HTTP %d", res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 20<<20)).Decode(out); err != nil {
		return errors.New("Steam returned an invalid response")
	}
	return nil
}

func (s *Steam) Library(ctx context.Context) ([]Game, error) {
	var payload struct {
		Response struct {
			GameCount *int `json:"game_count"`
			Games     []struct {
				AppID      int64  `json:"appid"`
				Name       string `json:"name"`
				Playtime   *int64 `json:"playtime_forever"`
				LastPlayed int64  `json:"rtime_last_played"`
			} `json:"games"`
		} `json:"response"`
	}
	err := s.get(ctx, "/IPlayerService/GetOwnedGames/v1/", url.Values{"include_appinfo": {"true"}, "include_played_free_games": {"true"}}, &payload)
	if err != nil {
		return nil, err
	}
	if payload.Response.GameCount == nil {
		return nil, errors.New("Steam library unavailable; check SteamID and game-details privacy")
	}
	if *payload.Response.GameCount != len(payload.Response.Games) {
		return nil, errors.New("Steam returned an incomplete library; existing data was preserved")
	}
	games := make([]Game, 0, len(payload.Response.Games))
	for _, item := range payload.Response.Games {
		if item.AppID <= 0 || strings.TrimSpace(item.Name) == "" {
			return nil, errors.New("Steam returned an invalid library entry")
		}
		id := strconv.FormatInt(item.AppID, 10)
		g := Game{Platform: "steam", PlatformGameID: id, Name: item.Name, SourceScope: "owned"}
		cover := "https://shared.fastly.steamstatic.com/store_item_assets/steam/apps/" + id + "/header.jpg"
		g.CoverURL = &cover
		if item.Playtime != nil && *item.Playtime >= 0 {
			seconds := *item.Playtime * 60
			g.PlaytimeSeconds = &seconds
		}
		if item.LastPlayed > 0 {
			t := item.LastPlayed
			g.LastPlayedAt = &t
		}
		games = append(games, g)
	}
	return games, nil
}

func (s *Steam) Achievements(ctx context.Context, appID string) (int64, int64, error) {
	var payload struct {
		Stats struct {
			Success      bool `json:"success"`
			Achievements *[]struct {
				Achieved int `json:"achieved"`
			} `json:"achievements"`
		} `json:"playerstats"`
	}
	err := s.get(ctx, "/ISteamUserStats/GetPlayerAchievements/v1/", url.Values{"appid": {appID}}, &payload)
	if err == nil && payload.Stats.Success && payload.Stats.Achievements != nil {
		var earned int64
		for _, a := range *payload.Stats.Achievements {
			if a.Achieved == 1 {
				earned++
			}
		}
		return earned, int64(len(*payload.Stats.Achievements)), nil
	}
	// Some games have no stats schema. Only a successful schema response can establish zero.
	var schema struct {
		Game *struct {
			Stats *struct {
				Achievements []json.RawMessage `json:"achievements"`
			} `json:"availableGameStats"`
		} `json:"game"`
	}
	if schemaErr := s.get(ctx, "/ISteamUserStats/GetSchemaForGame/v2/", url.Values{"appid": {appID}}, &schema); schemaErr == nil && schema.Game != nil {
		if schema.Game.Stats != nil && len(schema.Game.Stats.Achievements) == 0 {
			return 0, 0, nil
		}
	}
	return 0, 0, errors.New("achievement data unavailable")
}

type Syncer struct {
	Store      *Store
	Steam      *Steam
	DataDir    string
	SkipCovers bool
	// Minimum spacing for enrichment calls; tests can use a tiny duration.
	Spacing time.Duration
	mu      sync.Mutex
}

func (s *Syncer) Sync(ctx context.Context) (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runID, err := s.Store.StartRun(ctx, "steam")
	if err != nil {
		return err
	}
	status, count := "failed", 0
	var summary *string
	defer func() {
		if err != nil {
			message := err.Error()
			summary = &message
		}
		finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if finishErr := s.Store.FinishRun(finishCtx, runID, status, count, summary); finishErr != nil && err == nil {
			err = errors.New("could not save sync result")
		}
	}()
	previous, err := s.Store.Games(ctx)
	if err != nil {
		return errors.New("could not read existing games")
	}
	old := make(map[string]Game)
	for _, g := range previous {
		if g.Platform == "steam" {
			old[g.PlatformGameID] = g
		}
	}
	games, err := s.Steam.Library(ctx)
	if err != nil {
		return err
	}
	if err = s.Store.SaveLibrary(ctx, games); err != nil {
		return errors.New("could not save Steam library")
	}
	count = len(games)
	// Recent activity is enriched first, so large libraries become useful quickly.
	sort.SliceStable(games, func(i, j int) bool { return timestamp(games[i].LastPlayedAt) > timestamp(games[j].LastPlayedAt) })
	spacing := s.Spacing
	if spacing <= 0 {
		spacing = 250 * time.Millisecond
	}
	ticker := time.NewTicker(spacing)
	defer ticker.Stop()
	failures, coverFailures := 0, 0
	for _, g := range games {
		if ctx.Err() != nil {
			status = "partial"
			return errors.New("sync interrupted after library update")
		}
		prev, exists := old[g.PlatformGameID]
		changed := !exists || !equalInt(prev.PlaytimeSeconds, g.PlaytimeSeconds)
		stale := prev.AchievementUpdatedAt == nil || time.Since(time.Unix(*prev.AchievementUpdatedAt, 0)) > 24*time.Hour
		if changed || stale || !s.Store.HasAchievementDetails(ctx, g) {
			select {
			case <-ctx.Done():
				status = "partial"
				return errors.New("sync interrupted after library update")
			case <-ticker.C:
			}
			items, detailErr := s.Steam.AchievementDetails(ctx, g.PlatformGameID)
			if detailErr == nil {
				if s.Store.SaveAchievementDetails(ctx, g, items) != nil {
					failures++
				}
			} else {
				// Count-only data can still be useful; never replace a previous detail snapshot.
				failures++
				earned, total, e := s.Steam.Achievements(ctx, g.PlatformGameID)
				if e == nil {
					_ = s.Store.SaveAchievements(ctx, "steam", g.PlatformGameID, g.PlatformGameID, earned, total)
				}
			}
		}
		if !s.SkipCovers && (prev.CoverPath == nil || !s.coverExists(*prev.CoverPath)) {
			if e := s.cacheCover(ctx, g); e != nil {
				coverFailures++
			}
		}
	}
	status = "success"
	if failures > 0 || coverFailures > 0 {
		status = "partial"
		message := fmt.Sprintf("Library saved; %d achievement detail snapshots and %d cover downloads unavailable. Previous available data retained.", failures, coverFailures)
		summary = &message
	}
	return nil
}

func equalInt(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func timestamp(t *int64) int64 {
	if t == nil {
		return 0
	}
	return *t
}
func (s *Syncer) coverExists(name string) bool {
	_, err := os.Stat(filepath.Join(s.DataDir, "covers", filepath.Base(name)))
	return err == nil
}

func (s *Syncer) cacheCover(ctx context.Context, g Game) error {
	if g.CoverURL == nil {
		return nil
	}
	sourceURL := *g.CoverURL
	body, err := s.downloadCover(ctx, sourceURL)
	if err != nil && g.Platform == "steam" && ctx.Err() == nil {
		sourceURL, err = s.Steam.storeCover(ctx, g.PlatformGameID)
		if err == nil {
			body, err = s.downloadCover(ctx, sourceURL)
		}
	}
	if err != nil {
		return err
	}
	dir := filepath.Join(s.DataDir, "covers")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	name := g.Platform + "-" + g.PlatformGameID + ".jpg"
	f, err := os.CreateTemp(dir, "cover-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(body); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	return s.Store.SaveCover(ctx, g.Platform, g.PlatformGameID, name, sourceURL)
}

func (s *Syncer) downloadCover(ctx context.Context, sourceURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, errors.New("invalid cover URL")
	}
	res, err := s.Steam.HTTP.Do(req)
	if err != nil {
		return nil, errors.New("cover unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "image/") {
		return nil, errors.New("invalid cover response")
	}
	const maxBytes = 3 << 20
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBytes+1))
	if err != nil || len(body) > maxBytes || len(body) == 0 {
		return nil, errors.New("invalid cover size")
	}
	return body, nil
}

// Store artwork can contain a content-hash directory absent from guessed CDN URLs.
func (s *Steam) storeCover(ctx context.Context, appID string) (string, error) {
	query := url.Values{"appids": {appID}, "filters": {"basic"}, "l": {"english"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.StoreURL+"/api/appdetails?"+query.Encode(), nil)
	if err != nil {
		return "", errors.New("invalid store request")
	}
	res, err := s.HTTP.Do(req)
	if err != nil {
		return "", errors.New("store artwork unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", errors.New("store artwork unavailable")
	}
	var payload map[string]struct {
		Success bool `json:"success"`
		Data    struct {
			Header string `json:"header_image"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&payload) != nil {
		return "", errors.New("invalid store response")
	}
	item := payload[appID]
	u, err := url.Parse(item.Data.Header)
	if !item.Success || err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".steamstatic.com") || u.User != nil {
		return "", errors.New("store artwork missing")
	}
	return item.Data.Header, nil
}

// Repair only missing artwork; no account credentials or library/stat resync required.
func (s *Syncer) RepairCovers(ctx context.Context) (repaired, failed int, err error) {
	games, err := s.Store.Games(ctx)
	if err != nil {
		return 0, 0, err
	}
	for _, g := range games {
		if ctx.Err() != nil {
			return repaired, failed, ctx.Err()
		}
		if g.Platform != "steam" || g.CoverPath != nil && s.coverExists(*g.CoverPath) {
			continue
		}
		if e := s.cacheCover(ctx, g); e != nil {
			failed++
		} else {
			repaired++
		}
	}
	return repaired, failed, nil
}
