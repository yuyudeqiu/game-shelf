package shelf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type epicConfig struct {
	AccountID string `json:"account_id"`
	Access    string `json:"access_token"`
	Refresh   string `json:"refresh_token"`
	ExpiresAt int64  `json:"expires_at"`
}
type EpicStatus struct {
	AccountID  string `json:"account_id"`
	Configured bool   `json:"configured"`
	HasToken   bool   `json:"has_token"`
	Busy       bool   `json:"busy"`
	LoginURL   string `json:"login_url"`
}
type EpicSettings struct {
	mu      sync.Mutex
	config  epicConfig
	busy    bool
	dir     string
	store   *Store
	client  *Epic
	Wake    chan struct{}
	Spacing time.Duration
}

func NewEpicSettings(dir string, store *Store) (*EpicSettings, error) {
	s := &EpicSettings{dir: dir, store: store, client: NewEpic(), Wake: make(chan struct{}, 1), Spacing: 150 * time.Millisecond}
	body, err := os.ReadFile(filepath.Join(dir, "epic-settings.json"))
	if err == nil {
		if len(body) > 16384 || json.Unmarshal(body, &s.config) != nil {
			return nil, errors.New("本地 Epic 配置无法读取")
		}
	} else if !os.IsNotExist(err) {
		return nil, errors.New("无法读取本地 Epic 配置")
	}
	return s, nil
}
func (s *EpicSettings) Status() EpicStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return EpicStatus{s.config.AccountID, s.config.AccountID != "" && s.config.Refresh != "", s.config.Refresh != "", s.busy, EpicLoginURL()}
}
func (s *EpicSettings) take() (epicConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return epicConfig{}, errors.New("Epic 正在连接或同步，请稍候")
	}
	s.busy = true
	return s.config, nil
}
func (s *EpicSettings) end() { s.mu.Lock(); s.busy = false; s.mu.Unlock() }
func (s *EpicSettings) RequestSync() {
	select {
	case s.Wake <- struct{}{}:
	default:
	}
}

