package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/andreabreu76/harley-hunter/internal/config"
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
		fmt.Printf("crawl: %d sources enabled\n", len(cfg.Sources))
	case "serve":
		fmt.Println("serve: not implemented yet")
	default:
		fmt.Fprintln(os.Stderr, "usage: hunter [-config path] <crawl|serve>")
		os.Exit(2)
	}
}
