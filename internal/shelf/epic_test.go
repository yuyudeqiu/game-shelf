package shelf

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const epicTestAccount = "11111111111111111111111111111111"
const epicTestOther = "22222222222222222222222222222222"
const epicTestCode = "fixture-authorization-code-never-store"

func TestEpicAuthorizationJSONExtraction(t *testing.T) {
	full, _ := json.Marshal(map[string]any{"warning": "fixture warning", "authorizationCode": epicTestCode, "exchangeCode": nil, "sid": nil})
	for _, input := range []string{epicTestCode, " \n" + string(full) + "\n "} {
		code, err := epicAuthorizationCode(input)
		if err != nil || code != epicTestCode {
			t.Fatal("authorization code not extracted")
		}
	}
	for _, input := range []string{`{`, `{}`, `{"authorizationCode":null}`, `{"authorizationCode":123}`, `{"redirectUrl":"https://localhost/?code=fixture"}`, string(full) + `{}`, `{"authorizationCode":"invalid code"}`, strings.Repeat("x", 4097)} {
		code, err := epicAuthorizationCode(input)
		if err == nil || code != "" || strings.Contains(err.Error(), input) {
			t.Fatal("invalid input accepted or echoed")
		}
	}
	f := newEpicFixture(t)
	s := f.settings(t)
	if err := s.Save(context.Background(), string(full)); err != nil {
		t.Fatal(err)
	}
	stored, _ := os.ReadFile(filepath.Join(s.dir, "epic-settings.json"))
	if !s.Status().Configured || strings.Contains(string(stored), epicTestCode) || strings.Contains(string(stored), "fixture warning") {
		t.Fatal("JSON authorization was not transient")
	}
}

type epicFixture struct {
	server    *httptest.Server
	mode      string
	refreshes int
	items     []epicItem
	image     bool
	times     bool
}

