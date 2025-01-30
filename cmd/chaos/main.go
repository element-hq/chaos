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
	flagWeb := flag.Bool("web", false, "Enable the web UI and don't automate actions")
	flagWebPort := flag.Int("web-port", 3405, "Listen on this port")
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
	if *flagWeb {
		// spin up an HTTP server which will start Chaos / issue faults
		chaos.Web(*flagWebPort)
	} else {
		// use the provided test yaml to automate fault injection
		chaos.Orchestrate(cfg.WSPort, cfg.Verbose, cfg.Test)
	}
}
