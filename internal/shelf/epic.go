package shelf

// Independent implementation of Epic launcher wire protocols. Reference endpoints:
// Legendary 42f6bde (docs/EPIC.md). No Legendary code or runtime dependency is used.
import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Published launcher client identifiers, not the user's account credentials.
const epicClientID = "34a02cf8f4414e29b15921876da36f9a"
const epicClientSecret = "daafbccc737745039dffe53d94fc76cf"

func EpicLoginURL() string {
	redirect := "https://www.epicgames.com/id/api/redirect?clientId=" + epicClientID + "&responseType=code"
	return "https://www.epicgames.com/id/login?" + url.Values{"redirectUrl": {redirect}}.Encode()
}

type Epic struct {
	Token, AuthBase, LibraryBase, LauncherBase, CatalogBase string
	HTTP                                                    *http.Client
}

func NewEpic() *Epic {
	return &Epic{AuthBase: "https://account-public-service-prod03.ol.epicgames.com",
		LibraryBase:  "https://library-service.live.use1a.on.epicgames.com",
		LauncherBase: "https://launcher-public-service-prod06.ol.epicgames.com",
		CatalogBase:  "https://catalog-public-service-prod06.ol.epicgames.com",
		HTTP:         &http.Client{Timeout: 25 * time.Second}}
}

var errEpicLogin = errors.New("Epic 登录已失效或授权被拒绝，请在平台设置重新登录")

// Never expose transport errors, URLs, raw response bodies or OAuth diagnostics.
func (e *Epic) request(ctx context.Context, endpoint string, form url.Values, out any) error {
	method := http.MethodGet
	var body io.Reader
	if form != nil {
		method, body = http.MethodPost, strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return errors.New("无法创建 Epic 请求")
	}
	req.Header.Set("User-Agent", "UELauncher/11.0.1-14907503+++Portal+Release-Live Windows/10.0.19041.1.256.64bit")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth(epicClientID, epicClientSecret)
	} else {
		req.Header.Set("Authorization", "bearer "+e.Token)
	}
	client := *e.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return errors.New("Epic 网络请求失败或超时，已有数据保留")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 || form != nil && res.StatusCode == 400 {
		return errEpicLogin
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("Epic 接口返回 HTTP %d，已有数据保留", res.StatusCode)
	}
	const limit = 24 << 20
	raw, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil || len(raw) > limit || json.Unmarshal(raw, out) != nil {
		return errors.New("Epic 返回的数据格式不完整，已有数据保留")
	}
	return nil
}

type epicTokens struct {
	Access    string `json:"access_token"`
	Refresh   string `json:"refresh_token"`
	AccountID string `json:"account_id"`
	ExpiresIn int64  `json:"expires_in"`
}

func (e *Epic) tokens(ctx context.Context, grant, value string) (epicTokens, error) {
	form := url.Values{"grant_type": {grant}, "token_type": {"eg1"}}
	if grant == "refresh_token" {
		form.Set("refresh_token", value)
	} else {
		form.Set("code", value)
	}
	var t epicTokens
	if err := e.request(ctx, e.AuthBase+"/account/api/oauth/token", form, &t); err != nil {
		return t, err
	}
	if t.Access == "" || t.Refresh == "" || t.AccountID == "" || t.ExpiresIn <= 0 || t.ExpiresIn > 365*24*3600 {
		return epicTokens{}, errors.New("Epic 未返回完整登录令牌，请重新登录")
	}
	return t, nil
}

func (e *Epic) identity(ctx context.Context) (string, error) {
	var payload struct {
		AccountID string `json:"account_id"`
	}
	if err := e.request(ctx, e.AuthBase+"/account/api/oauth/verify", nil, &payload); err != nil {
		return "", err
	}
	if payload.AccountID == "" {
		return "", errors.New("无法验证 Epic 账号，已停止同步")
	}
	return payload.AccountID, nil
}

