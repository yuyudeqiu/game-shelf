package shelf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testNPSSO = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const psnSummaryCounts = `"definedTrophies":{"bronze":1,"silver":0,"gold":1,"platinum":0},"earnedTrophies":{"bronze":1,"silver":0,"gold":0,"platinum":0}`

type psnFixture struct {
	server      *httptest.Server
	mode        string
	account     string
	tokens      int
	definitions int
}

func newPSNFixture(t *testing.T) *psnFixture {
	t.Helper()
	f := &psnFixture{account: "123456789"}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/oauth/authorize":
			if r.Header.Get("Cookie") != "npsso="+testNPSSO {
				t.Error("NPSSO not provided to authorize")
			}
			w.Header().Set("Location", psnRedirect+"/?code=test-auth-code")
			w.WriteHeader(302)
			return
		case "/oauth/token":
			if r.Header.Get("Authorization") != psnClientAuth {
				t.Error("missing public-client auth")
			}
			r.ParseForm()
			if f.tokens > 0 && r.Form.Get("grant_type") != "refresh_token" && f.mode != "other-account" && f.mode != "bad-auth" {
				t.Error("expected refresh grant")
			}
			if f.mode == "bad-auth" {
				http.Error(w, "private-token-response-must-not-leak", 401)
				return
			}
			f.tokens++
			fmt.Fprintf(w, `{"access_token":"private-access-%d","refresh_token":"private-refresh-%d","expires_in":3600}`, f.tokens, f.tokens)
			return
		}
		if r.Header.Get("Cookie") != "" {
			t.Error("NPSSO leaked to data API")
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer private-access-") {
			t.Error("missing data bearer")
		}
		if r.Header.Get("Accept-Language") != "zh-Hans" {
			t.Error("missing localization header")
		}
		switch r.URL.Path {
		case "/api/trophy/v1/users/me/trophySummary":
			fmt.Fprintf(w, `{"accountId":%q}`, f.account)
		case "/api/gamelist/v2/users/me/titles":
			if f.mode == "missing-total" {
				io.WriteString(w, `{"titles":[]}`)
				return
			}
			if f.mode == "private" {
				http.Error(w, "private-api-response-must-not-leak", 403)
				return
			}
			if r.URL.Query().Get("offset") == "0" {
				io.WriteString(w, `{"titles":[{"titleId":"CUSA12345_00","name":"Shared game","category":"ps4_game","playDuration":"PT0S"}],"totalItemCount":2,"nextOffset":1}`)
			} else {
				if f.mode == "page-fail" {
					http.Error(w, "failed second page", 503)
					return
				}
				if f.mode == "repeat-page" {
					io.WriteString(w, `{"titles":[{"titleId":"CUSA12345_00","name":"Repeated"}],"totalItemCount":2}`)
					return
				}
				io.WriteString(w, `{"titles":[{"titleId":"PPSA12345_00","name":"Shared game","category":"ps5_native_game","playDuration":"PT1H2M3S","lastPlayedDateTime":"2026-01-01T12:30:00.123Z"}],"totalItemCount":2}`)
			}
		case "/api/trophy/v1/users/me/titles/trophyTitles":
			ids := strings.Split(r.URL.Query().Get("npTitleIds"), ",")
			io.WriteString(w, `{"titles":[`)
			for i, id := range ids {
				if i > 0 {
					io.WriteString(w, ",")
				}
				fmt.Fprintf(w, `{"npTitleId":%q,"trophyTitles":[{"npCommunicationId":"NPWR12345_00","npServiceName":"trophy",%s}]}`, id, psnSummaryCounts)
			}
			io.WriteString(w, ` ]}`)
		case "/api/trophy/v1/npCommunicationIds/NPWR12345_00/trophyGroups":
			if f.mode == "groups-fail" {
				http.Error(w, "group lookup unavailable", 503)
				return
			}
			io.WriteString(w, `{"trophyGroups":[{"trophyGroupId":"default","trophyGroupName":"本篇"},{"trophyGroupId":"001","trophyGroupName":"额外冒险"}]}`)
		case "/api/trophy/v1/npCommunicationIds/NPWR12345_00/trophyGroups/all/trophies":
			if r.URL.Query().Get("npServiceName") != "trophy" {
				t.Error("wrong PS4 trophy service")
			}
			f.definitions++
			io.WriteString(w, `{"totalItemCount":2,"trophies":[{"trophyId":0,"trophyName":"起点","trophyDetail":"完成教程","trophyType":"bronze","trophyGroupId":"default"},{"trophyId":1,"trophyName":"秘密终点","trophyHidden":true,"trophyType":"gold","trophyGroupId":"001"}]}`)
		case "/api/trophy/v1/users/me/npCommunicationIds/NPWR12345_00/trophyGroups/all/trophies":
			if f.mode == "detail-fail" {
				io.WriteString(w, `{"totalItemCount":1,"trophies":[{"trophyId":0,"earned":true}]}`)
				return
			}
			io.WriteString(w, `{"totalItemCount":2,"trophies":[{"trophyId":1,"earned":false},{"trophyId":0,"earned":true,"earnedDateTime":"2025-12-30T01:02:03Z"}]}`)
		default:
			t.Errorf("unexpected PSN path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}
func (f *psnFixture) client() *PSN {
	p := NewPSN("")
	p.APIBase = f.server.URL + "/api"
	p.AuthBase = f.server.URL + "/oauth"
	p.HTTP = f.server.Client()
	p.Spacing = time.Nanosecond
	return p
}

func TestPSNAuthSyncRefreshAndSharedTrophyDetails(t *testing.T) {
	ctx := context.Background()
	fixture := newPSNFixture(t)
	store := testStore(t)
	dir := t.TempDir()
	settings, err := NewPSNSettings(dir, store)
	if err != nil {
		t.Fatal(err)
	}
	settings.client = fixture.client()
	if err = settings.Save(ctx, testNPSSO); err != nil {
		t.Fatal(err)
	}
	if err = settings.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	games, err := store.Games(ctx)
	if err != nil || len(games) != 2 {
		t.Fatal("library not saved", err)
	}
	var ps4, ps5 Game
	for _, g := range games {
		if *g.EditionPlatform == "PS4" {
			ps4 = g
		} else {
			ps5 = g
		}
	}
	if ps4.PlaytimeSeconds == nil || *ps4.PlaytimeSeconds != 0 || ps4.LastPlayedAt != nil || ps5.PlaytimeSeconds == nil || *ps5.PlaytimeSeconds != 3723 || ps5.LastPlayedAt == nil {
		t.Fatal("null/zero/duration semantics incorrect")
	}
	if ps4.AchievementSetID == nil || ps5.AchievementSetID == nil || *ps4.AchievementSetID != *ps5.AchievementSetID || Summarize(games).Metrics.Earned != 1 {
		t.Fatal("shared trophy set double counted")
	}
	detail, err := store.GameDetail(ctx, ps4.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Achievements) != 2 || !detail.Achievements[0].Achieved || detail.Achievements[0].UnlockedAt == nil || detail.Achievements[1].Achieved || !detail.Achievements[1].Hidden || detail.Achievements[1].TrophyType != "gold" || detail.Achievements[1].GroupID != "001" {
		t.Fatalf("wrong trophy join: %+v", detail)
	}
	if fixture.definitions != 1 {
		t.Fatal("shared definition fetched more than once")
	}
	if len(detail.Groups) != 2 || detail.Groups[1].Name != "额外冒险" || detail.GroupsUpdatedAt == nil {
		t.Fatal("group names not persisted")
	}
	store.DB.Exec(`UPDATE achievement_details SET groups_updated_at=1`)
	fixture.mode = "groups-fail"
	if err = settings.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	preservedGroups, _ := store.GameDetail(ctx, ps4.ID)
	if len(preservedGroups.Groups) != 2 || *preservedGroups.GroupsUpdatedAt != 1 {
		t.Fatal("failed group fetch erased metadata")
	}
	fixture.mode = ""

	settings.mu.Lock()
	settings.config.ExpiresAt = 1
	settings.mu.Unlock()
	if err = settings.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if fixture.tokens != 2 {
		t.Fatal("expired access token not refreshed")
	}
	if fixture.definitions != 1 {
		t.Fatal("fresh details unnecessarily fetched")
	}
	run, _ := store.LatestRun(ctx, "psn")
	if run.Status != "success" || run.ItemsSynced != 2 {
		t.Fatal("success state missing")
	}
	stored, err := os.ReadFile(filepath.Join(dir, "psn-settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), testNPSSO) || !strings.Contains(string(stored), "private-refresh-2") {
		t.Fatal("NPSSO persisted or refreshed token not saved")
	}
	stat, _ := os.Stat(filepath.Join(dir, "psn-settings.json"))
	if stat.Mode().Perm() != 0600 {
		t.Fatal("PSN tokens must be 0600")
	}
	reloaded, err := NewPSNSettings(dir, store)
	if err != nil || !reloaded.Status().Configured {
		t.Fatal("login not persistent", err)
	}
	// Summary updates must not postpone stale detail refresh indefinitely.
	store.DB.Exec(`UPDATE achievement_details SET updated_at=1`)
	fixture.mode = "detail-fail"
	if err = settings.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	preserved, _ := store.GameDetail(ctx, ps4.ID)
	if len(preserved.Achievements) != 2 || *preserved.DetailsUpdatedAt != 1 {
		t.Fatal("partial trophy response replaced saved details")
	}
	run, _ = store.LatestRun(ctx, "psn")
	if run.Status != "partial" {
		t.Fatal("partial details not reported")
	}
	for _, mode := range []string{"page-fail", "repeat-page", "missing-total", "private"} {
		fixture.mode = mode
		if err = settings.Sync(ctx); err == nil {
			t.Fatalf("accepted incomplete library %s", mode)
		}
		kept, _ := store.Games(ctx)
		if len(kept) != 2 || kept[0].Name != "Shared game" {
			t.Fatal("failed library damaged previous games")
		}
	}
	fixture.mode = "other-account"
	fixture.account = "987654321"
	if err = settings.Save(ctx, testNPSSO); err == nil {
		t.Fatal("mixed PSN accounts allowed")
	}
	if settings.Status().AccountID != "123456789" {
		t.Fatal("wrong-account login replaced config")
	}
}

func TestPSNSettingsHTTPProtectsAndRedactsTokens(t *testing.T) {
	fixture := newPSNFixture(t)
	store := testStore(t)
	settings, _ := NewPSNSettings(t.TempDir(), store)
	settings.client = fixture.client()
	handler := (&Server{Store: store, PSN: settings}).Handler()
	save := func(body, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1/api/settings/psn", strings.NewReader(body))
		r.Header.Set("X-Game-Shelf", "1")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	body := `{"npsso":"` + testNPSSO + `"}`
	if w := save(body, "https://evil.example"); w.Code != 403 || fixture.tokens != 0 {
		t.Fatal("cross-site auth accepted")
	}
	for _, input := range []string{`{"npsso":"short"}`, `{"npsso":"x","account_id":"9"}`} {
		if w := save(input, "http://127.0.0.1"); w.Code != 400 {
			t.Fatal("invalid configuration accepted")
		}
	}
	w := save(body, "http://localhost:5173")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	for _, secret := range []string{testNPSSO, "private-access", "private-refresh"} {
		if strings.Contains(w.Body.String(), secret) {
			t.Fatal("POST leaked credential")
		}
	}
	select {
	case <-settings.Wake:
	default:
		t.Fatal("save did not trigger sync")
	}
	if err := settings.Save(context.Background(), ""); err != nil {
		t.Fatal("blank NPSSO should retain session")
	}
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest("GET", "/api/settings/psn", nil))
	var status PSNStatus
	if json.Unmarshal(get.Body.Bytes(), &status) != nil || !status.Configured {
		t.Fatal("configured state not returned")
	}
	if strings.Contains(get.Body.String(), "private-") {
		t.Fatal("GET leaked token")
	}
	_, err := settings.take()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Save(context.Background(), testNPSSO) == nil {
		t.Fatal("edited while syncing")
	}
	settings.end()
	fixture.mode = "bad-auth"
	w = save(body, "http://localhost")
	if w.Code != 400 || strings.Contains(w.Body.String(), "private-token-response") {
		t.Fatal("auth failure leaked body")
	}
	if !settings.Status().Configured {
		t.Fatal("bad replacement erased good login")
	}
}

func TestPSNDurationAndUnknownFields(t *testing.T) {
	for input, want := range map[string]int64{"PT0S": 0, "PT228H56M33S": 824193, "P1DT2H3M4S": 93784, "PT1.9S": 1} {
		got := psnDuration(input)
		if got == nil || *got != want {
			t.Fatalf("%s: %v", input, got)
		}
	}
	for _, input := range []string{"", "P", "PT", "tomorrow", "PT-1H", "P99999999999999999999999999999D"} {
		if psnDuration(input) != nil {
			t.Fatalf("invalid duration %s", input)
		}
	}
	if psnTime("") != nil || psnImage("https://evil.example/image.jpg") != nil {
		t.Fatal("unknown values inferred")
	}
}

func TestPSNExplicitRefreshBypassesFreshEnglishCache(t *testing.T) {
	fixture := newPSNFixture(t)
	store := testStore(t)
	settings, err := NewPSNSettings(t.TempDir(), store)
	if err != nil {
		t.Fatal(err)
	}
	settings.client = fixture.client()
	ctx := context.Background()
	if err = settings.Save(ctx, testNPSSO); err != nil {
		t.Fatal(err)
	}
	if err = settings.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err = settings.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if fixture.definitions != 1 {
		t.Fatal("normal sync ignored fresh cache")
	}
	if err = settings.RefreshDetails(ctx); err != nil {
		t.Fatal(err)
	}
	if fixture.definitions != 2 {
		t.Fatal("explicit refresh did not replace cached text")
	}
	fixture.mode = "detail-fail"
	if err = settings.RefreshDetails(ctx); err != nil {
		t.Fatal(err)
	}
	games, _ := store.Games(ctx)
	detail, _ := store.GameDetail(ctx, games[0].ID)
	if len(detail.Achievements) != 2 || detail.Achievements[0].Name != "起点" {
		t.Fatal("forced refresh failure damaged previous text")
	}
}
