package main

import (
	"flag"
	"log"

	"github.com/element-hq/chaos"
	"github.com/element-hq/chaos/config"
)

func main() {
	flagConfig := flag.String("config", "", "path to the config YAML")
	flag.Parse()
	cfg, err := config.OpenFile(*flagConfig)
	if err != nil {
		log.Fatalf("Error opening config: %s", err)
	}

	// print WS traffic
	go chaos.PrintTraffic(cfg.WSPort, cfg.Verbose)

	if err := chaos.Bootstrap(cfg); err != nil {
		log.Fatalf("Bootstrap: %s", err)
	}
}
