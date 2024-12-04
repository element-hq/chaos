package chaos

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/element-hq/chaos/config"
	"github.com/element-hq/chaos/internal"
	"github.com/element-hq/chaos/internal/ws"
	"github.com/element-hq/chaos/restart"
	"github.com/element-hq/chaos/snapshot"
	"github.com/gorilla/websocket"
)

type CreateSnapshotter func(hsc config.HomeserverConfig) (snapshot.Snapshotter, error)
type CreateRestarter func(hsc config.HomeserverConfig) (restart.Restarter, error)

var snapshotTypes = map[string]CreateSnapshotter{
	snapshot.SnapshotTypeDocker: snapshot.NewDockerSnapshotter,
}
var restartTypes = map[string]CreateRestarter{
	restart.RestartTypeDocker: restart.NewDockerRestarter,
}

// RegisterSnapshotter registers a new snapshot type with Chaos.
// The provided function will be invoked for any homeserver config which contains
// the provided snapshot.type.
func RegisterSnapshotter(snapshotType string, snapshotterCreateFn CreateSnapshotter) {
	snapshotTypes[snapshotType] = snapshotterCreateFn
}

// RegisterRestarter registers a new restart type with Chaos.
// The provided function will be invoked for any homeserver config which contains
// the provided restart.type.
func RegisterRestarter(restartType string, restarterCreateFn CreateRestarter) {
	restartTypes[restartType] = restarterCreateFn
}

// Bootstrap is the entry point for running Chaos.
func Bootstrap(cfg *config.Chaos) error {
	var snapshotters []snapshot.Snapshotter
	var restarters []restart.Restarter
	for _, hs := range cfg.Homeservers {
		if hs.Snapshot.Type != "" {
			snapshotCreator := snapshotTypes[hs.Snapshot.Type]
			if snapshotCreator == nil {
				return fmt.Errorf("hs %s has an unsupported snapshot type: %s", hs.Domain, hs.Snapshot.Type)
			}
			snapshotter, err := snapshotCreator(hs)
			if err != nil {
				return fmt.Errorf("hs %s : failed to create snapshotter of type %s: %s", hs.Domain, hs.Snapshot.Type, err)
			}
			snapshotters = append(snapshotters, snapshotter)
		}
		if hs.Restart.Type != "" {
			restartCreator := restartTypes[hs.Restart.Type]
			if restartCreator == nil {
				return fmt.Errorf("hs %s has an unsupported restart type: %s", hs.Domain, hs.Restart.Type)
			}
			restarter, err := restartCreator(hs)
			if err != nil {
				return fmt.Errorf("hs %s : failed to create restarter of type %s: %s", hs.Domain, hs.Restart.Type, err)
			}
			restarters = append(restarters, restarter)
		}
	}

	sdb, err := snapshot.NewStorage(cfg.Test.SnapshotDB)
	if err != nil {
		log.Fatalf("snapshot.NewStorage: %s", err)
	}
	doSnapshot(snapshotters, sdb)
	wsServer := ws.NewServer(cfg)

	var shouldBlockFederation atomic.Bool
	if err := setupFederationInterception(wsServer, cfg.MITMProxy.ContainerURL, cfg.MITMProxy.HostDomain, func() bool {
		return shouldBlockFederation.Load()
	}); err != nil {
		log.Fatalf("setupFederationInterception: %s", err)
	}

	go wsServer.Start(fmt.Sprintf("0.0.0.0:%d", cfg.WSPort))

	m := internal.NewMaster(wsServer)
	if err := m.Prepare(cfg); err != nil {
		log.Fatalf("Prepare: %s", err)
	}
	m.StartWorkers(cfg.Test.NumUsers, cfg.Test.OpsPerTick)

	go func() {
		for {
			wsServer.Send(&ws.PayloadNetsplit{
				Started: false,
			})
			shouldBlockFederation.Store(false)
			time.Sleep(time.Duration(cfg.Test.Netsplits.FreeSecs) * time.Second)
			shouldBlockFederation.Store(true)
			wsServer.Send(&ws.PayloadNetsplit{
				Started:      true,
				DurationSecs: cfg.Test.Netsplits.DurationSecs,
			})
			time.Sleep(time.Duration(cfg.Test.Netsplits.DurationSecs) * time.Second)
		}
	}()

	if cfg.Test.Restarts.IntervalSecs > 0 && len(restarters) > 0 {
		log.Printf("Tests with restarts every %d seconds\n", cfg.Test.Restarts.IntervalSecs)
		go func() {
			i := 0
			for {
				time.Sleep(time.Duration(cfg.Test.Restarts.IntervalSecs) * time.Second)
				r := restarters[i%len(restarters)]
				wsServer.Send(&ws.PayloadRestart{
					Domain:   r.Config().Domain,
					Finished: false,
				})
				r.Restart()
				wsServer.Send(&ws.PayloadRestart{
					Domain:   r.Config().Domain,
					Finished: true,
				})
				i++
			}
		}()
	}

	m.Start(cfg.Test.Seed, cfg.Test.OpsPerTick, func(tickIteration int) {
		doSnapshot(snapshotters, sdb)
		if cfg.Test.Convergence.Enabled && tickIteration%cfg.Test.Convergence.CheckEveryNTicks == 0 {
			// TODO: stop the netsplit goroutine from spliting during this process.
			wsServer.Send(&ws.PayloadConvergence{
				State: "starting",
			})
			if err := m.CheckConverged(time.Duration(cfg.Test.Convergence.BufferDurationSecs) * time.Second); err != nil {
				wsServer.Send(&ws.PayloadConvergence{
					State: "failure",
					Error: err.Error(),
				})
				return
			}
			wsServer.Send(&ws.PayloadConvergence{
				State: "success",
			})
		}
	})
	return nil // unreachable
}

