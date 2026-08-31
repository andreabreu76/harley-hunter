package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/browser"
	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/crawl"
	"github.com/andreabreu76/harley-hunter/internal/fipe"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/notify"
	"github.com/andreabreu76/harley-hunter/internal/paths"
	"github.com/andreabreu76/harley-hunter/internal/schedule"
	"github.com/andreabreu76/harley-hunter/internal/source"
	"github.com/andreabreu76/harley-hunter/internal/source/meta"
	"github.com/andreabreu76/harley-hunter/internal/store"
	"github.com/andreabreu76/harley-hunter/internal/web"
)

const dashboardAddr = "127.0.0.1:8080"

const shutdownGrace = 5 * time.Second

func main() {
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	dir, err := paths.AppDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	path := *configPath
	if path == "" {
		path = paths.ConfigFile(dir)
	}
	if err := config.EnsureFile(path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	var runErr error
	switch flag.Arg(0) {
	case "crawl":
		runErr = runCrawl(path, dir)
	case "serve":
		runErr = runServe(path, dir)
	case "repair-silenced":
		runErr = runRepairSilenced(path)
	case "paths":
		runErr = runPaths(dir, path)
	default:
		fmt.Fprintln(os.Stderr, "usage: hunter [-config path] <serve|crawl|repair-silenced|paths>")
		os.Exit(2)
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(1)
	}
}

func runPaths(dir, configPath string) error {
	fmt.Printf("directory: %s\nconfig:    %s\ndatabase:  %s\nprofile:   %s\nlog:       %s\n",
		dir, configPath, paths.DatabaseFile(dir), paths.ChromeProfile(dir), paths.LogFile(dir))
	return nil
}

func runServe(configPath, dir string) error {
	stopTee, err := teeOutput(paths.LogFile(dir))
	if err != nil {
		return err
	}
	defer stopTee()

	watcher, err := config.NewWatcher(configPath)
	if err != nil {
		return err
	}
	db, err := store.Open(watcher.Current().DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runner := &schedule.Runner{
		Config:  watcher,
		LastRun: db.LastRunStartedAt,
		Collect: func(cfg config.Config) error { return collect(ctx, cfg, db, dir) },
		Now:     time.Now,
		Warn:    os.Stderr,
	}
	scheduled := make(chan struct{})
	go func() {
		defer close(scheduled)
		if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Fprintf(os.Stderr, "the scheduler stopped: %v\n", err)
		}
	}()

	srv := &http.Server{
		Addr:    dashboardAddr,
		Handler: web.NewServer(db, func() []string { return watcher.Current().Sources }),
	}
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		<-ctx.Done()
		grace, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := srv.Shutdown(grace); err != nil {
			fmt.Fprintf(os.Stderr, "the dashboard did not close cleanly: %v\n", err)
		}
	}()

	fmt.Printf("config:    %s\n", watcher.Path())

	var serveErr error
	listener, err := net.Listen("tcp", dashboardAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "the dashboard has no address to answer on and stays down, the hunt goes on without it: %v\n", err)
		<-ctx.Done()
	} else {
		fmt.Printf("dashboard: http://%s\n", dashboardAddr)
		serveErr = srv.Serve(listener)
	}

	stop()
	<-closed
	select {
	case <-scheduled:
	case <-time.After(shutdownGrace):
		fmt.Fprintln(os.Stderr, "the round in flight did not stop in time; the browser goes down with the process")
	}

	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}

func runCrawl(configPath, dir string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("nothing to hunt for: %w; edit %s", err, configPath)
	}
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return collect(ctx, cfg, db, dir)
}

func collect(ctx context.Context, cfg config.Config, db *store.Store, dir string) error {
	handle, err := openBrowser(ctx, cfg, dir)
	if err != nil {
		return err
	}
	defer func() {
		if err := handle.Close(); err != nil && ctx.Err() == nil {
			fmt.Fprintf(os.Stderr, "the browser was left running: %v\n", err)
		}
	}()

	fetcher := source.NewBrowserFetcher(handle.DevtoolsURL())
	defer releaseTabs(fetcher)

	sources, err := buildSources(cfg, fetcher)
	if err != nil {
		return err
	}

	started := time.Now()
	report, err := crawl.Run(ctx, sources, db, cfg)
	if err != nil {
		return err
	}
	printReport(cfg, handle.DevtoolsURL(), report, time.Since(started))
	refreshFipe(db)
	sendAlerts(cfg, db)
	return nil
}

