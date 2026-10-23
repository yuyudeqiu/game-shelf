package shelf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func categoryPtr(value string) *string { return &value }

func TestPSNMediaClassificationCompleteSyncAndReadViews(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	dir := t.TempDir()
	entries := []psnPlayed{
		{ID: "PPSA06274_00", Name: "bilibili", Category: "ps5_web_based_media_app", Duration: "PT2H"},
		{ID: "PPSA03622_00", Name: "Apple Music", Category: "ps5_native_media_app", Duration: "PT1H"},
		{ID: "PPSA02584_00", Name: "原神", Category: "ps5_native_game", Duration: "PT0S"},
		{ID: "PPSA02585_00", Name: "原神", Category: "ps5_native_game", Duration: "PT1H"},
		{ID: "UNKNOWN", Name: "Unknown category", Category: "future_category"},
		{ID: "MISSING", Name: "Missing category"},
		{ID: "NAME_ONLY", Name: "bilibili", Category: "ps4_game"},
	}
	// Old rows have no classification. Include a separate platform with the same
	// name and media-looking category to verify PSN-only filtering.
	seed := []Game{{Platform: "steam", PlatformGameID: "1", Name: "bilibili", SourceCategory: categoryPtr("ps5_native_media_app"), SourceScope: "owned"}}
	for _, e := range entries {
		seed = append(seed, Game{Platform: "psn", PlatformGameID: e.ID, Name: e.Name, SourceScope: "played", PlaytimeSeconds: ptr(int64(10)), LastPlayedAt: ptr(int64(100))})
	}
	if err := store.SaveLibrary(ctx, seed); err != nil {
		t.Fatal(err)
	}
	var appID int64
	if err := store.DB.QueryRow(`SELECT id FROM games WHERE platform='psn' AND platform_game_id='PPSA06274_00'`).Scan(&appID); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveAchievementDetails(ctx, seed[1], []Achievement{{APIName: "0", Name: "Retained", Achieved: true}}); err != nil {
		t.Fatal(err)
	}
	mode := "failed-page"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/gamelist/v2/users/me/titles":
			if r.URL.Query().Get("offset") == "0" {
				_ = json.NewEncoder(w).Encode(map[string]any{"titles": entries[:2], "totalItemCount": len(entries), "nextOffset": 2})
			} else if mode == "failed-page" {
				w.WriteHeader(503)
			} else if mode == "duplicate" {
				_ = json.NewEncoder(w).Encode(map[string]any{"titles": append([]psnPlayed{entries[0]}, entries[3:]...), "totalItemCount": len(entries)})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"titles": entries[2:], "totalItemCount": len(entries)})
			}
		case "/trophy/v1/users/me/titles/trophyTitles":
			ids := r.URL.Query().Get("npTitleIds")
			if strings.Contains(ids, "PPSA06274_00") || strings.Contains(ids, "PPSA03622_00") {
				t.Error("media app sent for trophy enrichment")
			}
			_, _ = w.Write([]byte(`{"titles":[]}`))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	p := NewPSN("fixture")
	p.APIBase, p.HTTP, p.Spacing = server.URL, server.Client(), 0
	for _, failure := range []string{"failed-page", "duplicate"} {
		mode = failure
		if _, _, err := syncPSN(ctx, p, store, dir, false); err == nil {
			t.Fatalf("accepted %s", mode)
		}
		games, err := store.Games(ctx)
		if err != nil || len(games) != len(seed) {
			t.Fatal("failed list hid previous entries", err)
		}
	}
	mode = "complete"
	for range 2 {
		count, _, err := syncPSN(ctx, p, store, dir, false)
		if err != nil || count != 5 {
			t.Fatal("wrong visible sync count", count, err)
		}
	}
	var rows int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM games`).Scan(&rows); err != nil || rows != 8 {
		t.Fatal("rows deleted or duplicated", rows, err)
	}
	var savedCategory string
	if err := store.DB.QueryRow(`SELECT source_category FROM games WHERE id=?`, appID).Scan(&savedCategory); err != nil || savedCategory != "ps5_web_based_media_app" {
		t.Fatal("classification not persisted", err)
	}
	var snapshots int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM achievement_details WHERE game_id=?`, appID).Scan(&snapshots); err != nil || snapshots != 1 {
		t.Fatal("retained snapshot deleted", err)
	}
	handler := (&Server{Store: store}).Handler()
	for _, path := range []string{"/api/dashboard", "/api/games?platform=psn", "/api/games?q=Apple", fmt.Sprintf("/api/games/%d", appID)} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if strings.Contains(path, fmt.Sprintf("/%d", appID)) {
			if w.Code != 404 {
				t.Fatal("hidden application detail is accessible")
			}
			continue
		}
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		switch path {
		case "/api/dashboard":
			var d Dashboard
			if err := json.Unmarshal(w.Body.Bytes(), &d); err != nil {
				t.Fatal(err)
			}
			if d.Metrics.Games != 6 || d.Metrics.PlaytimeKnown != 5 || d.Metrics.PlaytimeSeconds != 3630 || d.Metrics.Earned != 0 || d.Metrics.AchievementsKnown != 0 {
				t.Fatalf("apps affected metrics: %+v", d.Metrics)
			}
			if d.Platforms[1].Count != 5 {
				t.Fatal("wrong PSN count")
			}
			for _, list := range [][]Game{d.Recent, d.TopPlayed} {
				for _, g := range list {
					if g.ID == appID || g.Name == "Apple Music" {
						t.Fatal("application in chart/recent")
					}
				}
			}
		default:
			var list struct {
				Games []Game
				Total int
			}
			if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
				t.Fatal(err)
			}
			if path == "/api/games?platform=psn" && list.Total != 5 {
				t.Fatal("wrong library count")
			}
			if path == "/api/games?q=Apple" && list.Total != 0 {
				t.Fatal("application searchable")
			}
		}
	}
	// Missing/empty categories and absent titles must not undo a confirmed exclusion.
	if err := store.SaveLibrary(ctx, []Game{{Platform: "psn", PlatformGameID: entries[0].ID, Name: "bilibili", SourceScope: "played", SourceCategory: categoryPtr("")}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLibrary(ctx, nil); err != nil {
		t.Fatal(err)
	}
	games, err := store.Games(ctx)
	if err != nil || len(games) != 6 {
		t.Fatal("missing classification changed visibility", err)
	}
	settings, err := NewPSNSettings(dir, store)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.bind(ctx, "123"); err == nil {
		t.Fatal("retained PSN rows must prevent account switching")
	}
	// A later explicit game classification restores the retained row and snapshot.
	if err := store.SaveLibrary(ctx, []Game{{Platform: "psn", PlatformGameID: entries[0].ID, Name: "Reclassified", SourceScope: "played", SourceCategory: categoryPtr("ps5_native_game")}}); err != nil {
		t.Fatal(err)
	}
	detail, err := store.GameDetail(ctx, appID)
	if err != nil || len(detail.Achievements) != 1 {
		t.Fatal("reclassification lost original snapshot", err)
	}
}

func TestPSNCategoryMigrationAndPersistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "v3.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	g := Game{Platform: "psn", PlatformGameID: "PPSA1", Name: "Preserved", SourceScope: "played", PlaytimeSeconds: ptr(int64(0))}
	if err := s.SaveLibrary(ctx, []Game{g}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveAchievementDetails(ctx, g, []Achievement{{APIName: "0", Name: "Preserved"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`ALTER TABLE games DROP COLUMN source_category; ALTER TABLE sync_runs DROP COLUMN report_json; PRAGMA user_version=3;`); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	games, err := s.Games(ctx)
	if err != nil || len(games) != 1 || games[0].SourceCategory != nil || games[0].PlaytimeSeconds == nil || *games[0].PlaytimeSeconds != 0 || games[0].LastPlayedAt != nil {
		t.Fatal("migration damaged unknown/zero semantics", err)
	}
	detail, err := s.GameDetail(ctx, games[0].ID)
	if err != nil || len(detail.Achievements) != 1 {
		t.Fatal("migration lost snapshot", err)
	}
	g.SourceCategory = categoryPtr("ps5_native_media_app")
	if err := s.SaveLibrary(ctx, []Game{g}); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	games, err = s.Games(ctx)
	if err != nil || len(games) != 0 {
		t.Fatal("classification lost after restart", err)
	}
	settings, err := NewPSNSettings(t.TempDir(), s)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.bind(ctx, "999"); err == nil {
		t.Fatal("app-only account allowed switching")
	}
}