func setupFederationInterception(wsServer *ws.Server, mitmProxyURL, hostDomain string, shouldBlock func() bool) error {
	cbServer, err := internal.NewCallbackServer(hostDomain)
	if err != nil {
		return fmt.Errorf("NewCallbackServer: %s", err)
	}
	cbURL := cbServer.SetOnRequestCallback(func(d internal.Data) *internal.Response {
		block := shouldBlock()
		wsServer.Send(&ws.PayloadFederationRequest{
			Method:  d.Method,
			URL:     d.URL,
			Blocked: block,
		})
		if block {
			return &internal.Response{
				RespondStatusCode: http.StatusGatewayTimeout,
				RespondBody:       []byte(`{"error":"gateway timeout"}`),
			}
		}
		return &internal.Response{} // let all requests through
	})
	proxyURL, err := url.Parse(mitmProxyURL)
	if err != nil {
		return fmt.Errorf("failed to parse mitmproxy url: %s", err)
	}
	mitmClient := internal.NewClient(proxyURL)

	// handle CTRL+C so we unlock correctly
	var lockIDAtomic atomic.Value
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigs
		v := lockIDAtomic.Load()
		if v != nil {
			mitmClient.UnlockOptions(v.([]byte))
		}
		os.Exit(0)
	}()

	lockID, err := mitmClient.LockOptions(map[string]any{
		"callback": map[string]any{
			"callback_request_url": cbURL,
		},
	})
	if err != nil {
		return fmt.Errorf("LockOptions: %s", err)
	}
	lockIDAtomic.Store(lockID)
	return nil
}

func PrintTraffic(wsPort int, verbose bool) {
	addr := fmt.Sprintf("ws://localhost:%d", wsPort)
	log.Printf("Dialling %s\n", addr)
	var c *websocket.Conn
	var err error

	now := time.Now()
	for c == nil {
		c, _, err = websocket.DefaultDialer.Dial(addr, nil)
		if err != nil {
			log.Printf("WS dial: %s\n", err)
			time.Sleep(10 * time.Millisecond)
		}
		if time.Since(now) > time.Second {
			log.Fatal("cannot connect to WS server")
		}
	}

	for {
		var wsMessage ws.WSMessage
		if err := c.ReadJSON(&wsMessage); err != nil {
			log.Fatalf("WS ReadJSON: %s", err)
		}
		action := ws.PayloadWorkerAction{}
		if wsMessage.Type == action.Type() && !verbose {
			continue
		}
		payload, err := wsMessage.DecodePayload()
		if err != nil {
			log.Fatalf("WS DecodePayload: %s with payload %s", err, string(wsMessage.Payload))
		}
		log.Println("> " + payload.String())
	}
}

func doSnapshot(snapshotters []snapshot.Snapshotter, sdb *snapshot.Storage) {
	var procEntries []snapshot.ProcessSnapshot
	for _, s := range snapshotters {
		snapshot, err := s.Snapshot()
		if err != nil {
			log.Fatalf("Failed to snapshot: %s", err)
		}
		procEntries = append(procEntries, snapshot.ProcessEntries...)
	}
	if err := sdb.WriteSnapshot(snapshot.Snapshot{
		ProcessEntries: procEntries,
	}); err != nil {
		log.Fatalf("Failed to write snapshot: %s", err)
	}
}