func openBrowser(ctx context.Context, cfg config.Config, dir string) (*browser.Handle, error) {
	opts := browser.Options{ExistingURL: cfg.DevtoolsURL}
	if opts.ExistingURL == "" {
		executable, err := browser.Locate()
		if err != nil {
			return nil, err
		}
		opts.ExecutablePath = executable
		opts.ProfileDir = paths.ChromeProfile(dir)
		opts.Headless = browser.HeadlessNeeded(runtime.GOOS, os.Getenv)
	}
	return browser.Launch(ctx, opts)
}

func refreshFipe(db *store.Store) {
	stored, err := fipe.Refresh(context.Background(), fipe.NewHTTPFetcher(), db, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "fipe refresh failed: %v\n", err)
		return
	}
	if stored > 0 {
		fmt.Printf("fipe references refreshed: %d\n", stored)
	}
}

func runRepairSilenced(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	silenced, err := db.SilencedTwinIDs()
	if err != nil {
		return err
	}
	if len(silenced) == 0 {
		fmt.Println("no silenced listing left to free")
		return nil
	}
	fmt.Printf("freeing %d silenced listings: %s\n", len(silenced), joinIDs(silenced))

	freed, err := db.RequeueSilencedTwins()
	if err != nil {
		return err
	}
	fmt.Printf("listings back in the alert queue: %d\n", freed)
	return nil
}

func joinIDs(ids []int64) string {
	text := make([]string, 0, len(ids))
	for _, id := range ids {
		text = append(text, strconv.FormatInt(id, 10))
	}
	return strings.Join(text, ", ")
}

func sendAlerts(cfg config.Config, db *store.Store) {
	refs, err := db.FipeReferences()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fipe references unavailable, alerts will not rank by discount: %v\n", err)
	}
	shown, err := crawl.Notify(context.Background(), db, notify.New(), cfg.Crawl.MaxAlertsPerRun, fipe.NewTable(refs))
	if err != nil {
		fmt.Fprintf(os.Stderr, "alerts failed after %d notifications: %v\n", shown, err)
		return
	}
	fmt.Printf("alerts shown: %d\n", shown)
}

var httpOnlySources = map[string]bool{model.SourceMobiauto: true}

func releaseTabs(fetcher *source.BrowserFetcher) {
	if err := fetcher.ReleaseTabs(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "browser tabs left open: %v\n", err)
	}
}

func buildSources(cfg config.Config, through *source.BrowserFetcher) ([]crawl.Source, error) {
	direct := source.NewHTTPFetcher()
	sources := make([]crawl.Source, 0, len(cfg.Sources))
	for _, name := range cfg.Sources {
		fetcher := source.PageFetcher(through)
		if httpOnlySources[name] {
			fetcher = direct
		}
		switch name {
		case model.SourceOLX:
			sources = append(sources, source.NewOLX(fetcher, cfg.SourceURLs[name]))
		case model.SourceMercadoLivre:
			sources = append(sources, source.NewMercadoLivre(fetcher, cfg.SourceURLs[name]))
		case model.SourceWebmotors:
			sources = append(sources, source.NewWebmotors(fetcher, cfg.SourceURLs[name]))
		case model.SourceMobiauto:
			sources = append(sources, source.NewMobiauto(fetcher, cfg.SourceURLs[name]))
		case model.SourceInstagram:
			sources = append(sources, meta.NewInstagram(fetcher, cfg.SourceURLs[name]))
		case model.SourceMarketplace:
			sources = append(sources, meta.NewMarketplace(fetcher, cfg.SourceURLs[name]))
		default:
			return nil, fmt.Errorf("unknown source in config: %s", name)
		}
	}
	return sources, nil
}

func browserSources(names []string) []string {
	through := make([]string, 0, len(names))
	for _, name := range names {
		if !httpOnlySources[name] {
			through = append(through, name)
		}
	}
	return through
}

func printReport(cfg config.Config, devtoolsURL string, report crawl.Report, elapsed time.Duration) {
	for _, r := range report.Results {
		if r.Err != nil && report.SharedCause == nil {
			fmt.Printf("%-14s %s: %v\n", r.Source, r.Status, r.Err)
			continue
		}
		fmt.Printf("%-14s %s: %d items\n", r.Source, r.Status, r.ItemCount)
	}
	if report.SharedCause != nil {
		fmt.Printf("all %d sources failed with the same cause: %v\n", len(report.Results), report.SharedCause)
		if through := browserSources(cfg.Sources); len(through) > 0 {
			fmt.Printf("%s read their pages through Chrome at %s; check that it is running\n",
				strings.Join(through, ", "), devtoolsURL)
		}
	}
	if report.StoreFailures > 0 {
		fmt.Printf("storage failures: %d (first: %v)\n", report.StoreFailures, report.StoreErr)
	}
	fmt.Printf("new matches: %d, closed: %d in %s\n",
		report.NewMatches, report.Expired, elapsed.Round(time.Second))
}
