package internal

import (
	"context"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/element-hq/chaos/config"
	"github.com/element-hq/chaos/ws"
)

type Master struct {
	cfg            *config.Chaos
	roomIDs        []string
	users          []CSAPI
	userIDToWorker map[string]*Worker
	workers        []*Worker
	masters        []CSAPI
	convergence    *Convergence
	wsServer       *ws.Server
}

func NewMaster(wsServer *ws.Server) *Master {
	return &Master{
		userIDToWorker: make(map[string]*Worker),
		wsServer:       wsServer,
	}
}

func (m *Master) Prepare(cfg *config.Chaos) error {
	m.cfg = cfg
	now := time.Now()
	// create masters on each server.
	// They will create the rooms and lurk to ensure that if all test users leave the room is still joinable.
	var masters []CSAPI
	var servers []struct {
		URL    string
		Domain string
	}
	var masterIDs []string
	for _, hs := range cfg.Homeservers {
		hsurl := hs.BaseURL
		master, err := m.registerUser(hs.Domain, fmt.Sprintf("master-%d", now.UnixMilli()), hsurl, cfg.Verbose)
		if err != nil {
			return fmt.Errorf("error registering master user on %s : %s", hsurl, err)
		}
		masters = append(masters, master)
		servers = append(servers, struct {
			URL    string
			Domain string
		}{
			URL:    hsurl,
			Domain: hs.Domain,
		})
		masterIDs = append(masterIDs, master.UserID)
	}
	log.Printf("Created masters: %v", masterIDs)
	// create the required rooms. Cycle who creates them to ensure we don't make them all on one server.

	ch := make(chan int, cfg.Test.NumRooms)
	for i := 0; i < cfg.Test.NumRooms; i++ {
		ch <- i
	}
	close(ch)
	errChan := make(chan error, cfg.Test.NumInitGoroutines)
	resultRoomsCh := make(chan *string, cfg.Test.NumRooms)

	var roomIDs []string

	var wgRooms sync.WaitGroup
	wgRooms.Add(cfg.Test.NumInitGoroutines)

	for i := 0; i < cfg.Test.NumInitGoroutines; i++ {
		go func() {
			defer wgRooms.Done()
			for work := range ch {
				creatorIndex := work % len(masters)
				creator := masters[creatorIndex]
				createOpts := map[string]interface{}{
					"preset": "public_chat",
				}
				if cfg.Test.RoomVersion != "" {
					createOpts["room_version"] = cfg.Test.RoomVersion
				}
				roomID, err := creator.CreateRoom(createOpts)
				if err != nil {
					errChan <- fmt.Errorf("%s failed to create room: %s", creator.UserID, err)
					return
				}
				// everyone else joins the room
				for m := range masters {
					if m == creatorIndex {
						continue
					}
					if err := masters[m].JoinRoom(roomID, []string{creator.Domain}); err != nil {
						errChan <- fmt.Errorf("%s failed to join room %s : %s", masters[m].UserID, roomID, err)
						return
					}
					masters[m].EnsureFullyJoined(roomID)
				}
				resultRoomsCh <- &roomID
			}
		}()
	}
	wgRooms.Wait()
	close(errChan)
	close(resultRoomsCh)

	for err := range errChan {
		return fmt.Errorf("failed to create rooms: %s", err)
	}

	for roomID := range resultRoomsCh {
		roomIDs = append(roomIDs, *roomID)
	}
	log.Printf("Created rooms: %v", roomIDs)

	// create the users, alternating each server
	var users []CSAPI
	var userIDs []string

	ch = make(chan int, cfg.Test.NumUsers)
	for i := 0; i < cfg.Test.NumUsers; i++ {
		ch <- i
	}
	close(ch)
	errChan = make(chan error, cfg.Test.NumInitGoroutines)
	resultUsersCh := make(chan *CSAPI, cfg.Test.NumUsers)

	var wgUsers sync.WaitGroup
	wgUsers.Add(cfg.Test.NumInitGoroutines)

	for i := 0; i < cfg.Test.NumInitGoroutines; i++ {
		go func() {
			defer wgUsers.Done()
			for work := range ch {
				server := servers[work%len(servers)]
				u, err := m.registerUser(server.Domain, fmt.Sprintf("user-%d-%d", now.UnixMilli(), work), server.URL, cfg.Verbose)
				if err != nil {
					errChan <- fmt.Errorf("failed to register user on domain %s: %s", server.Domain, err)
					return
				}
				resultUsersCh <- &u
			}
		}()
	}
	wgUsers.Wait()
	close(errChan)
	close(resultUsersCh)

	for err := range errChan {
		return fmt.Errorf("failed to create users: %s", err)
	}

	for user := range resultUsersCh {
		users = append(users, *user)
		userIDs = append(userIDs, user.UserID)
	}

	log.Printf("Created users: %v", userIDs)
	m.roomIDs = roomIDs
	m.users = users
	m.masters = masters
	return nil
}

