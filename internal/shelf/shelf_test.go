package shelf

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ptr(v int64) *int64 { return &v }
func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.DB.Close() })
	return s
}

func TestSyncPersistsIdempotentlyAndPreservesOnFailures(t *testing.T) {
	ctx := context.Background()
	mode := "ok"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "GetOwnedGames"):
			if mode == "private" {
				io.WriteString(w, `{"response":{}}`)
				return
			}
			if mode == "empty" {
				io.WriteString(w, `{"response":{"game_count":0}}`)
				return
			}
			if mode == "broken" {
				io.WriteString(w, `{"response":{"game_count":2,"games":[]}}`)
				return
			}
			playtime := "10"
			if mode == "achievements-fail" {
				playtime = "20"
			}
			io.WriteString(w, `{"response":{"game_count":2,"games":[{"appid":1,"name":"First","playtime_forever":`+playtime+`,"rtime_last_played":1700000000},{"appid":2,"name":"Unplayed","playtime_forever":0}]}}`)
		case strings.Contains(r.URL.Path, "GetPlayerAchievements"):
			if mode == "achievements-fail" {
				http.Error(w, "temporary failure", 503)
				return
			}
			io.WriteString(w, `{"playerstats":{"success":true,"achievements":[{"achieved":1},{"achieved":0}]}}`)
		case strings.Contains(r.URL.Path, "GetSchemaForGame"):
			http.Error(w, "temporary failure", 503)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
		}
	}))
	defer api.Close()
	s := testStore(t)
	steam := NewSteam("secret-key-never-log", "12345678901234567")
	steam.BaseURL = api.URL
	steam.HTTP = api.Client()
	syncer := &Syncer{Store: s, Steam: steam, SkipCovers: true, Spacing: time.Nanosecond}
	for range 2 {
		if err := syncer.Sync(ctx); err != nil {
			t.Fatal(err)
		}
	}
	games, err := s.Games(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 || *games[0].PlaytimeSeconds != 600 || *games[1].PlaytimeSeconds != 0 || games[1].LastPlayedAt != nil {
		t.Fatalf("wrong library: %+v", games)
	}
	if *games[0].EarnedCount != 1 || *games[0].TotalCount != 2 {
		t.Fatal("achievement counts not saved")
	}
	var setCount int
	s.DB.QueryRow(`SELECT COUNT(*) FROM achievement_sets`).Scan(&setCount)
	if setCount != 2 {
		t.Fatalf("duplicate sets: %d", setCount)
	}
	mode = "achievements-fail"
	if err := syncer.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	games, _ = s.Games(ctx)
	if *games[0].PlaytimeSeconds != 1200 || *games[0].EarnedCount != 1 {
		t.Fatal("partial sync did not preserve previous achievements")
	}
	run, _ := s.LatestRun(ctx, "steam")
	if run.Status != "partial" {
		t.Fatalf("expected partial, got %s", run.Status)
	}
	assertSyncIssue(t, run, "achievement_details", 2)
	for _, m := range []string{"private", "broken"} {
		mode = m
		if err := syncer.Sync(ctx); err == nil {
			t.Fatal("invalid library accepted")
		}
		games, _ = s.Games(ctx)
		if len(games) != 2 || *games[0].PlaytimeSeconds != 1200 {
			t.Fatal("failed sync damaged stored data")
		}
		run, _ = s.LatestRun(ctx, "steam")
		if run.Status != "failed" {
			t.Fatal("failure status missing")
		}
	}
	mode = "empty"
	if err := syncer.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	games, _ = s.Games(ctx)
	if len(games) != 2 {
		t.Fatal("empty response removed games")
	}
	// Persistence is checked by opening another connection to the same on-disk DB.
	var path string
	rows, _ := s.DB.Query(`PRAGMA database_list`)
	for rows.Next() {
		var seq int
		var name string
		rows.Scan(&seq, &name, &path)
	}
	rows.Close()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	stored, err := reopened.Games(ctx)
	if err != nil || len(stored) != 2 {
		t.Fatal("data did not survive reopen")
	}
}