func newEpicFixture(t *testing.T) *epicFixture {
	t.Helper()
	f := &epicFixture{items: []epicItem{{Namespace: "ns", CatalogID: "game", AppName: "app"}, {Namespace: "ns", CatalogID: "partner", AppName: "partner"}, {Namespace: "ns", CatalogID: "dlc", AppName: "dlc"}, {Namespace: "ns", CatalogID: "tool", AppName: "tool"}}}
	f.items = append(f.items, epicItem{Namespace: "private", CatalogID: "private", AppName: "private", SandboxType: "PRIVATE"}, epicItem{Namespace: "ue", CatalogID: "engine", AppName: "engine"})
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/account/api/oauth/token":
			if r.Method != "POST" {
				t.Error("token method")
			}
			user, pass, ok := r.BasicAuth()
			if !ok || user != epicClientID || pass != epicClientSecret {
				t.Error("missing launcher auth")
			}
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("grant_type") == "refresh_token" {
				f.refreshes++
				if r.Form.Get("refresh_token") != "fixture-refresh" {
					t.Error("refresh token not retained")
				}
			} else if r.Form.Get("code") != epicTestCode {
				t.Error("wrong authorization code")
			}
			if f.mode == "auth-fail" {
				http.Error(w, epicTestCode+" fixture-access fixture-refresh", 400)
				return
			}
			if f.mode == "redirect" {
				http.Redirect(w, r, f.server.URL+"/leak", http.StatusFound)
				return
			}
			refresh := "fixture-refresh"
			if r.Form.Get("grant_type") == "refresh_token" {
				refresh = "fixture-refresh-rotated"
			}
			writeJSON(w, map[string]any{"access_token": "fixture-access", "refresh_token": refresh, "account_id": epicTestAccount, "expires_in": 7200})
		case "/account/api/oauth/verify":
			if r.Header.Get("Authorization") != "bearer fixture-access" {
				t.Error("missing access token")
			}
			if f.mode == "verify-unauthorized" && f.refreshes == 0 {
				http.Error(w, "fixture-access", 401)
				return
			}
			id := epicTestAccount
			if f.mode == "other-account" {
				id = epicTestOther
			}
			writeJSON(w, map[string]string{"account_id": id})
		case "/library/api/public/items":
			if r.Header.Get("Authorization") != "bearer fixture-access" {
				t.Error("missing library token")
			}
			if f.mode == "empty" {
				io.WriteString(w, `{"records":[],"responseMetadata":{}}`)
				return
			}
			if f.mode == "missing-page" {
				io.WriteString(w, `{"records":[]}`)
				return
			}
			cursor := r.URL.Query().Get("cursor")
			if cursor == "" {
				writeJSON(w, map[string]any{"records": f.items[:1], "responseMetadata": map[string]string{"nextCursor": "page2"}})
			} else {
				if f.mode == "page-fail" {
					http.Error(w, "fixture-access", 503)
					return
				}
				next := ""
				if f.mode == "cursor-loop" {
					next = "page2"
				}
				records := append([]epicItem{}, f.items[1:]...)
				if f.mode == "conflict" {
					records = append(records, epicItem{Namespace: "ns", CatalogID: "different", AppName: "app"})
				}
				if f.mode == "invalid-id" {
					records = append(records, epicItem{Namespace: "ns", AppName: "broken"})
				}
				writeJSON(w, map[string]any{"records": records, "responseMetadata": map[string]string{"nextCursor": next}})
			}
		case "/library/api/public/playtime/account/" + epicTestAccount + "/all":
			if f.mode == "time-fail" {
				http.Error(w, "fixture-access", 503)
				return
			}
			if f.mode == "time-null" {
				io.WriteString(w, `null`)
				return
			}
			if !f.times {
				io.WriteString(w, `[]`)
				return
			}
			account := epicTestAccount
			if f.mode == "time-account" {
				account = epicTestOther
			}
			rows := []map[string]any{{"accountId": account, "artifactId": "app", "totalTime": 7201}, {"accountId": account, "artifactId": "partner", "totalTime": 0}}
			switch f.mode {
			case "time-missing":
				delete(rows[0], "totalTime")
			case "time-negative":
				rows[0]["totalTime"] = -1
			case "time-duplicate":
				rows = append(rows, rows[0])
			case "time-overflow":
				rows[0]["totalTime"] = uint64(1) << 63
			}
			writeJSON(w, rows)
		case "/launcher/api/public/assets/Windows", "/launcher/api/public/assets/Mac":
			if f.mode == "mac-fail" && strings.HasSuffix(r.URL.Path, "/Mac") {
				http.Error(w, "error", 503)
				return
			}
			assets := append([]epicItem{}, f.items[:1]...) // Both OSes point to the same exact ID.
			if len(f.items) >= 6 {
				private := f.items[4]
				private.SandboxType = "" // Asset API does not include the sandbox classification.
				assets = append(assets, private, f.items[5])
			}
			writeJSON(w, assets)
		case "/catalog/api/shared/namespace/ns/bulk/items", "/catalog/api/shared/namespace/ns2/bulk/items":
			id := r.URL.Query().Get("id")
			if r.URL.Query().Get("locale") != "zh-CN" {
				t.Error("missing Chinese locale")
			}
			if f.mode == "metadata-fail" && id == "game" {
				http.Error(w, "fixture-refresh", 503)
				return
			}
			if f.mode == "metadata-all-fail" {
				http.Error(w, "fixture-refresh", 503)
				return
			}
			category := "games"
			if id == "tool" {
				category = "applications/tools"
			}
			if f.mode == "unknown" && id == "game" {
				category = "new-category"
			}
			item := map[string]any{"id": id, "namespace": "ns", "title": "Fixture " + id, "categories": []map[string]string{{"path": category}}}
			if strings.Contains(r.URL.Path, "/ns2/") {
				item["namespace"] = "ns2"
			}
			if id == "dlc" || f.mode == "reclassify" && id == "game" {
				item["mainGameItem"] = map[string]string{"id": "base"}
			}
			if f.mode == "wrong-metadata-id" && id == "game" {
				item["id"] = "other"
			}
			if f.image && id == "game" {
				item["keyImages"] = []map[string]string{{"type": "DieselGameBox", "url": "https://cdn1.epicgames.com/fixture.jpg"}}
			}
			writeJSON(w, map[string]any{id: item})
		case "/cover":
			if r.Header.Get("Authorization") != "" {
				t.Error("cover leaked token")
			}
			if f.mode == "cover-fail" {
				http.Error(w, "bad image", 503)
				return
			}
			w.Header().Set("Content-Type", "image/jpeg")
			io.WriteString(w, "fixture-image")
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

type epicTransport struct {
	base  http.RoundTripper
	cover string
}

func (t epicTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Hostname() == "cdn1.epicgames.com" {
		r = r.Clone(r.Context())
		req, _ := http.NewRequestWithContext(r.Context(), "GET", t.cover, nil)
		r.URL, r.Host = req.URL, req.Host
	}
	return t.base.RoundTrip(r)
}
func (f *epicFixture) settings(t *testing.T) *EpicSettings {
	t.Helper()
	s, err := NewEpicSettings(t.TempDir(), testStore(t))
	if err != nil {
		t.Fatal(err)
	}
	s.client.AuthBase, s.client.LibraryBase, s.client.LauncherBase, s.client.CatalogBase = f.server.URL, f.server.URL, f.server.URL, f.server.URL
	s.client.HTTP = &http.Client{Transport: epicTransport{f.server.Client().Transport, f.server.URL + "/cover"}, Timeout: time.Second}
	s.Spacing = 0
	return s
}
func epicConnect(t *testing.T, s *EpicSettings) {
	t.Helper()
	if err := s.Save(context.Background(), epicTestCode); err != nil {
		t.Fatal(err)
	}
}

func TestEpicSyncPersistsExactIDsPlatformsUnknownsAndFilters(t *testing.T) {
	f := newEpicFixture(t)
	f.image = true
	s := f.settings(t)
	epicConnect(t, s)
	ctx := context.Background()
	for range 2 {
		if err := s.Sync(ctx); err != nil {
			t.Fatal(err)
		}
	}
	games, err := s.store.Games(ctx)
	if err != nil || len(games) != 2 {
		t.Fatal("expected base game and non-installable partner game", games, err)
	}
	for _, g := range games {
		if g.PlaytimeSeconds != nil || g.LastPlayedAt != nil || g.EarnedCount != nil || g.TotalCount != nil {
			t.Fatal("unknown metrics became zero")
		}
		if g.PlatformGameID == f.items[0].id() {
			if g.EditionPlatform == nil || *g.EditionPlatform != "Windows / Mac" || g.CoverPath == nil {
				t.Fatal("platform union or cover missing", g)
			}
			body, err := os.ReadFile(filepath.Join(s.dir, "covers", *g.CoverPath))
			if err != nil || string(body) != "fixture-image" {
				t.Fatal("cover not cached", err)
			}
		}
		if g.PlatformGameID == f.items[1].id() && g.EditionPlatform != nil {
			t.Fatal("invented platform for partner game")
		}
	}
	var total int
	if err := s.store.DB.QueryRow(`SELECT COUNT(*) FROM games WHERE platform='epic'`).Scan(&total); err != nil || total != 4 {
		t.Fatal("excluded raw rows not retained", total, err)
	}
	run, _ := s.store.LatestRun(ctx, "epic")
	if run.Status != "success" || run.ItemsSynced != 2 {
		t.Fatal("wrong sync result", run)
	}
	// Exact IDs, never title matching; delimiter-like values cannot collide.
	a := epicItem{Namespace: "a:b", CatalogID: "c", AppName: "d"}
	b := epicItem{Namespace: "a", CatalogID: "b:c", AppName: "d"}
	if a.id() == b.id() || len(a.id()) != 64 {
		t.Fatal("ambiguous or unsafe cache key")
	}
}

func TestEpicIncompleteResponsesPreservePreviousLibrary(t *testing.T) {
	for _, mode := range []string{"empty", "missing-page", "page-fail", "cursor-loop", "conflict", "invalid-id", "mac-fail", "metadata-all-fail", "other-account", "auth-fail"} {
		t.Run(mode, func(t *testing.T) {
			f := newEpicFixture(t)
			s := f.settings(t)
			epicConnect(t, s)
			ctx := context.Background()
			if err := s.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			before, _ := s.store.Games(ctx)
			f.mode = mode
			if mode == "auth-fail" {
				s.config.ExpiresAt = 0
			}
			if err := s.Sync(ctx); err == nil {
				t.Fatal("invalid response accepted")
			}
			after, _ := s.store.Games(ctx)
			x, _ := json.Marshal(before)
			y, _ := json.Marshal(after)
			if string(x) != string(y) {
				t.Fatal("failure damaged previous library")
			}
			run, _ := s.store.LatestRun(ctx, "epic")
			if run.Status != "failed" || run.ItemsSynced != 0 {
				t.Fatal("wrong failure result", run)
			}
		})
	}
}

func TestEpicMetadataPartialReclassificationAndCoverFailure(t *testing.T) {
	f := newEpicFixture(t)
	s := f.settings(t)
	epicConnect(t, s)
	ctx := context.Background()
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	var gameID int64
	if err := s.store.DB.QueryRow(`SELECT id FROM games WHERE platform_game_id=?`, f.items[0].id()).Scan(&gameID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.DB.Exec(`UPDATE games SET name='Retained name',playtime_seconds=0 WHERE id=?`, gameID); err != nil {
		t.Fatal(err)
	}
	f.mode = "metadata-fail"
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	g, err := s.store.GameDetail(ctx, gameID)
	if err != nil || g.Game.Name != "Retained name" || g.Game.PlaytimeSeconds == nil || *g.Game.PlaytimeSeconds != 0 {
		t.Fatal("metadata failure replaced old values", err)
	}
	run, _ := s.store.LatestRun(ctx, "epic")
	if run.Status != "partial" || run.ErrorSummary == nil {
		t.Fatal("missing partial diagnostics")
	}
	f.mode = "unknown"
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.GameDetail(ctx, gameID); err != nil {
		t.Fatal("unknown category hidden")
	}
	run, _ = s.store.LatestRun(ctx, "epic")
	if run.Status != "partial" || !strings.Contains(*run.ErrorSummary, "分类待核对") {
		t.Fatal("unknown category not reported")
	}
	f.mode = "reclassify"
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.store.GameDetail(ctx, gameID); err == nil {
		t.Fatal("confirmed DLC still visible")
	}
	f.mode, f.image = "cover-fail", true
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	run, _ = s.store.LatestRun(ctx, "epic")
	if run.Status != "partial" || !strings.Contains(*run.ErrorSummary, "1 张封面") {
		t.Fatal("cover failure not partial", run)
	}
	// Disappearance from a subsequent complete list is not deletion.
	f.mode = ""
	f.items = f.items[:1]
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	var total int
	s.store.DB.QueryRow(`SELECT COUNT(*) FROM games`).Scan(&total)
	if total != 4 {
		t.Fatal("missing entries were deleted")
	}
}

func TestEpicTokenRotationAndUnauthorizedRetry(t *testing.T) {
	for _, mode := range []string{"expired", "verify-unauthorized"} {
		t.Run(mode, func(t *testing.T) {
			f := newEpicFixture(t)
			s := f.settings(t)
			epicConnect(t, s)
			if mode == "expired" {
				s.config.ExpiresAt = 0
			} else {
				f.mode = mode
			}
			if err := s.Sync(context.Background()); err != nil {
				t.Fatal(err)
			}
			if f.refreshes != 1 {
				t.Fatal("expected exactly one refresh", f.refreshes)
			}
			reloaded, err := NewEpicSettings(s.dir, s.store)
			if err != nil || reloaded.config.Refresh != "fixture-refresh-rotated" || reloaded.config.ExpiresAt <= time.Now().Unix() {
				t.Fatal("rotated token not persisted", err)
			}
		})
	}
}

func TestEpicPlaytimeSecondsZeroAndFailurePreservation(t *testing.T) {
	f := newEpicFixture(t)
	f.times = true
	s := f.settings(t)
	epicConnect(t, s)
	ctx := context.Background()
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	check := func() {
		t.Helper()
		games, err := s.store.Games(ctx)
		if err != nil || len(games) != 2 {
			t.Fatal("wrong playtime library", err)
		}
		for _, g := range games {
			want := int64(0)
			if g.PlatformGameID == f.items[0].id() {
				want = 7201
			}
			if g.PlaytimeSeconds == nil || *g.PlaytimeSeconds != want || g.LastPlayedAt != nil {
				t.Fatal("seconds, zero or unknown last-played semantics changed")
			}
		}
	}
	check()
	for _, mode := range []string{"time-fail", "time-null", "time-account", "time-missing", "time-negative", "time-duplicate", "time-overflow"} {
		f.mode = mode
		if err := s.Sync(ctx); err != nil {
			t.Fatal("optional time failure blocked library", err)
		}
		check()
		run, _ := s.store.LatestRun(ctx, "epic")
		if run.Status != "partial" || run.ErrorSummary == nil || !strings.Contains(*run.ErrorSummary, "1 个时长请求失败") {
			t.Fatal("missing time failure diagnostics")
		}
	}
	f.mode, f.times = "", false
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	check() // Empty successful response does not reset previous known data to zero.
}

func TestEpicPlaytimeAmbiguousArtifactRemainsUnknown(t *testing.T) {
	f := newEpicFixture(t)
	f.times = true
	f.items = append(f.items, epicItem{Namespace: "ns2", CatalogID: "alias", AppName: "app"})
	s := f.settings(t)
	epicConnect(t, s)
	if err := s.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	games, _ := s.store.Games(context.Background())
	for _, g := range games {
		if g.PlatformGameID == f.items[0].id() || g.PlatformGameID == f.items[6].id() {
			if g.PlaytimeSeconds != nil {
				t.Fatal("ambiguous artifact mapped across namespace")
			}
		}
	}
	run, _ := s.store.LatestRun(context.Background(), "epic")
	if run.Status != "partial" || !strings.Contains(*run.ErrorSummary, "2 个时长映射不唯一") {
		t.Fatal("ambiguous mapping not reported")
	}
}

func TestEpicAccountIsolationIncludesHiddenRows(t *testing.T) {
	f := newEpicFixture(t)
	s := f.settings(t)
	epicConnect(t, s)
	ctx := context.Background()
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	// Keep only non-game rows, then corrupt/remove the binding. These still block writes.
	if _, err := s.store.DB.Exec(`DELETE FROM games WHERE platform_game_id IN (?,?)`, f.items[0].id(), f.items[1].id()); err != nil {
		t.Fatal(err)
	}
	visible, _ := s.store.Games(ctx)
	if len(visible) != 0 {
		t.Fatal("fixture should only have hidden rows")
	}
	if err := os.Remove(filepath.Join(s.dir, "epic-account")); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, epicTestCode); err == nil {
		t.Fatal("unbound hidden records allowed connection")
	}
	if err := s.Sync(ctx); err == nil {
		t.Fatal("unbound hidden records allowed sync")
	}
}

func TestEpicHTTPSettingsPrivacyAndLocalWrites(t *testing.T) {
	f := newEpicFixture(t)
	s := f.settings(t)
	h := (&Server{Store: s.store, Epic: s}).Handler()
	request := func(method, path, body, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Game-Shelf", "1")
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	body := `{"authorization_code":"` + epicTestCode + `"}`
	for _, path := range []string{"/api/settings/epic", "/api/sync/epic"} {
		if w := request("POST", path, body, "https://evil.example"); w.Code != 403 {
			t.Fatal("external write accepted")
		}
	}
	for _, invalid := range []string{`{}`, body + ` {}`, `{"authorization_code":"bad"}`, `{"authorization_code":"` + epicTestCode + `","unknown":1}`} {
		if w := request("POST", "/api/settings/epic", invalid, ""); w.Code != 400 {
			t.Fatal("bad input accepted", w.Code)
		}
	}
	w := request("POST", "/api/settings/epic", body, "http://127.0.0.1:5173")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	select {
	case <-s.Wake:
	default:
		t.Fatal("save did not request sync")
	}
	for _, name := range []string{"epic-settings.json", "epic-account"} {
		stat, err := os.Stat(filepath.Join(s.dir, name))
		if err != nil || stat.Mode().Perm() != 0600 {
			t.Fatal("unsafe file permission", err)
		}
		data, _ := os.ReadFile(filepath.Join(s.dir, name))
		if strings.Contains(string(data), epicTestCode) {
			t.Fatal("code stored on disk")
		}
	}
	for _, path := range []string{"/api/settings/epic", "/api/dashboard"} {
		w := request("GET", path, "", "")
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		for _, secret := range []string{epicTestCode, "fixture-access", "fixture-refresh"} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("API leaked credentials")
			}
		}
	}
	for _, mode := range []string{"auth-fail", "redirect"} {
		f.mode = mode
		w := request("POST", "/api/settings/epic", body, "")
		if w.Code != 400 || strings.Contains(w.Body.String(), epicTestCode) || strings.Contains(w.Body.String(), "fixture-access") {
			t.Fatal("unsafe authentication response")
		}
	}
	if !s.Status().Configured {
		t.Fatal("failed reauth destroyed previous config")
	}
}
