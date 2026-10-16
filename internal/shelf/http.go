package shelf

import (
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Metrics struct {
	Games             int   `json:"games"`
	PlaytimeSeconds   int64 `json:"playtime_seconds"`
	PlaytimeKnown     int   `json:"playtime_known"`
	Earned            int64 `json:"earned"`
	AchievementsKnown int   `json:"achievements_known"`
}
type PlatformStatus struct {
	Platform        string   `json:"platform"`
	Configured      bool     `json:"configured"`
	Supported       bool     `json:"supported"`
	Scope           string   `json:"scope"`
	Count           int      `json:"count"`
	Earned          int64    `json:"earned"`
	Run             *SyncRun `json:"run"`
	LastLibrarySync *int64   `json:"last_library_sync"`
}
type Dashboard struct {
	Metrics   Metrics          `json:"metrics"`
	Platforms []PlatformStatus `json:"platforms"`
	Recent    []Game           `json:"recent"`
	TopPlayed []Game           `json:"top_played"`
}

func Summarize(games []Game) Dashboard {
	d := Dashboard{Recent: []Game{}, TopPlayed: []Game{}, Platforms: []PlatformStatus{
		{Platform: "steam", Supported: true, Scope: "owned"},
		{Platform: "psn", Supported: true, Scope: "played"}, {Platform: "epic", Scope: "owned"},
	}}
	seen := map[int64]bool{}
	for _, g := range games {
		d.Metrics.Games++
		if g.PlaytimeSeconds != nil {
			d.Metrics.PlaytimeKnown++
			d.Metrics.PlaytimeSeconds += *g.PlaytimeSeconds
			if *g.PlaytimeSeconds > 0 {
				d.TopPlayed = append(d.TopPlayed, g)
			}
		}
		if g.EarnedCount != nil {
			d.Metrics.AchievementsKnown++
		}
		newSet := g.AchievementSetID != nil && !seen[*g.AchievementSetID]
		if newSet && g.EarnedCount != nil {
			d.Metrics.Earned += *g.EarnedCount
			seen[*g.AchievementSetID] = true
		}
		if g.LastPlayedAt != nil && *g.LastPlayedAt > 0 {
			d.Recent = append(d.Recent, g)
		}
		for i := range d.Platforms {
			if d.Platforms[i].Platform == g.Platform {
				d.Platforms[i].Count++
				if newSet && g.EarnedCount != nil {
					d.Platforms[i].Earned += *g.EarnedCount
				}
			}
		}
	}
	sort.SliceStable(d.Recent, func(i, j int) bool { return timestamp(d.Recent[i].LastPlayedAt) > timestamp(d.Recent[j].LastPlayedAt) })
	if len(d.Recent) > 6 {
		d.Recent = d.Recent[:6]
	}
	sort.SliceStable(d.TopPlayed, func(i, j int) bool { return *d.TopPlayed[i].PlaytimeSeconds > *d.TopPlayed[j].PlaytimeSeconds })
	if len(d.TopPlayed) > 5 {
		d.TopPlayed = d.TopPlayed[:5]
	}
	return d
}

type Server struct {
	Store      *Store
	Configured bool
	Settings   *Settings
	PSN        *PSNSettings
	DataDir    string
	WebDir     string
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/settings/psn", func(w http.ResponseWriter, r *http.Request) {
		if s.PSN == nil {
			writeJSON(w, PSNStatus{})
			return
		}
		writeJSON(w, s.PSN.Status())
	})
	mux.HandleFunc("POST /api/settings/psn", func(w http.ResponseWriter, r *http.Request) {
		if !localWrite(r) {
			jsonError(w, 403, "仅允许从本机书架页面保存配置")
			return
		}
		if s.PSN == nil {
			apiError(w)
			return
		}
		var input struct {
			NPSSO string `json:"npsso"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			jsonError(w, 400, "配置格式不正确")
			return
		}
		if err := s.PSN.Save(r.Context(), input.NPSSO); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		s.PSN.RequestSync()
		writeJSON(w, s.PSN.Status())
	})
	mux.HandleFunc("POST /api/sync/psn", func(w http.ResponseWriter, r *http.Request) {
		if !localWrite(r) {
			jsonError(w, 403, "仅允许从本机书架页面同步")
			return
		}
		if s.PSN == nil || !s.PSN.Status().Configured {
			jsonError(w, 400, "请先连接 PSN")
			return
		}
		if s.PSN.Status().Busy {
			jsonError(w, 409, "PSN 正在同步，请稍候")
			return
		}
		s.PSN.RequestSync()
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /api/settings/steam", func(w http.ResponseWriter, r *http.Request) {
		if s.Settings == nil {
			apiError(w)
			return
		}
		writeJSON(w, s.Settings.Status())
	})
	mux.HandleFunc("POST /api/settings/steam", func(w http.ResponseWriter, r *http.Request) {
		if !localWrite(r) {
			jsonError(w, 403, "仅允许从本机书架页面保存配置")
			return
		}
		if s.Settings == nil {
			apiError(w)
			return
		}
		var input struct {
			Key string `json:"api_key"`
			ID  string `json:"steam_id"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			jsonError(w, 400, "配置格式不正确")
			return
		}
		if err := s.Settings.Save(r.Context(), input.Key, input.ID); err != nil {
			jsonError(w, 400, err.Error())
			return
		}
		s.Settings.RequestSync()
		writeJSON(w, s.Settings.Status())
	})
	mux.HandleFunc("POST /api/sync/steam", func(w http.ResponseWriter, r *http.Request) {
		if !localWrite(r) {
			jsonError(w, 403, "仅允许从本机书架页面同步")
			return
		}
		if s.Settings == nil || !s.Settings.Status().Configured {
			jsonError(w, 400, "请先配置 Steam")
			return
		}
		if s.Settings.Status().Busy {
			jsonError(w, 409, "正在同步，请稍候")
			return
		}
		s.Settings.RequestSync()
		w.WriteHeader(http.StatusAccepted)
	})
	mux.HandleFunc("GET /api/dashboard", func(w http.ResponseWriter, r *http.Request) {
		games, err := s.Store.Games(r.Context())
		if err != nil {
			apiError(w)
			return
		}
		d := Summarize(games)
		d.Platforms[0].Configured = s.Configured
		if s.Settings != nil {
			d.Platforms[0].Configured = s.Settings.Status().Configured
		}
		if s.PSN != nil {
			d.Platforms[1].Configured = s.PSN.Status().Configured
		}
		for i := range d.Platforms {
			d.Platforms[i].Run, err = s.Store.LatestRun(r.Context(), d.Platforms[i].Platform)
			if err != nil {
				apiError(w)
				return
			}
			d.Platforms[i].LastLibrarySync, err = s.Store.LastLibrarySync(r.Context(), d.Platforms[i].Platform)
			if err != nil {
				apiError(w)
				return
			}
		}
		writeJSON(w, d)
	})
	mux.HandleFunc("GET /api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id < 1 {
			jsonError(w, 404, "游戏不存在")
			return
		}
		detail, err := s.Store.GameDetail(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			jsonError(w, 404, "游戏不存在")
			return
		}
		if err != nil {
			apiError(w)
			return
		}
		writeJSON(w, detail)
	})
	mux.HandleFunc("GET /api/games", func(w http.ResponseWriter, r *http.Request) {
		games, err := s.Store.Games(r.Context())
		if err != nil {
			apiError(w)
			return
		}
		q, platform := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))), r.URL.Query().Get("platform")
		filtered := []Game{}
		for _, g := range games {
			if (q == "" || strings.Contains(strings.ToLower(g.Name), q)) && (platform == "" || platform == "all" || g.Platform == platform) {
				filtered = append(filtered, g)
			}
		}
		switch r.URL.Query().Get("sort") {
		case "name":
			sort.SliceStable(filtered, func(i, j int) bool { return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name) })
		case "playtime":
			sort.SliceStable(filtered, func(i, j int) bool {
				a, b := filtered[i].PlaytimeSeconds, filtered[j].PlaytimeSeconds
				return a != nil && (b == nil || *a > *b)
			})
		case "achievements":
			sort.SliceStable(filtered, func(i, j int) bool { a, b := progress(filtered[i]), progress(filtered[j]); return a > b })
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		const size = 20
		total := len(filtered)
		pages := max(1, int(math.Ceil(float64(total)/size)))
		if page > pages {
			page = pages
		}
		start := min((page-1)*size, total)
		end := min(start+size, total)
		writeJSON(w, map[string]any{"games": filtered[start:end], "total": total, "page": page, "pages": pages, "page_size": size})
	})
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "Not found", http.StatusNotFound) })
	mux.Handle("GET /covers/", http.StripPrefix("/covers/", http.FileServer(http.Dir(filepath.Join(s.DataDir, "covers")))))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if _, err := os.Stat(filepath.Join(s.WebDir, "index.html")); err != nil {
				http.Error(w, "Frontend not built. Run: npm --prefix web run build", http.StatusServiceUnavailable)
				return
			}
		}
		http.FileServer(http.Dir(s.WebDir)).ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}

func progress(g Game) float64 {
	if g.EarnedCount == nil || g.TotalCount == nil || *g.TotalCount == 0 {
		return -1
	}
	return float64(*g.EarnedCount) / float64(*g.TotalCount)
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	writeJSON(w, map[string]string{"error": "Local data could not be read. Check the backend terminal."})
}

func jsonError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	writeJSON(w, map[string]string{"error": message})
}
func loopbackHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	return h == "localhost" || net.ParseIP(h) != nil && net.ParseIP(h).IsLoopback()
}
func localWrite(r *http.Request) bool {
	if !loopbackHost(r.Host) || r.Header.Get("X-Game-Shelf") != "1" {
		return false
	}
	typeName, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || typeName != "application/json" {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "http" || !loopbackHost(u.Host) {
			return false
		}
	}
	return true
}