func TestNullAndSharedAchievementSets(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	games := []Game{
		{Platform: "psn", PlatformGameID: "ps4", Name: "Same PS4", SourceScope: "played", PlaytimeSeconds: ptr(0)},
		{Platform: "psn", PlatformGameID: "ps5", Name: "Same PS5", SourceScope: "played", PlaytimeSeconds: ptr(60)},
		{Platform: "epic", PlatformGameID: "ns:item", Name: "Unknown", SourceScope: "owned"},
	}
	if err := s.SaveLibrary(ctx, games); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"ps4", "ps5"} {
		if err := s.SaveAchievements(ctx, "psn", id, "shared-set", 3, 10); err != nil {
			t.Fatal(err)
		}
	}
	stored, _ := s.Games(ctx)
	d := Summarize(stored)
	if d.Metrics.Games != 3 || d.Metrics.PlaytimeKnown != 2 || d.Metrics.PlaytimeSeconds != 60 || d.Metrics.Earned != 3 || d.Metrics.AchievementsKnown != 2 {
		t.Fatalf("wrong coverage or duplicate trophies: %+v", d.Metrics)
	}
	if d.Platforms[1].Earned != 3 {
		t.Fatal("platform trophies counted twice")
	}
	for _, g := range stored {
		if g.Platform == "epic" && (g.PlaytimeSeconds != nil || g.EarnedCount != nil) {
			t.Fatal("unknown turned into zero")
		}
	}
	if err := s.SaveAchievements(ctx, "psn", "ps4", "shared-set", 11, 10); err == nil {
		t.Fatal("invalid progress accepted")
	}
}

func TestReadOnlyAPIAndPagination(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	games := []Game{}
	for i := range 25 {
		games = append(games, Game{Platform: "steam", PlatformGameID: time.Unix(int64(i), 0).Format(time.RFC3339), Name: "Game", SourceScope: "owned", PlaytimeSeconds: ptr(int64(i))})
	}
	if err := s.SaveLibrary(ctx, games); err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: s}).Handler()
	for _, test := range []struct {
		path               string
		count, page, total int
	}{{"/api/games?page=2", 5, 2, 25}, {"/api/games?page=999", 5, 2, 25}, {"/api/games?platform=epic", 0, 1, 0}, {"/api/games?q=missing", 0, 1, 0}, {"/api/games?q=GAME&sort=playtime", 20, 1, 25}} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", test.path, nil))
		var got struct {
			Games       []Game
			Page, Total int
		}
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got.Games) != test.count || got.Page != test.page || got.Total != test.total {
			t.Fatalf("wrong response for %s: %+v", test.path, got)
		}
		if strings.Contains(test.path, "sort=playtime") && *got.Games[0].PlaytimeSeconds != 24 {
			t.Fatal("sort failed")
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/api/games", nil))
	if w.Code != 404 && w.Code != 405 {
		t.Fatal("write route accepted")
	}
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/dashboard", nil))
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var d Dashboard
	if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Metrics.Games != 25 {
		t.Fatal("wrong dashboard total")
	}
}

type failTransport struct{}

func (failTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, &urlError{url: r.URL.String()}
}

type urlError struct{ url string }

func (e *urlError) Error() string { return e.url }
func TestSteamTransportErrorDoesNotLeakCredential(t *testing.T) {
	steam := NewSteam("top-secret-api-key", "12345678901234567")
	steam.HTTP = &http.Client{Transport: failTransport{}}
	_, err := steam.Library(context.Background())
	if err == nil || strings.Contains(err.Error(), "top-secret") || strings.Contains(err.Error(), "?key=") {
		t.Fatalf("unsafe error: %v", err)
	}
}

func TestNoAchievementsIsNotInferredFromFailure(t *testing.T) {
	mode := "schema-missing"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "GetPlayerAchievements") {
			io.WriteString(w, `{"playerstats":{"success":false}}`)
			return
		}
		if mode == "schema-missing" {
			io.WriteString(w, `{"game":{}}`)
		} else {
			io.WriteString(w, `{"game":{"availableGameStats":{"stats":[]}}}`)
		}
	}))
	defer api.Close()
	steam := NewSteam("key", "12345678901234567")
	steam.BaseURL = api.URL
	if _, _, err := steam.Achievements(context.Background(), "1"); err == nil {
		t.Fatal("unknown schema inferred as zero")
	}
	mode = "known-empty"
	earned, total, err := steam.Achievements(context.Background(), "1")
	if err != nil || earned != 0 || total != 0 {
		t.Fatal("known zero not represented")
	}
}
