package shelf

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestAchievementDetailSnapshotJoinAndFailurePreservation(t *testing.T) {
	ctx := context.Background()
	mode := "ok"
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("l") != "schinese" {
			t.Error("missing requested language")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "GetSchemaForGame") {
			w.Write([]byte(`{"game":{"availableGameStats":{"achievements":[{"name":"FIRST","displayName":"初次冒险","description":"完成教程","icon":"https://cdn.steamstatic.com/a.jpg","icongray":"https://cdn.steamstatic.com/b.jpg","hidden":0},{"name":"SECRET","displayName":"秘密","hidden":1}]}}}`))
			return
		}
		switch mode {
		case "ok":
			w.Write([]byte(`{"playerstats":{"success":true,"achievements":[{"apiname":"SECRET","achieved":0,"unlocktime":0},{"apiname":"FIRST","achieved":1,"unlocktime":1700000000}]}}`))
		case "incomplete":
			w.Write([]byte(`{"playerstats":{"success":true,"achievements":[{"apiname":"FIRST","achieved":1}]}}`))
		case "missing-state":
			w.Write([]byte(`{"playerstats":{"success":true,"achievements":[{"apiname":"FIRST"},{"apiname":"SECRET","achieved":0}]}}`))
		case "duplicate":
			w.Write([]byte(`{"playerstats":{"success":true,"achievements":[{"apiname":"FIRST","achieved":1},{"apiname":"FIRST","achieved":0}]}}`))
		default:
			http.Error(w, "unavailable", 503)
		}
	}))
	defer api.Close()
	steam := NewSteam("test-secret", "12345678901234567")
	steam.BaseURL = api.URL
	items, err := steam.AchievementDetails(ctx, "10")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "初次冒险" || !items[0].Achieved || items[0].UnlockedAt == nil || *items[0].UnlockedAt != 1700000000 || !items[1].Hidden || items[1].Achieved || items[1].UnlockedAt != nil {
		t.Fatalf("incorrect join: %+v", items)
	}
	s := testStore(t)
	g := Game{Platform: "steam", PlatformGameID: "10", Name: "Game", SourceScope: "owned"}
	if err = s.SaveLibrary(ctx, []Game{g}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = s.SaveAchievementDetails(ctx, g, items); err != nil {
			t.Fatal(err)
		}
	}
	games, _ := s.Games(ctx)
	id := games[0].ID
	detail, err := s.GameDetail(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if detail.DetailsUpdatedAt == nil || *detail.Game.EarnedCount != 1 || *detail.Game.TotalCount != 2 {
		t.Fatalf("wrong snapshot: %+v", detail)
	}
	for _, broken := range []string{"incomplete", "missing-state", "duplicate", "network"} {
		mode = broken
		if _, err = steam.AchievementDetails(ctx, "10"); err == nil {
			t.Fatalf("accepted %s", broken)
		}
	}
	preserved, _ := s.GameDetail(ctx, id)
	if len(preserved.Achievements) != 2 || !preserved.Achievements[0].Achieved {
		t.Fatal("failed fetch damaged snapshot")
	}
	handler := (&Server{Store: s}).Handler()
	for path, status := range map[string]int{"/api/games/1": 200, "/api/games/999": 404, "/api/games/nope": 404} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
		if recorder.Code != status {
			t.Fatalf("%s: %d", path, recorder.Code)
		}
		if strings.Contains(recorder.Body.String(), "test-secret") {
			t.Fatal("secret exposed")
		}
	}
	// Persisted content is served while Steam is unavailable, including after reopen.
	var dbPath string
	rows, _ := s.DB.Query(`PRAGMA database_list`)
	for rows.Next() {
		var seq int
		var name string
		rows.Scan(&seq, &name, &dbPath)
	}
	rows.Close()
	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.DB.Close()
	stored, err := reopened.GameDetail(ctx, id)
	if err != nil || len(stored.Achievements) != 2 {
		t.Fatal("detail did not survive reopen", err)
	}
}

func TestAchievementDetailsMigrationUnknownAndKnownZero(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "old.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	g := Game{Platform: "steam", PlatformGameID: "10", Name: "Saved game", SourceScope: "owned"}
	if err = s.SaveLibrary(ctx, []Game{g}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`DROP TABLE achievement_details; PRAGMA user_version=1;`); err != nil {
		t.Fatal(err)
	}
	s.DB.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.DB.Close()
	games, _ := s.Games(ctx)
	if len(games) != 1 || games[0].Name != "Saved game" {
		t.Fatal("migration lost library")
	}
	d, err := s.GameDetail(ctx, games[0].ID)
	if err != nil || d.DetailsUpdatedAt != nil || d.Game.EarnedCount != nil || len(d.Achievements) != 0 {
		t.Fatal("unknown was coerced to zero")
	}
	if err = s.SaveAchievementDetails(ctx, g, []Achievement{}); err != nil {
		t.Fatal(err)
	}
	d, _ = s.GameDetail(ctx, games[0].ID)
	if d.DetailsUpdatedAt == nil || d.Game.TotalCount == nil || *d.Game.TotalCount != 0 {
		t.Fatal("known empty not saved")
	}
	body, _ := json.Marshal(d)
	if !strings.Contains(string(body), `"achievements":[]`) {
		t.Fatal("empty collection must not be null")
	}
}