// StartWorkers starts the requested number of workers, returning their user IDs
func (m *Master) StartWorkers(numWorkers, opsPerTick int) []string {
	if numWorkers > len(m.users) {
		log.Printf("Requested %d workers but only %d users exist, setting workers to %d", numWorkers, len(m.users), len(m.users))
		numWorkers = len(m.users)
	}
	// TODO: handle multi users per worker
	if numWorkers < len(m.users) {
		panic("not implemented")
	}
	var result []string
	for i := 0; i < numWorkers; i++ {
		users := []CSAPI{m.users[i]}
		// if the tick randomly makes work all for one worker we want to be able to queue it all up without blocking + EOF signal
		workerCh := make(chan WorkerCommand, opsPerTick+1)
		// if an error is sent back or if we EOF we should block the worker
		errCh := make(chan error)
		w := NewWorker(users, m.wsServer, workerCh, errCh)
		for _, u := range users {
			m.userIDToWorker[u.UserID] = w
			result = append(result, u.UserID)
		}
		m.workers = append(m.workers, w)
		go w.Run()
	}
	log.Printf("Started %d workers", numWorkers)
	if len(m.userIDToWorker) != len(m.users) {
		log.Fatalf("not all users have workers: %d != %d", len(m.userIDToWorker), len(m.users))
	}
	return result
}

func (m *Master) Start(postTickFn func(tickIteration int)) {
	userIDs := slices.Collect(maps.Keys(m.userIDToWorker))
	stateMachine := NewStateMachine(m.cfg.Test.Seed, m.cfg.Test.OpsPerTick, m.cfg.Test.SendToLeaveProbability, userIDs, m.roomIDs)
	convMasters := make([]CSAPIConvergence, len(m.masters))
	for i := range convMasters {
		convMasters[i] = &m.masters[i]
	}
	m.convergence = NewConvergence(convMasters, m.roomIDs, stateMachine, func(wc ws.PayloadConvergence) {
		m.wsServer.Send(&wc)
	})
	for {
		var joins, sends, leaves int = 0, 0, 0
		cmds := stateMachine.Tick()
		for _, cmd := range cmds {
			switch cmd.Action {
			case ActionJoin:
				joins++
			case ActionLeave:
				leaves++
			case ActionSend:
				sends++
			}
			w := m.userIDToWorker[cmd.UserID]
			if w == nil {
				log.Fatalf("unknown user %s", cmd.UserID)
			}
			w.Chan <- cmd
		}
		// send EOF action last so we know when workers are done
		for _, w := range m.workers {
			w.Chan <- WorkerCommand{
				Action: ActionTickEOF,
			}
		}
		m.wsServer.Send(&ws.PayloadTickGeneration{
			Number: stateMachine.Index,
			Joins:  joins,
			Sends:  sends,
			Leaves: leaves,
		})
		// wait for responses
		for _, w := range m.workers {
			for signalErr := range w.SignalChan {
				if signalErr == ErrTickEOF {
					// wait until we see EOF then go to the next worker
					break
				}
				// otherwise we got a genuine error. This could be a CSAPI timeout and hence ephemeral
				// but it breaks the state machine so we have to terminate. If we were cleverer, we could
				// rollback the state transition.
				log.Fatalf("worker returned an error, terminating: %s", signalErr)
			}
		}
		// we either paniced or saw EOF from every worker, so update our internal state and go onto the next tick.
		stateMachine.Apply(cmds)
		if postTickFn != nil {
			postTickFn(stateMachine.Index)
		}
	}
}

func (m *Master) CheckConverged(syncTimeoutDuration, bufferDuration time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeoutDuration)
	defer cancel()
	return m.convergence.Assert(ctx, bufferDuration)
}

func (m *Master) registerUser(hsDomain, localpart, serverURL string, verbose bool) (CSAPI, error) {
	client := CSAPI{
		BaseURL: serverURL,
		Domain:  hsDomain,
		Client: &http.Client{
			Timeout:   20 * time.Second,
			Transport: &LocalhostRoundTripper{},
		},
		Debug: verbose,
	}
	err := client.Register(localpart)
	return client, err
}