func epicWriteFile(dir, name string, body []byte) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.New("无法创建 Epic 配置目录")
	}
	f, err := os.CreateTemp(dir, ".epic-*") // CreateTemp always uses 0600.
	if err != nil {
		return errors.New("无法保存 Epic 本地配置")
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(body); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return errors.New("无法保存 Epic 本地配置")
	}
	if os.Rename(f.Name(), filepath.Join(dir, name)) != nil {
		return errors.New("无法替换 Epic 本地配置")
	}
	return nil
}
func (s *EpicSettings) persist(cfg epicConfig) error {
	body, _ := json.Marshal(cfg)
	if err := epicWriteFile(s.dir, "epic-settings.json", body); err != nil {
		return err
	}
	s.mu.Lock()
	s.config = cfg
	s.mu.Unlock()
	return nil
}
func (s *EpicSettings) bind(ctx context.Context, id string) error {
	if !regexp.MustCompile(`^[a-fA-F0-9]{32}$`).MatchString(id) {
		return errors.New("Epic 未返回有效账号 ID")
	}
	body, err := os.ReadFile(filepath.Join(s.dir, "epic-account"))
	if err != nil && !os.IsNotExist(err) {
		return errors.New("无法读取 Epic 账号绑定")
	}
	bound := strings.TrimSpace(string(body))
	if bound == id {
		return nil
	}
	// Include all retained hidden rows, not only games visible in the dashboard.
	var exists bool
	if err := s.store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM games WHERE platform='epic')`).Scan(&exists); err != nil {
		return errors.New("无法核对 Epic 账号")
	}
	if exists {
		return errors.New("书架已有其他 Epic 账号的数据，请使用新的 DATA_DIR")
	}
	return epicWriteFile(s.dir, "epic-account", []byte(id+"\n"))
}
func epicAuthorizationCode(input string) (string, error) {
	code := strings.TrimSpace(input)
	invalid := errors.New("请粘贴 Epic 返回的完整 JSON 或 authorizationCode 的值，失效时重新打开登录入口")
	if len(code) > 4096 {
		return "", invalid
	}
	if strings.HasPrefix(code, "{") {
		var response struct {
			Code string `json:"authorizationCode"`
		}
		if json.Unmarshal([]byte(code), &response) != nil {
			return "", invalid
		}
		code = strings.TrimSpace(response.Code)
	}
	if len(code) < 16 || len(code) > 1024 || !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(code) {
		return "", invalid
	}
	return code, nil
}

func (s *EpicSettings) Save(ctx context.Context, code string) error {
	_, err := s.take()
	if err != nil {
		return err
	}
	defer s.end()
	code, err = epicAuthorizationCode(code)
	if err != nil {
		return err
	}
	client := *s.client
	t, err := client.tokens(ctx, "authorization_code", code)
	if err != nil {
		return err
	}
	client.Token = t.Access
	id, err := client.identity(ctx)
	if err != nil {
		return err
	}
	if id != t.AccountID {
		return errors.New("Epic 登录与验证账号不一致，已停止连接")
	}
	if err := s.bind(ctx, id); err != nil {
		return err
	}
	// Code is transient; only renewable tokens reach disk.
	return s.persist(epicConfig{id, t.Access, t.Refresh, time.Now().Unix() + t.ExpiresIn})
}

func (s *EpicSettings) session(ctx context.Context, cfg epicConfig) (*Epic, error) {
	client := *s.client
	refresh := func() error {
		t, err := client.tokens(ctx, "refresh_token", cfg.Refresh)
		if err != nil {
			return err
		}
		if t.AccountID != cfg.AccountID {
			return errors.New("Epic 续期账号不一致，已停止同步")
		}
		cfg.Access, cfg.Refresh, cfg.ExpiresAt = t.Access, t.Refresh, time.Now().Unix()+t.ExpiresIn
		return nil
	}
	renewed := false
	if cfg.Access == "" || cfg.ExpiresAt <= time.Now().Unix()+120 {
		if err := refresh(); err != nil {
			return nil, err
		}
		renewed = true
	}
	client.Token = cfg.Access
	id, err := client.identity(ctx)
	if errors.Is(err, errEpicLogin) && !renewed {
		if err = refresh(); err != nil {
			return nil, err
		}
		client.Token = cfg.Access
		id, err = client.identity(ctx)
	}
	if err != nil {
		return nil, err
	}
	if id != cfg.AccountID {
		return nil, errors.New("Epic 返回的账号与本地绑定不一致，已停止同步")
	}
	if err := s.bind(ctx, id); err != nil {
		return nil, err
	}
	if err := s.persist(cfg); err != nil {
		return nil, err
	}
	return &client, nil
}

func (s *EpicSettings) Sync(ctx context.Context) (err error) {
	cfg, err := s.take()
	if err != nil {
		return err
	}
	defer s.end()
	if cfg.Refresh == "" || cfg.AccountID == "" {
		return errors.New("请先在平台设置中连接 Epic")
	}
	runID, err := s.store.StartRun(ctx, "epic")
	if err != nil {
		return errors.New("无法记录 Epic 同步")
	}
	status, count := "failed", 0
	var summary *string
	report := &SyncReport{Issues: []SyncIssue{}}
	defer func() {
		if err != nil {
			if report.LibrarySaved {
				report.add("incomplete", 1)
			}
			msg := err.Error()
			summary = &msg
		}
		finish, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := s.store.FinishRun(finish, runID, status, count, summary, report); e != nil && err == nil {
			err = errors.New("无法保存 Epic 同步状态")
		}
	}()
	client, err := s.session(ctx, cfg)
	if err != nil {
		return err
	}
	items, editions, err := client.libraryItems(ctx)
	if err != nil {
		return err
	}
	times, timeErr := client.playtimes(ctx, cfg.AccountID)
	// artifactId alone must identify exactly one full ID tuple in this library.
	artifacts := map[string]int{}
	for _, item := range items {
		artifacts[item.AppName]++
	}
	timeFailures, ambiguous := 0, 0
	defer func() {
		report.add("playtime_request", timeFailures)
		report.add("playtime_mapping", ambiguous)
	}()
	if timeErr != nil {
		timeFailures = 1
	}
	games := make([]Game, 0, len(items))
	metadataFailures, unknown := 0, 0
	defer func() {
		report.add("metadata", metadataFailures)
		report.add("classification", unknown)
	}()
	for n, item := range items {
		if n > 0 && s.Spacing > 0 {
			timer := time.NewTimer(s.Spacing)
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.New("Epic 同步中断，已有列表保留")
			case <-timer.C:
			}
		}
		g, unclassified, e := client.game(ctx, item, editions[item.id()])
		if e != nil {
			if errors.Is(e, errEpicLogin) {
				return e
			}
			metadataFailures++
			continue // Previous complete row remains untouched.
		}
		if unclassified {
			unknown++
		}
		if seconds, ok := times[item.AppName]; ok {
			if artifacts[item.AppName] == 1 {
				g.PlaytimeSeconds = &seconds
			} else {
				ambiguous++
			}
		}
		games = append(games, g)
	}
	if ctx.Err() != nil {
		return errors.New("Epic 同步中断，已有列表保留")
	}
	if len(games) == 0 {
		return errors.New("Epic 游戏元数据均不可用，已有列表保留")
	}
	if s.store.SaveLibrary(ctx, games) != nil {
		return errors.New("无法保存 Epic 游戏列表")
	}
	report.LibrarySaved = true
	status = "partial"
	visible, err := s.store.Games(ctx)
	if err != nil {
		return errors.New("无法读取 Epic 本地列表")
	}
	byID := map[string]Game{}
	for _, g := range visible {
		if g.Platform == "epic" {
			byID[g.PlatformGameID] = g
		}
	}
	coverSync := &Syncer{Store: s.store, DataDir: s.dir, Steam: NewSteam("", "")}
	// Separate client: cover requests carry no account Authorization header.
	coverHTTP := *client.HTTP
	coverHTTP.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	coverSync.Steam.HTTP = &coverHTTP
	coverFailures := 0
	defer func() { report.add("covers", coverFailures) }()
	for _, g := range games {
		stored, ok := byID[g.PlatformGameID]
		if !ok {
			continue
		}
		count++
		if ctx.Err() != nil {
			return errors.New("Epic 同步中断，已保存游戏列表")
		}
		if stored.CoverURL != nil && (stored.CoverPath == nil || !coverSync.coverExists(*stored.CoverPath)) {
			if coverSync.cacheCover(ctx, stored) != nil {
				coverFailures++
			}
		}
	}
	status = "success"
	if metadataFailures+unknown+coverFailures+timeFailures+ambiguous > 0 {
		status = "partial"
		msg := fmt.Sprintf("已保存 %d 个可见 Epic 游戏；%d 个元数据请求失败，%d 个条目分类待核对，%d 张封面暂不可用，%d 个时长请求失败，%d 个时长映射不唯一。已有数据保留；成就与最近游玩时间尚未接入。", count, metadataFailures, unknown, coverFailures, timeFailures, ambiguous)
		summary = &msg
	}
	return nil
}
