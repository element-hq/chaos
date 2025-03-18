package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/element-hq/chaos"
	"github.com/element-hq/chaos/config"
	"github.com/element-hq/chaos/ws"
)

func open(url string) error {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start()
}

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
		hasServersOtherThanHS12 := false
		for _, hs := range cfg.Homeservers {
			if hs.Domain != "hs1" && hs.Domain != "hs2" {
				hasServersOtherThanHS12 = true
				break
			}
		}
		if hasServersOtherThanHS12 {
			log.Println("WARNING:")
			log.Println("The web UI only current supports 2 servers named 'hs1' and 'hs2'.")
			log.Println("The config you have provided suggests there are other servers with different names, which will not render correctly. Use without --web.")
		}
		go func() {
			time.Sleep(500 * time.Millisecond)
			open(fmt.Sprintf("http://localhost:%d", *flagWebPort))
		}()
		// spin up an HTTP server which will start Chaos / issue faults
		chaos.Web(*flagWebPort)
	} else {
		// use the provided test yaml to automate fault injection
		chaos.Orchestrate(cfg.WSPort, cfg.Verbose, cfg.Test)
	}
}
