package shelf

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func assertSyncIssue(t *testing.T, run *SyncRun, code string, count int) {
	t.Helper()
	if run == nil || run.Report == nil || !run.Report.LibrarySaved {
		t.Fatal("missing saved-library report")
	}
	for _, issue := range run.Report.Issues {
		if issue.Code == code && issue.Count == count {
			return
		}
	}
	t.Fatalf("missing issue %s/%d: %+v", code, count, run.Report.Issues)
}

func TestSyncReportMigrationAndPersistence(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "report.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate v4 with a historical result and an existing game.
	id, err := store.StartRun(ctx, "steam")
	if err != nil {
		t.Fatal(err)
	}
	summary := "legacy summary"
	if err := store.FinishRun(ctx, id, "partial", 1, &summary); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLibrary(ctx, []Game{{Platform: "steam", PlatformGameID: "1", Name: "Fixture", SourceScope: "owned"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`ALTER TABLE sync_runs DROP COLUMN report_json; PRAGMA user_version=4;`); err != nil {
		t.Fatal(err)
	}
	store.DB.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	old, err := store.LatestRun(ctx, "steam")
	if err != nil || old.Report != nil || old.ErrorSummary == nil || *old.ErrorSummary != summary {
		t.Fatalf("legacy report changed: %+v / %v", old, err)
	}
	games, err := store.Games(ctx)
	if err != nil || len(games) != 1 || games[0].PlaytimeSeconds != nil {
		t.Fatal("migration lost game or NULL")
	}
	report := &SyncReport{LibrarySaved: true, Issues: []SyncIssue{{Code: "covers", Count: 3}}}
	id, err = store.StartRun(ctx, "steam")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishRun(ctx, id, "partial", 1, nil, report); err != nil {
		t.Fatal(err)
	}
	store.DB.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.DB.Close()
	run, err := store.LatestRun(ctx, "steam")
	if err != nil || !reflect.DeepEqual(run.Report, report) {
		t.Fatalf("report did not survive reopening: %+v / %v", run, err)
	}
}

func TestLibrarySyncTimestampUsesSavedOutcome(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	id, _ := store.StartRun(ctx, "epic")
	if err := store.FinishRun(ctx, id, "partial", 1, nil, &SyncReport{LibrarySaved: true, Issues: []SyncIssue{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`UPDATE sync_runs SET finished_at=100 WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	// Even a newer result must not advance freshness without a saved list.
	id, _ = store.StartRun(ctx, "epic")
	if err := store.FinishRun(ctx, id, "partial", 0, nil, &SyncReport{Issues: []SyncIssue{}}); err != nil {
		t.Fatal(err)
	}
	at, err := store.LastLibrarySync(ctx, "epic")
	if err != nil || at == nil || *at != 100 {
		t.Fatalf("unsaved list advanced freshness: %v %v", at, err)
	}
	id, _ = store.StartRun(ctx, "epic")
	if err := store.FinishRun(ctx, id, "failed", 0, nil, &SyncReport{Issues: []SyncIssue{}}); err != nil {
		t.Fatal(err)
	}
	at, err = store.LastLibrarySync(ctx, "epic")
	if err != nil || at == nil || *at != 100 {
		t.Fatal("failed result erased freshness")
	}
}

// Cancellation after the library commit must retain both the saved result and
// issues already observed, even though the caller's context can no longer write.
func TestInterruptedSyncRetainsStructuredOutcome(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "GetOwnedGames") {
			io.WriteString(w, `{"response":{"game_count":2,"games":[{"appid":1,"name":"Fixture A"},{"appid":2,"name":"Fixture B"}]}}`)
			return
		}
		cancel()
		http.Error(w, "fixture interrupted", http.StatusServiceUnavailable)
	}))
	defer api.Close()
	store := testStore(t)
	steam := NewSteam("fixture-key", "fixture-id")
	steam.BaseURL = api.URL
	syncer := &Syncer{Store: store, Steam: steam, SkipCovers: true, Spacing: time.Microsecond}
	if err := syncer.Sync(ctx); err == nil {
		t.Fatal("expected interruption")
	}
	run, err := store.LatestRun(context.Background(), "steam")
	if err != nil || run.Status != "partial" {
		t.Fatalf("interruption lost result: %+v %v", run, err)
	}
	assertSyncIssue(t, run, "incomplete", 1)
	assertSyncIssue(t, run, "achievement_details", 1)
	games, err := store.Games(context.Background())
	if err != nil || len(games) != 2 || games[0].PlaytimeSeconds != nil {
		t.Fatal("interruption lost saved list or NULL")
	}
}