// Launcher Library playtime uses seconds, keyed by artifactId (= appName),
// independently of Catalog. See Playnite's Epic protocol reference in docs/EPIC.md.
// A missing record is unknown; only an explicit totalTime:0 establishes zero.
func (e *Epic) playtimes(ctx context.Context, accountID string) (map[string]int64, error) {
	var rows []struct {
		AccountID  string `json:"accountId"`
		ArtifactID string `json:"artifactId"`
		Total      *int64 `json:"totalTime"`
	}
	err := e.request(ctx, e.LibraryBase+"/library/api/public/playtime/account/"+url.PathEscape(accountID)+"/all", nil, &rows)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, errors.New("Epic 时长响应不完整，已有时长保留")
	}
	result := make(map[string]int64, len(rows))
	for _, row := range rows {
		if row.AccountID != accountID || strings.TrimSpace(row.ArtifactID) == "" || row.Total == nil || *row.Total < 0 {
			return nil, errors.New("Epic 时长账号或记录校验失败，已有时长保留")
		}
		if _, exists := result[row.ArtifactID]; exists {
			return nil, errors.New("Epic 返回重复时长 ID，已有时长保留")
		}
		result[row.ArtifactID] = *row.Total
	}
	return result, nil
}

type epicItem struct {
	Namespace   string `json:"namespace"`
	CatalogID   string `json:"catalogItemId"`
	AppName     string `json:"appName"`
	SandboxType string `json:"sandboxType"`
}

// Hash a length-unambiguous exact-ID tuple; keep cache filenames short and safe.
func (i epicItem) id() string {
	b, _ := json.Marshal([3]string{i.Namespace, i.CatalogID, i.AppName})
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
func (i epicItem) valid() bool {
	return strings.TrimSpace(i.Namespace) != "" && strings.TrimSpace(i.CatalogID) != "" && strings.TrimSpace(i.AppName) != ""
}
func (i epicItem) excluded() bool { return i.Namespace == "ue" || i.SandboxType == "PRIVATE" }

// Complete all library pages and both platform asset lists before any game writes.
func (e *Epic) libraryItems(ctx context.Context) ([]epicItem, map[string]string, error) {
	items := map[string]epicItem{}
	blocked := map[string]bool{}
	apps := map[string]string{}
	platforms := map[string]map[string]bool{}
	add := func(i epicItem, platform string) error {
		if i.excluded() {
			if i.valid() {
				blocked[i.id()] = true
				delete(items, i.id())
			}
			return nil
		}
		if !i.valid() {
			return errors.New("Epic 库条目缺少精确 ID，已有列表保留")
		}
		key := i.id()
		if blocked[key] {
			return nil
		}
		// Different catalog IDs for the same namespace/app must be investigated.
		appKey := i.Namespace + "\x00" + i.AppName
		if old, ok := apps[appKey]; ok && old != key {
			return errors.New("Epic 库存在冲突的条目 ID，已有列表保留")
		}
		apps[appKey], items[key] = key, i
		if platform != "" {
			if platforms[key] == nil {
				platforms[key] = map[string]bool{}
			}
			platforms[key][platform] = true
		}
		return nil
	}
	cursor := ""
	seen := map[string]bool{}
	for page := 0; ; page++ {
		if page >= 1000 || seen[cursor] {
			return nil, nil, errors.New("Epic 分页游标重复或超出限制，已有列表保留")
		}
		seen[cursor] = true
		var p struct {
			Records  *[]epicItem `json:"records"`
			Metadata *struct {
				Next string `json:"nextCursor"`
			} `json:"responseMetadata"`
		}
		q := url.Values{"includeMetadata": {"true"}}
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		if err := e.request(ctx, e.LibraryBase+"/library/api/public/items?"+q.Encode(), nil, &p); err != nil {
			return nil, nil, err
		}
		if p.Records == nil || p.Metadata == nil {
			return nil, nil, errors.New("Epic 库分页不完整，已有列表保留")
		}
		for _, i := range *p.Records {
			if err := add(i, ""); err != nil {
				return nil, nil, err
			}
		}
		cursor = p.Metadata.Next
		if cursor == "" {
			break
		}
		if len(*p.Records) == 0 {
			return nil, nil, errors.New("Epic 返回空的中间分页，已有列表保留")
		}
	}
	// Treat even explicit empty library responses conservatively; assets cannot prove
	// that an empty or otherwise unavailable library response is complete.
	if len(items) == 0 {
		return nil, nil, errors.New("Epic 未返回可验证的游戏库，已有列表保留")
	}
	for _, platform := range []string{"Windows", "Mac"} {
		var assets []epicItem
		if err := e.request(ctx, e.LauncherBase+"/launcher/api/public/assets/"+platform+"?label=Live", nil, &assets); err != nil {
			return nil, nil, err
		}
		if assets == nil {
			return nil, nil, errors.New("Epic 平台列表不完整，已有列表保留")
		}
		for _, i := range assets {
			if err := add(i, platform); err != nil {
				return nil, nil, err
			}
		}
	}
	result := make([]epicItem, 0, len(items))
	editions := map[string]string{}
	for key, i := range items {
		result = append(result, i)
		var names []string
		for _, p := range []string{"Windows", "Mac"} {
			if platforms[key][p] {
				names = append(names, p)
			}
		}
		editions[key] = strings.Join(names, " / ")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].id() < result[j].id() })
	return result, editions, nil
}

