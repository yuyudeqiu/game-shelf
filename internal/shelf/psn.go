package shelf

// Wire formats/endpoints verified against achievements-app/psn-api (see docs/PROJECT.md).
// This is an unofficial Sony API; return sanitized errors, never response bodies/tokens.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type PSN struct {
	Language string
	Token    string
	APIBase  string
	AuthBase string
	HTTP     *http.Client
	Spacing  time.Duration
	lastCall time.Time
}

func NewPSN(token string) *PSN {
	return &PSN{Token: token, APIBase: "https://m.np.playstation.com/api", AuthBase: "https://ca.account.sony.com/api/authz/v3/oauth", Spacing: 250 * time.Millisecond, HTTP: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type psnTokens struct {
	Access    string `json:"access_token"`
	Refresh   string `json:"refresh_token"`
	ExpiresIn int64  `json:"expires_in"`
}

const psnRedirect = "com.scee.psxandroid.scecompcall://redirect"

// Public mobile-client identifier from the documented community auth flow, not a user secret.
const psnClientAuth = "Basic MDk1MTUxNTktNzIzNy00MzcwLTliNDAtMzgwNmU2N2MwODkxOnVjUGprYTV0bnRCMktxc1A="

func (p *PSN) authorize(ctx context.Context, npsso string) (psnTokens, error) {
	params := url.Values{"access_type": {"offline"}, "client_id": {"09515159-7237-4370-9b40-3806e67c0891"}, "redirect_uri": {psnRedirect}, "response_type": {"code"}, "scope": {"psn:mobile.v2.core psn:clientapp"}}
	req, _ := http.NewRequestWithContext(ctx, "GET", p.AuthBase+"/authorize?"+params.Encode(), nil)
	req.Header.Set("Cookie", "npsso="+npsso)
	// Never follow OAuth redirects or send NPSSO to a redirected host.
	client := *p.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return psnTokens{}, errors.New("PSN 登录请求失败或超时")
	}
	defer res.Body.Close()
	redirect, err := url.Parse(res.Header.Get("Location"))
	if err != nil || redirect.Scheme != "com.scee.psxandroid.scecompcall" || redirect.Host != "redirect" || redirect.Query().Get("code") == "" || res.StatusCode < 300 || res.StatusCode >= 400 {
		return psnTokens{}, errors.New("PSN 授权失败，请重新登录 Sony 并获取 NPSSO")
	}
	return p.token(ctx, url.Values{"code": {redirect.Query().Get("code")}, "redirect_uri": {psnRedirect}, "grant_type": {"authorization_code"}, "token_format": {"jwt"}})
}
func (p *PSN) token(ctx context.Context, form url.Values) (psnTokens, error) {
	req, _ := http.NewRequestWithContext(ctx, "POST", p.AuthBase+"/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", psnClientAuth)
	res, err := p.HTTP.Do(req)
	if err != nil {
		return psnTokens{}, errors.New("PSN 令牌请求失败或超时")
	}
	defer res.Body.Close()
	var result psnTokens
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&result) != nil || result.Access == "" || result.Refresh == "" || result.ExpiresIn < 60 {
		return psnTokens{}, errors.New("PSN 登录已失效，请在平台设置中重新填写 NPSSO")
	}
	return result, nil
}
func (p *PSN) get(ctx context.Context, path string, params url.Values, out any) error {
	if delay := p.Spacing - time.Since(p.lastCall); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	p.lastCall = time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", p.APIBase+path+"?"+params.Encode(), nil)
	if err != nil {
		return errors.New("PSN 请求无法创建")
	}
	req.Header.Set("Authorization", "Bearer "+p.Token)
	language := p.Language
	if language == "" {
		// Sony trophy endpoints use script tags; region tags silently fall back to English.
		language = "zh-Hans"
	}
	req.Header.Set("Accept-Language", language)
	req.Header.Set("Accept", "application/json")
	res, err := p.HTTP.Do(req)
	if err != nil {
		return errors.New("PSN 请求失败或超时")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 {
		return errors.New("PSN 登录已失效，请重新连接")
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("PSN 返回 HTTP %d；数据已保留", res.StatusCode)
	}
	var body json.RawMessage
	if json.NewDecoder(io.LimitReader(res.Body, 20<<20)).Decode(&body) != nil {
		return errors.New("PSN 返回无效数据")
	}
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(body, &envelope) != nil || len(envelope.Error) > 0 && string(envelope.Error) != "null" {
		return errors.New("PSN 数据不可用，请检查账号隐私或稍后重试")
	}
	if json.Unmarshal(body, out) != nil {
		return errors.New("PSN 数据格式不完整")
	}
	return nil
}
func (p *PSN) identity(ctx context.Context) (string, error) {
	var result struct {
		AccountID string `json:"accountId"`
	}
	if err := p.get(ctx, "/trophy/v1/users/me/trophySummary", nil, &result); err != nil {
		return "", err
	}
	if !regexp.MustCompile(`^[0-9]{1,20}$`).MatchString(result.AccountID) {
		return "", errors.New("PSN 未返回有效账号标识")
	}
	return result.AccountID, nil
}

// Stop on missing/unstable totals, repeated offsets, duplicates (validated by callers),
// or incomplete pages. Only a fully fetched library is eligible for persistence.
func psnPages[T any](ctx context.Context, p *PSN, path string, params url.Values, key string) ([]T, error) {
	if params == nil {
		params = url.Values{}
	}
	result := []T{}
	offset, total := 0, -1
	for {
		if offset > 100000 {
			return nil, errors.New("PSN 分页超出范围")
		}
		params.Set("limit", "100")
		params.Set("offset", strconv.Itoa(offset))
		var payload map[string]json.RawMessage
		if err := p.get(ctx, path, params, &payload); err != nil {
			return nil, err
		}
		var count *int
		var items []T
		var next *int
		if json.Unmarshal(payload["totalItemCount"], &count) != nil || count == nil || *count < 0 || *count > 100000 || json.Unmarshal(payload[key], &items) != nil || string(payload[key]) == "null" {
			return nil, errors.New("PSN 列表不完整，保留已有数据")
		}
		if total < 0 {
			total = *count
		}
		if total != *count {
			return nil, errors.New("PSN 列表在分页期间变化，请稍后重试")
		}
		if len(result)+len(items) > total || len(items) == 0 && len(result) < total {
			return nil, errors.New("PSN 分页数据不完整")
		}
		result = append(result, items...)
		if len(result) == total {
			return result, nil
		}
		if raw := payload["nextOffset"]; len(raw) > 0 && string(raw) != "null" {
			if json.Unmarshal(raw, &next) != nil {
				return nil, errors.New("PSN 分页标识无效")
			}
		}
		expected := offset + len(items)
		if next != nil && *next != expected {
			return nil, errors.New("PSN 分页标识不连续")
		}
		offset = expected
	}
}

type psnPlayed struct {
	ID         string `json:"titleId"`
	Name       string `json:"name"`
	LocalName  string `json:"localizedName"`
	Image      string `json:"imageUrl"`
	LocalImage string `json:"localizedImageUrl"`
	Category   string `json:"category"`
	Duration   string `json:"playDuration"`
	LastPlayed string `json:"lastPlayedDateTime"`
}

var psnTitleID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
var isoPlaytime = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+(?:\.\d+)?)S)?)?$`)

func psnDuration(value string) *int64 {
	match := isoPlaytime.FindStringSubmatch(value)
	if match == nil || value == "P" || value == "PT" {
		return nil
	}
	factors := []float64{86400, 3600, 60, 1}
	var total float64
	for i, part := range match[1:] {
		if part != "" {
			v, err := strconv.ParseFloat(part, 64)
			if err != nil {
				return nil
			}
			total += v * factors[i]
		}
	}
	if total > float64(1<<53) {
		return nil
	}
	seconds := int64(total)
	return &seconds
}
func psnTime(value string) *int64 {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || t.Unix() <= 0 {
		return nil
	}
	stamp := t.Unix()
	return &stamp
}
func psnImage(value string) *string {
	u, err := url.Parse(value)
	if err != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return nil
	}
	host := u.Hostname()
	if !strings.HasSuffix(host, ".playstation.com") && !strings.HasSuffix(host, ".playstation.net") {
		return nil
	}
	u.Scheme = "https"
	v := u.String()
	return &v
}
func (p *PSN) Library(ctx context.Context) ([]Game, error) {
	items, err := psnPages[psnPlayed](ctx, p, "/gamelist/v2/users/me/titles", nil, "titles")
	if err != nil {
		return nil, err
	}
	games := []Game{}
	seen := map[string]bool{}
	for _, item := range items {
		name := item.LocalName
		if name == "" {
			name = item.Name
		}
		if !psnTitleID.MatchString(item.ID) || seen[item.ID] || strings.TrimSpace(name) == "" {
			return nil, errors.New("PSN 游戏条目无效或重复")
		}
		seen[item.ID] = true
		edition := map[string]string{"ps4_game": "PS4", "ps5_native_game": "PS5", "pspc_game": "PC"}[item.Category]
		if edition == "" {
			edition = "PSN"
		}
		image := psnImage(item.LocalImage)
		if image == nil {
			image = psnImage(item.Image)
		}
		var category *string
		if item.Category != "" {
			value := item.Category
			category = &value
		}
		games = append(games, Game{Platform: "psn", PlatformGameID: item.ID, Name: name, EditionPlatform: &edition, CoverURL: image, PlaytimeSeconds: psnDuration(item.Duration), LastPlayedAt: psnTime(item.LastPlayed), SourceScope: "played", SourceCategory: category})
	}
	return games, nil
}

// Only categories confirmed as media applications are excluded. Missing/new
// categories remain visible; names, title IDs and trophy availability are not classifiers.
func (g Game) isPSNMediaApp() bool {
	if g.Platform != "psn" || g.SourceCategory == nil {
		return false
	}
	switch *g.SourceCategory {
	case "ps5_web_based_media_app", "ps5_native_media_app":
		return true
	default:
		return false
	}
}

type psnCounts struct {
	Bronze   *int64 `json:"bronze"`
	Silver   *int64 `json:"silver"`
	Gold     *int64 `json:"gold"`
	Platinum *int64 `json:"platinum"`
}

func (c psnCounts) sum() (int64, bool) {
	var total int64
	for _, v := range []*int64{c.Bronze, c.Silver, c.Gold, c.Platinum} {
		if v == nil || *v < 0 || *v > 100000 {
			return 0, false
		}
		total += *v
	}
	return total, true
}

type psnTrophyTitle struct {
	ID      string    `json:"npCommunicationId"`
	Service string    `json:"npServiceName"`
	Defined psnCounts `json:"definedTrophies"`
	Earned  psnCounts `json:"earnedTrophies"`
}

func (t psnTrophyTitle) setID() string { return t.Service + ":" + t.ID }
func (p *PSN) TrophyMappings(ctx context.Context, ids []string) (map[string]psnTrophyTitle, error) {
	var result struct {
		Titles *[]struct {
			ID       string           `json:"npTitleId"`
			Trophies []psnTrophyTitle `json:"trophyTitles"`
		} `json:"titles"`
	}
	if err := p.get(ctx, "/trophy/v1/users/me/titles/trophyTitles", url.Values{"npTitleIds": {strings.Join(ids, ",")}}, &result); err != nil {
		return nil, err
	}
	if result.Titles == nil {
		return nil, errors.New("PSN 奖杯映射缺失")
	}
	out := map[string]psnTrophyTitle{}
	allowed := map[string]bool{}
	seen := map[string]bool{}
	for _, id := range ids {
		allowed[id] = true
	}
	for _, t := range *result.Titles {
		if !allowed[t.ID] || seen[t.ID] {
			return nil, errors.New("PSN 奖杯映射不完整")
		}
		seen[t.ID] = true
		// Multiple sets are ambiguous; leave unknown rather than guess by game name.
		if len(t.Trophies) != 1 {
			continue
		}
		item := t.Trophies[0]
		if !psnTitleID.MatchString(item.ID) || (item.Service != "trophy" && item.Service != "trophy2") {
			continue
		}
		out[t.ID] = item
	}
	return out, nil
}

type psnTrophy struct {
	ID          *int64 `json:"trophyId"`
	Name        string `json:"trophyName"`
	Description string `json:"trophyDetail"`
	Icon        string `json:"trophyIconUrl"`
	Hidden      bool   `json:"trophyHidden"`
	Type        string `json:"trophyType"`
	Group       string `json:"trophyGroupId"`
	Earned      *bool  `json:"earned"`
	Date        string `json:"earnedDateTime"`
}

func (p *PSN) TrophyDetails(ctx context.Context, title psnTrophyTitle) ([]Achievement, error) {
	suffix := "/npCommunicationIds/" + url.PathEscape(title.ID) + "/trophyGroups/all/trophies"
	definitions, err := psnPages[psnTrophy](ctx, p, "/trophy/v1"+suffix, url.Values{"npServiceName": {title.Service}}, "trophies")
	if err != nil {
		return nil, err
	}
	states, err := psnPages[psnTrophy](ctx, p, "/trophy/v1/users/me"+suffix, url.Values{"npServiceName": {title.Service}}, "trophies")
	if err != nil {
		return nil, err
	}
	personal := map[int64]psnTrophy{}
	for _, state := range states {
		if state.ID == nil || *state.ID < 0 || state.Earned == nil {
			return nil, errors.New("PSN 奖杯状态不完整")
		}
		if _, exists := personal[*state.ID]; exists {
			return nil, errors.New("PSN 奖杯状态重复")
		}
		personal[*state.ID] = state
	}
	items := []Achievement{}
	seen := map[int64]bool{}
	for _, def := range definitions {
		if def.ID == nil || seen[*def.ID] {
			return nil, errors.New("PSN 奖杯定义无效")
		}
		seen[*def.ID] = true
		state, exists := personal[*def.ID]
		if !exists {
			return nil, errors.New("PSN 奖杯定义与状态不匹配")
		}
		name := def.Name
		if name == "" {
			name = "奖杯 " + strconv.FormatInt(*def.ID, 10)
		}
		icon := ""
		if image := psnImage(def.Icon); image != nil {
			icon = *image
		}
		a := Achievement{APIName: strconv.FormatInt(*def.ID, 10), Name: name, Description: def.Description, Icon: icon, IconGray: icon, Hidden: def.Hidden, Achieved: *state.Earned, TrophyType: def.Type, GroupID: def.Group}
		if a.Achieved {
			a.UnlockedAt = psnTime(state.Date)
		}
		items = append(items, a)
	}
	if len(items) != len(states) {
		return nil, errors.New("PSN 奖杯状态不完整")
	}
	return items, nil
}

func (p *PSN) TrophyGroups(ctx context.Context, title psnTrophyTitle) ([]AchievementGroup, error) {
	var payload struct {
		Groups *[]struct {
			ID   string `json:"trophyGroupId"`
			Name string `json:"trophyGroupName"`
			Icon string `json:"trophyGroupIconUrl"`
		} `json:"trophyGroups"`
	}
	if err := p.get(ctx, "/trophy/v1/npCommunicationIds/"+url.PathEscape(title.ID)+"/trophyGroups", url.Values{"npServiceName": {title.Service}}, &payload); err != nil {
		return nil, err
	}
	if payload.Groups == nil || len(*payload.Groups) == 0 {
		return nil, errors.New("PSN 未返回奖杯分组")
	}
	groups := []AchievementGroup{}
	seen := map[string]bool{}
	for _, item := range *payload.Groups {
		if item.ID == "" || seen[item.ID] || strings.TrimSpace(item.Name) == "" {
			return nil, errors.New("PSN 奖杯分组不完整")
		}
		seen[item.ID] = true
		icon := ""
		if image := psnImage(item.Icon); image != nil {
			icon = *image
		}
		groups = append(groups, AchievementGroup{ID: item.ID, Name: item.Name, Icon: icon})
	}
	return groups, nil
}
