package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"game-shelf/internal/shelf"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "serve" && command != "sync" && command != "covers" && command != "sync-psn" {
		return errors.New("usage: go run ./cmd/gameshelf [serve|sync|sync-psn|covers] [-env .env] [-web web/dist]")
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	envPath := flags.String("env", ".env", "local environment file")
	webDir := flags.String("web", "web/dist", "built frontend directory")
	refreshDetails := flags.Bool("refresh-details", false, "refresh all cached PSN trophy text (sync-psn only)")
	args := []string{}
	if len(os.Args) > 2 {
		args = os.Args[2:]
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *refreshDetails && command != "sync-psn" {
		return errors.New("-refresh-details is only supported with sync-psn")
	}
	if err := loadEnv(*envPath); err != nil {
		return err
	}
	dataDir := env("DATA_DIR", "./data")
	key, id := strings.TrimSpace(os.Getenv("STEAM_API_KEY")), strings.TrimSpace(os.Getenv("STEAM_ID"))
	interval, err := time.ParseDuration(env("SYNC_INTERVAL", "6h"))
	if err != nil || interval < time.Minute {
		return errors.New("SYNC_INTERVAL must be a duration of at least 1m, e.g. 6h")
	}
	store, err := shelf.Open(filepath.Join(dataDir, "gameshelf.db"))
	if err != nil {
		return errors.New("could not open local database")
	}
	defer store.DB.Close()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if command == "covers" {
		repaired, failed, err := (&shelf.Syncer{Store: store, Steam: shelf.NewSteam("", ""), DataDir: dataDir}).RepairCovers(ctx)
		if err != nil {
			return errors.New("cover repair interrupted or local data unavailable")
		}
		log.Printf("Cover repair: %d repaired, %d unavailable", repaired, failed)
		return nil
	}
	settings, err := shelf.NewSettings(dataDir, store, key, id)
	if err != nil {
		return err
	}
	psnSettings, err := shelf.NewPSNSettings(dataDir, store)
	if err != nil {
		return err
	}
	psnSync := func() error {
		log.Print("PSN sync started")
		var syncErr error
		if *refreshDetails {
			syncErr = psnSettings.RefreshDetails(ctx)
		} else {
			syncErr = psnSettings.Sync(ctx)
		}
		if syncErr != nil {
			return syncErr
		}
		run, err := store.LatestRun(ctx, "psn")
		if err != nil {
			return errors.New("could not read PSN sync status")
		}
		if run != nil {
			log.Printf("PSN sync %s: %d games", run.Status, run.ItemsSynced)
			if run.ErrorSummary != nil {
				log.Print(*run.ErrorSummary)
			}
		}
		return nil
	}
	if command == "sync-psn" {
		return psnSync()
	}
	syncOnce := func() error {
		steam, err := settings.Begin(ctx)
		if err != nil {
			return err
		}
		defer settings.End()
		syncer := &shelf.Syncer{Store: store, Steam: steam, DataDir: dataDir}
		log.Print("Steam sync started")
		if err := syncer.Sync(ctx); err != nil {
			return err
		}
		run, err := store.LatestRun(ctx, "steam")
		if err != nil {
			return errors.New("could not read sync status")
		}
		if run != nil {
			log.Printf("Steam sync %s: %d games", run.Status, run.ItemsSynced)
			if run.ErrorSummary != nil {
				log.Print(*run.ErrorSummary)
			}
		}
		return nil
	}
	if command == "sync" {
		return syncOnce()
	}
	addr := env("LISTEN_ADDR", "127.0.0.1:8080")
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("LISTEN_ADDR must be a loopback host:port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("LISTEN_ADDR must use a loopback IP, e.g. 127.0.0.1:8080")
	}
	server := &http.Server{Addr: addr, Handler: (&shelf.Server{Store: store, Settings: settings, PSN: psnSettings, DataDir: dataDir, WebDir: *webDir}).Handler(), ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return errors.New("local address unavailable; check LISTEN_ADDR or an already running server")
	}
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		runSyncWorker(ctx, "steam", store, interval, func() bool { return settings.Status().Configured }, settings.Wake, syncOnce)
	}()
	go func() {
		defer workers.Done()
		runSyncWorker(ctx, "psn", store, interval, func() bool { return psnSettings.Status().Configured }, psnSettings.Wake, psnSync)
	}()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Printf("Game Shelf ready at http://%s", addr)
	err = server.Serve(listener)
	stop()
	workers.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// One small worker per platform keeps a failed PSN login from delaying Steam.
func runSyncWorker(ctx context.Context, platform string, store *shelf.Store, interval time.Duration, configured func() bool, wake <-chan struct{}, syncOnce func() error) {
	var lastAttempt time.Time
	run := func() {
		lastAttempt = time.Now()
		if err := syncOnce(); err != nil && ctx.Err() == nil {
			log.Print(err)
		}
	}
	refresh := func() {
		if !configured() || !lastAttempt.IsZero() && time.Since(lastAttempt) < min(interval, 15*time.Minute) {
			return
		}
		last, err := store.LastLibrarySync(ctx, platform)
		if err != nil {
			log.Print("Could not read last sync time")
			return
		}
		if last == nil || time.Since(time.Unix(*last, 0)) >= interval {
			run()
		}
	}
	refresh()
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
			if configured() {
				run()
			}
		case <-ticker.C:
			refresh()
		}
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// Parse simple dotenv assignments as data, never execute a shell or expand variables.
func loadEnv(path string) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return errors.New("could not open local environment file")
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		key, value, ok := strings.Cut(text, "=")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || !regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`).MatchString(key) {
			return fmt.Errorf("invalid .env assignment at line %d", line)
		}
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
	if scanner.Err() != nil {
		return errors.New("could not read local environment file")
	}
	return nil
}