type epicCategory struct {
	Paths    []string `json:"paths"`
	MainGame string   `json:"main_game,omitempty"`
}

func (c epicCategory) excluded() bool {
	if c.MainGame != "" {
		return true
	}
	for _, path := range c.Paths {
		switch path {
		case "addons", "addons/dlc", "addons/launchable", "mods", "editors", "applications/editors", "applications/tools", "tools":
			return true
		}
	}
	return false
}
func (c epicCategory) knownGame() bool {
	for _, path := range c.Paths {
		if path == "games" || path == "applications/games" {
			return true
		}
	}
	return false
}
func (g Game) isEpicNonGame() bool {
	if g.Platform != "epic" || g.SourceCategory == nil {
		return false
	}
	var c epicCategory
	return json.Unmarshal([]byte(*g.SourceCategory), &c) == nil && c.excluded()
}

func (e *Epic) game(ctx context.Context, i epicItem, edition string) (Game, bool, error) {
	var payload map[string]struct {
		ID        string `json:"id"`
		Namespace string `json:"namespace"`
		Title     string `json:"title"`
		Main      *struct {
			ID string `json:"id"`
		} `json:"mainGameItem"`
		Categories []struct {
			Path string `json:"path"`
		} `json:"categories"`
		Images []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"keyImages"`
	}
	q := url.Values{"id": {i.CatalogID}, "includeDLCDetails": {"true"}, "includeMainGameDetails": {"true"}, "locale": {"zh-CN"}}
	endpoint := e.CatalogBase + "/catalog/api/shared/namespace/" + url.PathEscape(i.Namespace) + "/bulk/items?" + q.Encode()
	if err := e.request(ctx, endpoint, nil, &payload); err != nil {
		return Game{}, false, err
	}
	m, ok := payload[i.CatalogID]
	if !ok || m.ID != i.CatalogID || m.Namespace != i.Namespace || strings.TrimSpace(m.Title) == "" {
		return Game{}, false, errors.New("Epic 游戏元数据缺失或 ID 不符")
	}
	c := epicCategory{Paths: []string{}}
	for _, v := range m.Categories {
		if v.Path != "" {
			c.Paths = append(c.Paths, v.Path)
		}
	}
	if m.Main != nil {
		if m.Main.ID == "" {
			return Game{}, false, errors.New("Epic 主游戏关系不完整")
		}
		c.MainGame = m.Main.ID
	}
	g := Game{Platform: "epic", PlatformGameID: i.id(), Name: m.Title, SourceScope: "owned"}
	if edition != "" {
		g.EditionPlatform = &edition
	}
	if len(c.Paths) > 0 || c.MainGame != "" {
		b, _ := json.Marshal(c)
		s := string(b)
		g.SourceCategory = &s
	}
	// Only accept known Epic image hosts; never send account tokens to image URLs.
	for _, kind := range []string{"DieselGameBox", "DieselGameBoxTall", "OfferImageWide", "Thumbnail", "DieselStoreFrontWide"} {
		for _, img := range m.Images {
			u, err := url.Parse(img.URL)
			if img.Type == kind && err == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" && (u.Hostname() == "cdn1.epicgames.com" || u.Hostname() == "cdn2.unrealengine.com" || u.Hostname() == "cdn.unrealengine.com") {
				cover := img.URL
				g.CoverURL = &cover
				break
			}
		}
		if g.CoverURL != nil {
			break
		}
	}
	return g, !c.knownGame() && !c.excluded(), nil
}
