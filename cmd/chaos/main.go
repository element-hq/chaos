package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/element-hq/chaos"
	"github.com/element-hq/chaos/config"
	"github.com/element-hq/chaos/internal/ws"
)

func main() {
	flagConfig := flag.String("config", "", "path to the config YAML")
	flagTimeoutSecs := flag.Int("timeout_secs", 0, "number of seconds to run chaos")
	flag.Parse()
	cfg, err := config.OpenFile(*flagConfig)
	if err != nil {
		log.Fatalf("Error opening config: %s", err)
	}

	timeoutSecs := *flagTimeoutSecs
	if timeoutSecs > 0 {
		log.Printf("Terminating in %ds\n", timeoutSecs)
		go func() {
			time.Sleep(time.Duration(timeoutSecs) * time.Second)
			os.Exit(0)
		}()
	}

	wsServer := ws.NewServer(cfg)
	if err := chaos.Bootstrap(cfg, wsServer); err != nil {
		log.Fatalf("Bootstrap: %s", err)
	}

	// blocks forever
	chaos.Orchestrate(cfg.WSPort, cfg.Verbose, cfg.Test)
}
