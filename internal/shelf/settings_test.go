package shelf

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsPersistWithoutReturningKey(t *testing.T) {
	dir := t.TempDir()
	store := testStore(t)
	settings, err := NewSettings(dir, store, "", "")
	if err != nil {
		t.Fatal(err)
	}
	handler := (&Server{Store: store, Settings: settings}).Handler()
	key := strings.Repeat("a", 32)
	save := func(body, origin, host string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/settings/steam", strings.NewReader(body))
		r.Host = host
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-Game-Shelf", "1")
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	body := `{"api_key":"` + key + `","steam_id":"76561198000000000"}`
	for _, test := range []struct{ origin, host string }{{"https://evil.example", "127.0.0.1:8080"}, {"http://evil.example", "evil.example"}} {
		if w := save(body, test.origin, test.host); w.Code != 403 {
			t.Fatal("cross-site configuration accepted")
		}
	}
	w := save(body, "http://127.0.0.1:5173", "127.0.0.1:8080")
	if w.Code != 200 || strings.Contains(w.Body.String(), key) {
		t.Fatalf("unexpected save response %d %s", w.Code, w.Body.String())
	}
	select {
	case <-settings.Wake:
	default:
		t.Fatal("saving did not request a sync")
	}
	info, err := os.Stat(filepath.Join(dir, "settings.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("configuration permissions must be 0600")
	}
	reloaded, err := NewSettings(dir, store, "different-env-key", "76561198000000001")
	if err != nil {
		t.Fatal(err)
	}
	status := reloaded.Status()
	if !status.HasKey || status.SteamID != "76561198000000000" || status.Source != "local" {
		t.Fatalf("config not retained: %+v", status)
	}
	if err := reloaded.Save(context.Background(), "", "76561198000000000"); err != nil {
		t.Fatal("blank key should keep saved key", err)
	}
	steam, err := reloaded.Begin(context.Background())
	if err != nil || steam.Key != key {
		t.Fatal("saved key not used by sync")
	}
	if err := reloaded.Save(context.Background(), key, "76561198000000000"); err == nil {
		t.Fatal("configuration changed mid-sync")
	}
	reloaded.End()
	get := httptest.NewRecorder()
	handler.ServeHTTP(get, httptest.NewRequest("GET", "http://127.0.0.1:8080/api/settings/steam", nil))
	var decoded SettingsStatus
	if err := json.Unmarshal(get.Body.Bytes(), &decoded); err != nil || !decoded.HasKey {
		t.Fatal("status unavailable")
	}
	if strings.Contains(get.Body.String(), key) {
		t.Fatal("GET leaked stored API key")
	}
}

func TestSettingsValidationAndAccountIsolation(t *testing.T) {
	dir := t.TempDir()
	store := testStore(t)
	s, _ := NewSettings(dir, store, "", "")
	ctx := context.Background()
	key := strings.Repeat("b", 32)
	for _, input := range []struct{ key, id string }{{"", "76561198000000000"}, {key, "username"}, {"bad-key", "76561198000000000"}} {
		if s.Save(ctx, input.key, input.id) == nil {
			t.Fatal("invalid credentials accepted")
		}
	}
	if s.Status().Configured {
		t.Fatal("invalid save changed settings")
	}
	if err := s.Save(ctx, key, "76561198000000000"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Begin(ctx); err != nil {
		t.Fatal(err)
	}
	s.End()
	if err := store.SaveLibrary(ctx, []Game{{Platform: "steam", PlatformGameID: "1", Name: "Game", SourceScope: "owned"}}); err != nil {
		t.Fatal(err)
	}
	if s.Save(ctx, key, "76561198000000001") == nil {
		t.Fatal("mixed accounts allowed")
	}
	if s.Status().SteamID != "76561198000000000" {
		t.Fatal("failed save damaged previous config")
	}
	if err := s.Save(ctx, strings.Repeat("c", 32), "76561198000000000"); err != nil {
		t.Fatal("key rotation should be allowed", err)
	}
}
