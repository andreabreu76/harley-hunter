package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/andreabreu76/harley-hunter/internal/config"
	"github.com/andreabreu76/harley-hunter/internal/crawl"
	"github.com/andreabreu76/harley-hunter/internal/fipe"
	"github.com/andreabreu76/harley-hunter/internal/model"
	"github.com/andreabreu76/harley-hunter/internal/notify"
	"github.com/andreabreu76/harley-hunter/internal/source"
	"github.com/andreabreu76/harley-hunter/internal/source/meta"
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
	refreshFipe(db)
	sendAlerts(cfg, db, report.Drops)
	return nil
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

func sendAlerts(cfg config.Config, db *store.Store, drops []crawl.PriceDrop) {
	shown, err := crawl.Notify(context.Background(), db, notify.NewMacOS(), cfg.Crawl.MaxAlertsPerRun, drops)
	if err != nil {
		fmt.Fprintf(os.Stderr, "alerts failed after %d notifications: %v\n", shown, err)
		return
	}
	fmt.Printf("alerts shown: %d\n", shown)
}

var httpOnlySources = map[string]bool{model.SourceMobiauto: true}

func buildSources(cfg config.Config) ([]crawl.Source, error) {
	browser := source.NewBrowserFetcher(cfg.DevtoolsURL)
	direct := source.NewHTTPFetcher()
	sources := make([]crawl.Source, 0, len(cfg.Sources))
	for _, name := range cfg.Sources {
		fetcher := source.PageFetcher(browser)
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
		if through := browserSources(cfg.Sources); len(through) > 0 {
			fmt.Printf("%s read their pages through Chrome at %s; check that it is running\n",
				strings.Join(through, ", "), cfg.DevtoolsURL)
		}
	}
	if report.StoreFailures > 0 {
		fmt.Printf("storage failures: %d (first: %v)\n", report.StoreFailures, report.StoreErr)
	}
	fmt.Printf("new matches: %d, closed: %d in %s\n",
		report.NewMatches, report.Expired, elapsed.Round(time.Second))
}
