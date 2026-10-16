package shelf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type psnConfig struct {
	AccountID string `json:"account_id"`
	Access    string `json:"access_token"`
	Refresh   string `json:"refresh_token"`
	ExpiresAt int64  `json:"expires_at"`
}
type PSNStatus struct {
	AccountID  string `json:"account_id"`
	Configured bool   `json:"configured"`
	HasToken   bool   `json:"has_token"`
	Busy       bool   `json:"busy"`
}
type PSNSettings struct {
	mu     sync.Mutex
	config psnConfig
	busy   bool
	dir    string
	store  *Store
	client *PSN
	Wake   chan struct{}
}

func NewPSNSettings(dir string, store *Store) (*PSNSettings, error) {
	s := &PSNSettings{dir: dir, store: store, client: NewPSN(""), Wake: make(chan struct{}, 1)}
	f, err := os.Open(filepath.Join(dir, "psn-settings.json"))
	if err == nil {
		defer f.Close()
		if json.NewDecoder(io.LimitReader(f, 16384)).Decode(&s.config) != nil {
			return nil, errors.New("本地 PSN 配置无法读取")
		}
	} else if !os.IsNotExist(err) {
		return nil, errors.New("无法读取本地 PSN 配置")
	}
	return s, nil
}
func (s *PSNSettings) Status() PSNStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return PSNStatus{s.config.AccountID, s.config.AccountID != "" && s.config.Refresh != "", s.config.Refresh != "", s.busy}
}
func (s *PSNSettings) take() (psnConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return psnConfig{}, errors.New("PSN 正在连接或同步，请稍候")
	}
	s.busy = true
	return s.config, nil
}
func (s *PSNSettings) end() { s.mu.Lock(); s.busy = false; s.mu.Unlock() }
func (s *PSNSettings) RequestSync() {
	select {
	case s.Wake <- struct{}{}:
	default:
	}
}
func (s *PSNSettings) persist(cfg psnConfig) error {
	body, err := json.Marshal(cfg)
	if err != nil {
		return errors.New("无法保存 PSN 配置")
	}
	if err = os.MkdirAll(s.dir, 0700); err != nil {
		return errors.New("无法创建配置目录")
	}
	f, err := os.CreateTemp(s.dir, "psn-settings-*")
	if err != nil {
		return errors.New("无法保存 PSN 配置")
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(body); err != nil {
		f.Close()
		return errors.New("无法保存 PSN 配置")
	}
	if err = f.Close(); err != nil {
		return errors.New("无法保存 PSN 配置")
	}
	if err = os.Rename(f.Name(), filepath.Join(s.dir, "psn-settings.json")); err != nil {
		return errors.New("无法保存 PSN 配置")
	}
	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()
	return nil
}
func (s *PSNSettings) bind(ctx context.Context, id string) error {
	bound, err := os.ReadFile(filepath.Join(s.dir, "psn-account"))
	if err != nil && !os.IsNotExist(err) {
		return errors.New("无法读取 PSN 账号绑定")
	}
	if strings.TrimSpace(string(bound)) != id {
		games, err := s.store.Games(ctx)
		if err != nil {
			return errors.New("无法检查 PSN 账号")
		}
		for _, g := range games {
			if g.Platform == "psn" {
				return errors.New("书架已有其他 PSN 账号的数据，请使用新的 DATA_DIR")
			}
		}
	}
	if err = os.MkdirAll(s.dir, 0700); err != nil {
		return errors.New("无法保存 PSN 账号绑定")
	}
	if err = os.WriteFile(filepath.Join(s.dir, "psn-account"), []byte(id+"\n"), 0600); err != nil {
		return errors.New("无法保存 PSN 账号绑定")
	}
	return nil
}
func (s *PSNSettings) Save(ctx context.Context, npsso string) error {
	cfg, err := s.take()
	if err != nil {
		return err
	}
	defer s.end()
	npsso = strings.TrimSpace(npsso)
	if npsso == "" && cfg.Refresh != "" {
		return nil
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{64}$`).MatchString(npsso) {
		return errors.New("请填写 64 位 NPSSO，只粘贴值本身")
	}
	client := *s.client
	tokens, err := client.authorize(ctx, npsso)
	if err != nil {
		return err
	}
	client.Token = tokens.Access
	id, err := client.identity(ctx)
	if err != nil {
		return err
	}
	if err = s.bind(ctx, id); err != nil {
		return err
	}
	// NPSSO is only used for this exchange; persist renewable tokens, never NPSSO.
	return s.persist(psnConfig{id, tokens.Access, tokens.Refresh, time.Now().Unix() + tokens.ExpiresIn})
}
func (s *PSNSettings) Sync(ctx context.Context) error { return s.sync(ctx, false) }

// RefreshDetails bypasses the 24h detail cache, e.g. after correcting a locale.
// It still validates complete responses and retains prior snapshots on failure.
func (s *PSNSettings) RefreshDetails(ctx context.Context) error { return s.sync(ctx, true) }
func (s *PSNSettings) sync(ctx context.Context, refreshDetails bool) (err error) {
	cfg, err := s.take()
	if err != nil {
		return err
	}
	defer s.end()
	if cfg.Refresh == "" || cfg.AccountID == "" {
		return errors.New("请先在平台设置中连接 PSN")
	}
	runID, err := s.store.StartRun(ctx, "psn")
	if err != nil {
		return errors.New("无法记录 PSN 同步")
	}
	status, count := "failed", 0
	var summary *string
	defer func() {
		if err != nil {
			message := err.Error()
			summary = &message
		}
		finish, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := s.store.FinishRun(finish, runID, status, count, summary); e != nil && err == nil {
			err = errors.New("无法保存 PSN 同步状态")
		}
	}()
	client := *s.client
	if cfg.Access == "" || cfg.ExpiresAt <= time.Now().Unix()+120 {
		tokens, e := client.token(ctx, url.Values{"refresh_token": {cfg.Refresh}, "grant_type": {"refresh_token"}, "token_format": {"jwt"}, "scope": {"psn:mobile.v2.core psn:clientapp"}})
		if e != nil {
			return e
		}
		cfg.Access = tokens.Access
		cfg.Refresh = tokens.Refresh
		cfg.ExpiresAt = time.Now().Unix() + tokens.ExpiresIn
	}
	client.Token = cfg.Access
	id, err := client.identity(ctx)
	if err != nil {
		return err
	}
	if id != cfg.AccountID {
		return errors.New("PSN 返回的账号与本地绑定不一致，已停止同步")
	}
	if err = s.bind(ctx, id); err != nil {
		return err
	}
	if err = s.persist(cfg); err != nil {
		return err
	}
	count, summary, err = syncPSN(ctx, &client, s.store, s.dir, refreshDetails)
	if err != nil {
		if count > 0 {
			status = "partial"
		}
		return err
	}
	status = "success"
	if summary != nil {
		status = "partial"
	}
	return nil
}

func syncPSN(ctx context.Context, p *PSN, store *Store, dir string, refreshDetails bool) (int, *string, error) {
	games, err := p.Library(ctx)
	if err != nil {
		return 0, nil, err
	}
	previous, err := store.Games(ctx)
	if err != nil {
		return 0, nil, errors.New("无法读取本地游戏")
	}
	old := map[string]Game{}
	for _, g := range previous {
		if g.Platform == "psn" {
			old[g.PlatformGameID] = g
		}
	}
	if err = store.SaveLibrary(ctx, games); err != nil {
		return 0, nil, errors.New("无法保存 PSN 游戏列表")
	}
	mappings := map[string]psnTrophyTitle{}
	mappingFailed := 0
	for start := 0; start < len(games); start += 5 {
		end := min(start+5, len(games))
		ids := []string{}
		for _, g := range games[start:end] {
			ids = append(ids, g.PlatformGameID)
		}
		batch, e := p.TrophyMappings(ctx, ids)
		if e != nil {
			// One unsupported title may make a whole batch return 404. Salvage others.
			for _, id := range ids {
				single, e := p.TrophyMappings(ctx, []string{id})
				if e != nil {
					mappingFailed++
					continue
				}
				for key, value := range single {
					mappings[key] = value
				}
			}
		} else {
			for key, value := range batch {
				mappings[key] = value
			}
		}
		if ctx.Err() != nil {
			return len(games), nil, errors.New("PSN 同步中断，已保存游戏列表")
		}
	}
	coverSync := &Syncer{Store: store, Steam: NewSteam("", ""), DataDir: dir}
	coverSync.Steam.HTTP = p.HTTP
	failures, coverFailures, unmapped := 0, 0, 0
	// Shared sets are requested once per run and linked by stable set IDs, never names.
	fetched := map[string][]Achievement{}
	unavailable := map[string]bool{}
	fetchedGroups := map[string][]AchievementGroup{}
	unavailableGroups := map[string]bool{}
	refreshGroups := func(g Game, title psnTrophyTitle) {
		setID := title.setID()
		if unavailableGroups[setID] {
			failures++
			return
		}
		groups, done := fetchedGroups[setID]
		if !done {
			var e error
			groups, e = p.TrophyGroups(ctx, title)
			if e != nil {
				unavailableGroups[setID] = true
				failures++
				return
			}
			fetchedGroups[setID] = groups
		}
		if store.SaveAchievementGroups(ctx, g, groups) != nil {
			failures++
		}
	}
	for _, g := range games {
		if ctx.Err() != nil {
			return len(games), nil, errors.New("PSN 同步中断，已保存游戏列表")
		}
		prev := old[g.PlatformGameID]
		if g.CoverURL != nil && (prev.CoverPath == nil || !coverSync.coverExists(*prev.CoverPath)) {
			if coverSync.cacheCover(ctx, g) != nil {
				coverFailures++
			}
		}
		title, exists := mappings[g.PlatformGameID]
		if !exists {
			unmapped++
			continue
		}
		setID := title.setID()
		earned, earnedOK := title.Earned.sum()
		total, totalOK := title.Defined.sum()
		// Evaluate staleness against the previous snapshot before updating summary counts.
		snapshot, snapshotErr := store.GameDetail(ctx, prev.ID)
		stale := snapshotErr != nil || snapshot.DetailsUpdatedAt == nil || time.Since(time.Unix(*snapshot.DetailsUpdatedAt, 0)) > 24*time.Hour
		changed := !equalInt(prev.EarnedCount, &earned) || !equalInt(prev.TotalCount, &total) || !equalInt(prev.PlaytimeSeconds, g.PlaytimeSeconds)
		if earnedOK && totalOK && earned <= total {
			if store.SaveAchievements(ctx, "psn", g.PlatformGameID, setID, earned, total) != nil {
				failures++
				continue
			}
		}
		if !refreshDetails && !stale && !changed {
			if snapshot.GroupsUpdatedAt == nil || time.Since(time.Unix(*snapshot.GroupsUpdatedAt, 0)) > 24*time.Hour {
				refreshGroups(g, title)
			}
			continue
		}
		if unavailable[setID] {
			failures++
			continue
		}
		items, done := fetched[setID]
		if !done {
			items, err = p.TrophyDetails(ctx, title)
			if err != nil || totalOK && int64(len(items)) != total {
				unavailable[setID] = true
				failures++
				continue
			}
			fetched[setID] = items
		}
		if store.SaveAchievementDetailsForSet(ctx, g, setID, items) != nil {
			failures++
			continue
		}
		refreshGroups(g, title)
	}
	if mappingFailed+failures+coverFailures+unmapped > 0 {
		message := fmtPSNSummary(mappingFailed, unmapped, failures, coverFailures)
		return len(games), &message, nil
	}
	return len(games), nil, nil
}
func fmtPSNSummary(mapping, unmapped, details, covers int) string {
	return fmt.Sprintf("已保存已玩游戏；%d 个奖杯映射请求失败，%d 个游戏无唯一奖杯映射，%d 个奖杯详情/分组及 %d 张封面暂不可用。已有数据保留。", mapping, unmapped, details, covers)
}
