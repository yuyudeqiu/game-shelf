package shelf

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

type steamConfig struct {
	Key string `json:"steam_api_key"`
	ID  string `json:"steam_id"`
}
type SettingsStatus struct {
	SteamID    string `json:"steam_id"`
	HasKey     bool   `json:"has_key"`
	Configured bool   `json:"configured"`
	Source     string `json:"source"`
	Busy       bool   `json:"busy"`
}
type Settings struct {
	mu     sync.Mutex
	config steamConfig
	source string
	busy   bool
	dir    string
	store  *Store
	Wake   chan struct{}
}

func NewSettings(dir string, store *Store, key, id string) (*Settings, error) {
	s := &Settings{dir: dir, store: store, config: steamConfig{strings.TrimSpace(key), strings.TrimSpace(id)}, source: "environment", Wake: make(chan struct{}, 1)}
	f, err := os.Open(filepath.Join(dir, "settings.json"))
	if err == nil {
		defer f.Close()
		if json.NewDecoder(io.LimitReader(f, 16384)).Decode(&s.config) != nil {
			return nil, errors.New("本地平台配置无法读取，请检查 data/settings.json")
		}
		s.source = "local"
	} else if !os.IsNotExist(err) {
		return nil, errors.New("无法读取本地平台配置")
	}
	if s.config.Key == "" && s.config.ID == "" {
		s.source = "none"
	}
	return s, nil
}
func (s *Settings) Status() SettingsStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SettingsStatus{s.config.ID, s.config.Key != "", s.config.Key != "" && regexp.MustCompile(`^\d{17}$`).MatchString(s.config.ID), s.source, s.busy}
}
func (s *Settings) Save(ctx context.Context, key, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return errors.New("正在同步，请等待本次同步结束后再修改配置")
	}
	key, id = strings.TrimSpace(key), strings.TrimSpace(id)
	if key == "" {
		key = s.config.Key
	}
	if !regexp.MustCompile(`^[a-fA-F0-9]{32}$`).MatchString(key) {
		return errors.New("请填写 32 位十六进制 Steam API Key")
	}
	if !regexp.MustCompile(`^\d{17}$`).MatchString(id) {
		return errors.New("SteamID64 必须是 17 位数字")
	}
	if err := s.checkAccount(ctx, id); err != nil {
		return err
	}
	body, err := json.Marshal(steamConfig{key, id})
	if err != nil {
		return errors.New("无法保存平台配置")
	}
	if err = os.MkdirAll(s.dir, 0700); err != nil {
		return errors.New("无法创建本地配置目录")
	}
	f, err := os.CreateTemp(s.dir, "settings-*")
	if err != nil {
		return errors.New("无法保存平台配置")
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(body); err != nil {
		f.Close()
		return errors.New("无法保存平台配置")
	}
	if err = f.Close(); err != nil {
		return errors.New("无法保存平台配置")
	}
	if err = os.Rename(f.Name(), filepath.Join(s.dir, "settings.json")); err != nil {
		return errors.New("无法保存平台配置")
	}
	s.config = steamConfig{key, id}
	s.source = "local"
	return nil
}
func (s *Settings) checkAccount(ctx context.Context, id string) error {
	body, err := os.ReadFile(filepath.Join(s.dir, "steam-account"))
	if err != nil && !os.IsNotExist(err) {
		return errors.New("无法读取账号绑定")
	}
	if strings.TrimSpace(string(body)) == id {
		return nil
	}
	games, err := s.store.Games(ctx)
	if err != nil {
		return errors.New("无法检查已同步账号")
	}
	for _, g := range games {
		if g.Platform == "steam" {
			return errors.New("当前书架已有另一个 Steam 账号的数据，请使用新的 DATA_DIR 创建独立书架")
		}
	}
	return nil
}
func (s *Settings) RequestSync() {
	select {
	case s.Wake <- struct{}{}:
	default:
	}
}

// Begin snapshots credentials for a whole sync so edits cannot mix accounts mid-run.
func (s *Settings) Begin(ctx context.Context) (*Steam, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return nil, errors.New("正在同步")
	}
	if s.config.Key == "" || !regexp.MustCompile(`^\d{17}$`).MatchString(s.config.ID) {
		return nil, errors.New("请先在平台设置中填写 Steam API Key 和 SteamID64")
	}
	if err := s.checkAccount(ctx, s.config.ID); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(s.dir, "steam-account"), []byte(s.config.ID+"\n"), 0600); err != nil {
		return nil, errors.New("无法保存账号绑定")
	}
	s.busy = true
	return NewSteam(s.config.Key, s.config.ID), nil
}
func (s *Settings) End() { s.mu.Lock(); s.busy = false; s.mu.Unlock() }
