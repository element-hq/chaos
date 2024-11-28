package internal

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/element-hq/chaos/internal/ws"
)

type ConvergenceMechanism = int

type Membership string

const (
	MembershipJoin   Membership = "join"
	MembershipLeave  Membership = "leave"
	MembershipInvite Membership = "invite"
	MembershipBan    Membership = "ban"
	MembershipKnock  Membership = "knock"
)

const (
	// Verifies convergence by using /sync as a sync mechanism.
	// This is very unreliable, and frequently gives wrong values, even for local users.
	ConvergenceMechanismSync ConvergenceMechanism = iota
	// Verifies convergence by using /members as a sync mechanism.
	// This is more reliable, but less realistic for actual clients.
	ConvergenceMechanismMembers
)

// CSAPI functions needed to do convergence checks
type CSAPIConvergence interface {
	Members(roomID string) ([]Event, error)
	Sync(syncReq SyncReq) (*SyncResponse, error)
	SendMessageWithText(roomID string, text string) (string, error)
	Event(roomID, eventID string) (*Event, error)
	GetUserID() string
}

// StateMachine functions needed to do convergence checks
type StateMachineConvergence interface {
	GetInternalState() map[string]map[string]State //user->room->state
}

type Convergence struct {
	masters       []CSAPIConvergence
	roomIDs       []string
	sm            StateMachineConvergence
	convMechanism ConvergenceMechanism
	updaterFn     func(ws.PayloadConvergence)
}

func NewConvergence(masters []CSAPIConvergence, roomIDs []string, sm StateMachineConvergence, updaterFn func(ws.PayloadConvergence)) *Convergence {
	return &Convergence{
		masters:       masters,
		roomIDs:       roomIDs,
		sm:            sm,
		convMechanism: ConvergenceMechanismMembers,
		updaterFn:     updaterFn,
	}
}

func (c *Convergence) Assert(ctx context.Context, bufferDuration time.Duration) error {
	err := c.ensureSynchronised(ctx)
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	c.updaterFn(ws.PayloadConvergence{
		State: "synchronised",
		Error: errStr,
	})
	c.updaterFn(ws.PayloadConvergence{
		State: "waiting",
	})
	time.Sleep(bufferDuration)
	// room ID => user ID => State, confusingly the inverse of StateMachine's user ID => room ID => State
	roomStates := make(map[string]map[string]State)
	state := c.sm.GetInternalState()
	for userID := range state {
		for roomID := range state[userID] {
			rs, ok := roomStates[roomID]
			if !ok {
				rs = make(map[string]State)
			}
			// collapse states to either joined or left
			s := state[userID][roomID]
			switch s {
			case StateSend:
				s = StateJoined
			case StateStart:
				s = StateLeft
			}
			rs[userID] = s
			roomStates[roomID] = rs
		}
	}
	// each master is on a different server, so we need to check state from both
	c.updaterFn(ws.PayloadConvergence{
		State: "checking",
	})
	for _, master := range c.masters {
		switch c.convMechanism {
		case ConvergenceMechanismMembers:
			err := c.assertWithMembers(master, roomStates)
			if err != nil {
				return err
			}
		case ConvergenceMechanismSync:
			err := c.assertWithSync(master, roomStates)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown convergence mechanism: %v", c.convMechanism)
		}
	}
	return nil
}

func (c *Convergence) assertWithMembers(master CSAPIConvergence, roomStates map[string]map[string]State) error {
	for roomID, wantRoomState := range roomStates {
		stateEvents, err := master.Members(roomID)
		if err != nil {
			return fmt.Errorf("/members for %s failed: %s", roomID, err)
		}
		if err := c.checkRoomState(stateEvents, nil, wantRoomState); err != nil {
			return fmt.Errorf("room %s from %s perspective mismatch: %s", roomID, master.GetUserID(), err)
		}
	}
	return nil
}

func (c *Convergence) assertWithSync(master CSAPIConvergence, roomStates map[string]map[string]State) error {
	sr, err := master.Sync(SyncReq{
		FullState: true,
	})
	if err != nil {
		return fmt.Errorf("failed to /sync on %s : %s", master.GetUserID(), err)
	}
	for roomID, roomState := range roomStates {
		room, ok := sr.Rooms.Join[roomID]
		if !ok {
			return fmt.Errorf("rooms.join.%s does not exist", roomID)
		}
		if err := c.checkRoomState(room.State.Events, room.Timeline.Events, roomState); err != nil {
			return fmt.Errorf("room %s from %s perspective mismatch: %s", roomID, master.GetUserID(), err)
		}
	}
	return nil
}

