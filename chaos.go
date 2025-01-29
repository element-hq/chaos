package chaos

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
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
func Bootstrap(cfg *config.Chaos, wsServer *ws.Server) error {
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

	// process requests to netsplit or restart servers, or check for convergence / start the tests.
	// doesn't control _when_ this happens, that's the caller's responsibility.
	started := atomic.Bool{}
	convergenceRequested := atomic.Bool{}
	go func() {
		for req := range wsServer.ClientRequests() {
			// we only want to process fault injection if we aren't asked to check for convergence
			if !convergenceRequested.Load() {
				if req.Netsplit != nil {
					new := *req.Netsplit
					old := shouldBlockFederation.Swap(new)
					if old != new {
						// broadcast a netsplit state change
						wsServer.Send(&ws.PayloadNetsplit{
							Started: new,
						})
					}
				}
				for _, server := range req.RestartServers {
					for _, r := range restarters {
						domain := r.Config().Domain
						if domain == server {
							wsServer.Send(&ws.PayloadRestart{
								Domain:   domain,
								Finished: false,
							})
							r.Restart()
							wsServer.Send(&ws.PayloadRestart{
								Domain:   domain,
								Finished: true,
							})
						}
					}
				}
			}
			// To check convergence we cannot be restarting servers or doing netsplits.
			// If we're in the middle of a restart that's fine as we send sync messages to catch up
			// which will fail until we are restarted. Netsplits however are a bigger problem as they
			// are undetectable, so we block all requests to netsplit/restart until we have checked convergence,
			// and un-netsplit things immediately.
			if req.CheckConvergence {
				shouldStartChecks := convergenceRequested.CompareAndSwap(false, true)
				if shouldStartChecks { // multiple calls to check convergence no-op
					// heal the netsplit
					swapped := shouldBlockFederation.CompareAndSwap(true, false)
					if swapped {
						// tell the clients
						wsServer.Send(&ws.PayloadNetsplit{
							Started: false,
						})
					}
					// we keep convergenceRequested set, so when the tick ends and the Start callback is called, we'll
					// do a convergence check, and the callback will unset convergenceRequested.
				}
			}
			if req.Begin && started.CompareAndSwap(false, true) {
				go func() {
					m.Start(func(tickIteration int) {
						doSnapshot(snapshotters, sdb)
						if convergenceRequested.Load() {
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
							convergenceRequested.CompareAndSwap(true, false)
						}
					})
				}()
			}
		}
	}()
	return nil
}

func setupFederationInterception(wsServer *ws.Server, mitmProxyURL, hostDomain string, shouldBlock func() bool) error {
	cbServer, err := internal.NewCallbackServer(hostDomain)
	if err != nil {
		return fmt.Errorf("NewCallbackServer: %s", err)
	}
	cbURL := cbServer.SetOnRequestCallback(func(d internal.Data) *internal.Response {
		block := shouldBlock()
		if block && strings.HasSuffix(d.URL, "/.well-known/matrix/server") {
			// allow .well-known lookups. This is a bit horrible as it means netsplits aren't
			// truly netsplits, but Synapse has an in-memory cache of well-known respones, so
			// when it gets restarted it does them again which can fail if the restart happens during a netsplit.
			// If that happens, Synapse has a hardcoded 2min retry
			// See https://github.com/element-hq/synapse/blob/a00d0b3d0e72cd56733c30b1b52b5402c92f81cc/synapse/http/federation/well_known_resolver.py#L51-L53
			// which then causes chaos to time out as it doesn't expect operations to take that long.
			// These timeouts should be configurable.
			block = false
		}
		wsServer.Send(&ws.PayloadFederationRequest{
			Method:  d.Method,
			URL:     d.URL,
			Body:    d.RequestBody,
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

// Orchestrate connects to the WS server and orchestrates netsplit/restart requests based on the provided test config.
// If not test config is provided, no requests are made.
// Blocks forever.
func Orchestrate(wsPort int, verbose bool, testConfig config.TestConfig) {
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

	// setup a single writer so we don't do concurrent writes
	reqCh := make(chan ws.RequestPayload, 1)
	go func() {
		for req := range reqCh {
			if err := c.WriteJSON(req); err != nil {
				log.Fatalf("Orchestrate.WriteJSON failed: %s", err)
			}
		}
	}()

	// orchestrate netsplits/restarts/convergence
	if testConfig.Netsplits.DurationSecs > 0 {
		yes := true
		no := false
		go func() {
			for {
				time.Sleep(time.Duration(testConfig.Netsplits.FreeSecs) * time.Second)
				reqCh <- ws.RequestPayload{
					Netsplit: &yes,
				}
				time.Sleep(time.Duration(testConfig.Netsplits.DurationSecs) * time.Second)
				reqCh <- ws.RequestPayload{
					Netsplit: &no,
				}
			}
		}()
	}
	if testConfig.Restarts.IntervalSecs > 0 && len(testConfig.Restarts.RoundRobin) > 0 {
		i := 0
		go func() {
			for {
				nextHS := testConfig.Restarts.RoundRobin[i%len(testConfig.Restarts.RoundRobin)]
				time.Sleep(time.Duration(testConfig.Restarts.IntervalSecs) * time.Second)
				reqCh <- ws.RequestPayload{
					RestartServers: []string{nextHS},
				}
				i++
			}
		}()
	}
	if testConfig.Convergence.Enabled && testConfig.Convergence.IntervalSecs > 0 {
		go func() {
			for {
				time.Sleep(time.Duration(testConfig.Convergence.IntervalSecs) * time.Second)
				reqCh <- ws.RequestPayload{
					CheckConvergence: true,
				}
			}
		}()
	}

	actionPayload := ws.PayloadWorkerAction{}
	configPayload := ws.PayloadConfig{}
	for {
		var wsMessage ws.WSMessage
		if err := c.ReadJSON(&wsMessage); err != nil {
			log.Fatalf("WS ReadJSON: %s", err)
		}

		if wsMessage.Type == actionPayload.Type() && !verbose {
			continue
		}
		payload, err := wsMessage.DecodePayload()
		if err != nil {
			log.Fatalf("WS DecodePayload: %s with payload %s", err, string(wsMessage.Payload))
		}
		log.Println("> " + payload.String())
		// we start after we have been echoed back the config
		if payload.Type() == configPayload.Type() {
			reqCh <- ws.RequestPayload{
				Begin: true,
			}
		}
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
