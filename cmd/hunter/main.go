package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/crawl"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/notify"
	"github.com/andreabreu76/harley-hunter/internal/source"
	"github.com/andreabreu76/harley-hunter/internal/store"
	"github.com/andreabreu76/harley-hunter/internal/web"
)

func main() {
	configPath := flag.String("config", "config/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	switch flag.Arg(0) {
	case "crawl":
		if err := runCrawl(cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "serve":
		if err := runServe(cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: hunter [-config path] <crawl|serve>")
		os.Exit(2)
	}
}

func runCrawl(cfg config.Config) error {
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	sources, err := buildSources(cfg)
	if err != nil {
		return err
	}

	started := time.Now()
	report, err := crawl.Run(context.Background(), sources, db, cfg)
	if err != nil {
		return err
	}
	printReport(cfg, report, time.Since(started))
	sendAlerts(cfg, db)
	return nil
}

func runServe(cfg config.Config) error {
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	addr := "127.0.0.1:8080"
	fmt.Printf("dashboard: http://%s\n", addr)
	return http.ListenAndServe(addr, web.NewServer(db, cfg.Sources))
}

func sendAlerts(cfg config.Config, db *store.Store) {
	shown, err := crawl.Notify(context.Background(), db, notify.NewMacOS(), cfg.Crawl.MaxAlertsPerRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "alerts failed after %d notifications: %v\n", shown, err)
		return
	}
	fmt.Printf("alerts shown: %d\n", shown)
}

func buildSources(cfg config.Config) ([]crawl.Source, error) {
	fetcher := source.NewBrowserFetcher(cfg.DevtoolsURL)
	sources := make([]crawl.Source, 0, len(cfg.Sources))
	for _, name := range cfg.Sources {
		switch name {
		case model.SourceOLX:
			sources = append(sources, source.NewOLX(fetcher, cfg.SourceURLs[name]))
		case model.SourceMercadoLivre:
			sources = append(sources, source.NewMercadoLivre(fetcher, cfg.SourceURLs[name]))
		case model.SourceWebmotors:
			sources = append(sources, source.NewWebmotors(fetcher, cfg.SourceURLs[name]))
		default:
			return nil, fmt.Errorf("unknown source in config: %s", name)
		}
	}
	return sources, nil
}

func printReport(cfg config.Config, report crawl.Report, elapsed time.Duration) {
	for _, r := range report.Results {
		if r.Err != nil && report.SharedCause == nil {
			fmt.Printf("%-14s %s: %v\n", r.Source, r.Status, r.Err)
			continue
		}
		fmt.Printf("%-14s %s: %d items\n", r.Source, r.Status, r.ItemCount)
	}
	if report.SharedCause != nil {
		fmt.Printf("all %d sources failed with the same cause: %v\n", len(report.Results), report.SharedCause)
		fmt.Printf("every source reads its pages through Chrome at %s; check that it is running\n", cfg.DevtoolsURL)
	}
	if report.StoreFailures > 0 {
		fmt.Printf("storage failures: %d (first: %v)\n", report.StoreFailures, report.StoreErr)
	}
	fmt.Printf("new matches: %d in %s\n", report.NewMatches, elapsed.Round(time.Second))
}