// To ensure we have synchronised rooms after a netsplit, each master sends a synchronise message and we wait
// until that has arrived before checking room state.
func (c *Convergence) ensureSynchronised(ctx context.Context) error {
	syncMessages := make(map[string][]string) // room ID => event IDs

	// each master sends a synchronise in each room. Remember the event ID of each.
	for _, master := range c.masters {
		for _, roomID := range c.roomIDs {
			eventID, err := master.SendMessageWithText(roomID, "SYNCHRONISE")
			if err != nil {
				return fmt.Errorf("master %s failed to send event in room %s : %s", master.GetUserID(), roomID, err)
			}
			syncMessages[roomID] = append(syncMessages[roomID], eventID)
		}
	}
	// sync on all masters until we see all events
	errCh := make(chan error, len(c.masters))
	var wg sync.WaitGroup
	wg.Add(len(c.masters))
	for _, master := range c.masters {
		// clone the messages so each goroutine can check them off, and use a set not a slice
		// for ergonomics
		syncMessagesCopy := make(map[string]map[string]bool)
		for roomID, eventIDs := range syncMessages {
			syncMessagesCopy[roomID] = map[string]bool{}
			for _, eventID := range eventIDs {
				syncMessagesCopy[roomID][eventID] = true
			}
		}
		go func(m CSAPIConvergence, workingCopy map[string]map[string]bool) {
			defer wg.Done()

			for len(workingCopy) > 0 {
				for roomID, eventIDs := range workingCopy {
					for eventID := range eventIDs {
						time.Sleep(10 * time.Millisecond) // avoid hammering
						ev, _ := m.Event(roomID, eventID)
						if ev == nil {
							continue // not found yet
						}
						// log.Printf("%s found event %v\n", m.UserID, eventID)
						delete(workingCopy[roomID], eventID)
						if len(workingCopy[roomID]) == 0 {
							delete(workingCopy, roomID)
						}
					}
				}
			}

			/* unreliable when run for a few minutes :/ never returns the events.
			since := ""
			for len(workingCopy) > 0 {
				log.Printf("%s working copy = %+v\n", m.UserID, workingCopy)
				res, err := m.Sync(SyncReq{
					Since:         since,
					TimeoutMillis: "1000",
					Filter:        `{"room":{"timeline":{"limit":512}}}`,
				})
				if err != nil {
					errCh <- fmt.Errorf("/sync on %s failed, terminating: %s", m.UserID, err)
					return
				}
				for roomID, room := range res.Rooms.Join {
					if len(workingCopy[roomID]) == 0 {
						delete(workingCopy, roomID)
						continue // base case
					}
					for _, ev := range room.Timeline.Events {
						delete(workingCopy[roomID], ev.ID)
					}
					if len(workingCopy[roomID]) == 0 {
						delete(workingCopy, roomID)
					}
				}
				since = res.NextBatch
				time.Sleep(1000 * time.Millisecond) // ensure we never hammer the HS
			} */
			log.Printf("  %s has synchronised", m.GetUserID())
		}(master, syncMessagesCopy)
	}
	done := make(chan bool)
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Printf("Failed to see all event IDs from all servers:\n %+v", syncMessages)
		return fmt.Errorf("context cancelled: %s", ctx.Err())
	case <-done:
	}
	return nil
}

func (c *Convergence) checkRoomState(stateEvents, timelineEvents []Event, want map[string]State) error {
	gotMemberships := make(map[string]Membership)
	processEvent := func(ev Event) {
		if ev.Type != "m.room.member" {
			return
		}
		if ev.StateKey == nil {
			return
		}
		gotMemberships[*ev.StateKey] = Membership(ev.Content["membership"].(string))
	}
	membershipToState := func(m Membership) State {
		switch m {
		case MembershipBan: // TODO
			fallthrough
		case MembershipInvite: // TODO
			fallthrough
		case MembershipKnock: // TODO
			fallthrough
		case MembershipLeave:
			return StateLeft
		case MembershipJoin:
			return StateJoined
		default:
			panic("unknown membership: " + m)
		}
	}
	for _, ev := range stateEvents {
		processEvent(ev)
	}
	for _, ev := range timelineEvents {
		processEvent(ev)
	}
	errs := []string{}
	for wantUserID, wantState := range want {
		if gotMemberships[wantUserID] == "" {
			gotMemberships[wantUserID] = MembershipLeave
		}
		if membershipToState(gotMemberships[wantUserID]) != wantState {
			errs = append(errs, fmt.Sprintf("user %s is '%s'. Want '%s'", wantUserID, gotMemberships[wantUserID], wantState))
		}
	}
	// we don't explicitly check if the server sends back MORE members than expected, as we do expect this due to
	// master users sitting in each room. We aren't really interested in that though, hence we never do
	// assert(len(got) == len(want))
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf(strings.Join(errs, "\n"))
}
