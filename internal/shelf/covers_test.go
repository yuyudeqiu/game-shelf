package shelf

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type coverTransport func(*http.Request) (*http.Response, error)

func (f coverTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCoverRepairResolvesHashedStoreImageAndRetainsURL(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	dir := t.TempDir()
	guessed := "https://shared.fastly.steamstatic.com/store_item_assets/steam/apps/3472040/header.jpg"
	resolved := "https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/3472040/content-hash/header.jpg?t=123"
	game := Game{Platform: "steam", PlatformGameID: "3472040", Name: "NBA 2K26", CoverURL: &guessed, SourceScope: "owned", PlaytimeSeconds: ptr(100)}
	if err := store.SaveLibrary(ctx, []Game{game}); err != nil {
		t.Fatal(err)
	}
	metadataCalls := 0
	steam := NewSteam("must-not-send-this-key", "")
	steam.HTTP = &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
		if strings.Contains(r.URL.String(), "must-not-send") || r.URL.Query().Has("key") {
			t.Fatal("public artwork request contains credentials")
		}
		status, kind, body := 404, "text/plain", "missing"
		switch r.URL.String() {
		case resolved:
			status, kind, body = 200, "image/jpeg", "example-image-bytes"
		default:
			if r.URL.Host == "store.steampowered.com" {
				metadataCalls++
				if r.URL.Query().Get("appids") != "3472040" {
					t.Fatal("wrong title lookup")
				}
				status, kind, body = 200, "application/json", `{"3472040":{"success":true,"data":{"header_image":"`+resolved+`"}}}`
			}
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {kind}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
	})}
	syncer := &Syncer{Store: store, Steam: steam, DataDir: dir}
	repaired, failed, err := syncer.RepairCovers(ctx)
	if err != nil || repaired != 1 || failed != 0 {
		t.Fatalf("repair failed: %d %d %v", repaired, failed, err)
	}
	games, _ := store.Games(ctx)
	if games[0].CoverPath == nil || *games[0].CoverURL != resolved || *games[0].PlaytimeSeconds != 100 {
		t.Fatal("artwork repair damaged game or failed to persist URL")
	}
	body, err := os.ReadFile(filepath.Join(dir, "covers", *games[0].CoverPath))
	if err != nil || string(body) != "example-image-bytes" {
		t.Fatal("image not cached")
	}
	if err := store.SaveLibrary(ctx, []Game{game}); err != nil {
		t.Fatal(err)
	}
	games, _ = store.Games(ctx)
	if *games[0].CoverURL != resolved {
		t.Fatal("next library sync overwrote resolved artwork URL")
	}
	repaired, failed, err = syncer.RepairCovers(ctx)
	if err != nil || repaired != 0 || failed != 0 || metadataCalls != 1 {
		t.Fatal("already cached artwork requested again")
	}
	// A missing cache file is retried using the resolved source, without another metadata query.
	os.Remove(filepath.Join(dir, "covers", *games[0].CoverPath))
	repaired, failed, err = syncer.RepairCovers(ctx)
	if err != nil || repaired != 1 || failed != 0 || metadataCalls != 1 {
		t.Fatal("cache loss recovery failed")
	}
}

func TestCoverRepairUnavailableMetadataLeavesGameUnchanged(t *testing.T) {
	for _, metadata := range []string{`{"1":{"success":false}}`, `{"1":{"success":true,"data":{"header_image":"https://untrusted.example/header.jpg"}}}`} {
		t.Run(metadata, func(t *testing.T) {
			store := testStore(t)
			ctx := context.Background()
			url := "https://shared.fastly.steamstatic.com/store_item_assets/steam/apps/1/header.jpg"
			store.SaveLibrary(ctx, []Game{{Platform: "steam", PlatformGameID: "1", Name: "Game", SourceScope: "owned", CoverURL: &url}})
			steam := NewSteam("", "")
			steam.HTTP = &http.Client{Transport: coverTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.Host == "untrusted.example" {
					t.Fatal("untrusted metadata URL fetched")
				}
				status, body := 404, "missing"
				if r.URL.Host == "store.steampowered.com" {
					status, body = 200, metadata
				}
				return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}
			repaired, failed, err := (&Syncer{Store: store, Steam: steam, DataDir: t.TempDir()}).RepairCovers(ctx)
			if err != nil || repaired != 0 || failed != 1 {
				t.Fatal("unavailable artwork should be reported")
			}
			games, _ := store.Games(ctx)
			if games[0].CoverPath != nil || *games[0].CoverURL != url {
				t.Fatal("failed repair changed stored artwork")
			}
		})
	}
}
