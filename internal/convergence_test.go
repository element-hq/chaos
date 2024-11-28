package internal

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/element-hq/chaos/internal/ws"
	"github.com/stretchr/testify/assert"
)

const (
	userA = "@alice:localhost"
	userB = "@bob:localhost"
	userC = "@charlie:localhost"
	userD = "@doris:localhost"
)

type mockCSAPI struct {
	userID              string
	events              map[string]*Event
	sentCounter         int
	onSync              func(syncReq SyncReq) (*SyncResponse, error)
	onMembers           func(roomID string) ([]Event, error)
	dontAddEventsOnSend bool
}

func (c *mockCSAPI) Members(roomID string) ([]Event, error) {
	return c.onMembers(roomID)
}
func (c *mockCSAPI) Sync(syncReq SyncReq) (*SyncResponse, error) {
	return c.onSync(syncReq)
}
func (c *mockCSAPI) SendMessageWithText(roomID string, text string) (string, error) {
	c.sentCounter++
	eventID := fmt.Sprintf("$sent-%d", c.sentCounter)
	if !c.dontAddEventsOnSend {
		c.events[roomID+eventID] = &Event{
			RoomID: roomID,
			ID:     eventID,
			Type:   "m.room.message",
			Content: map[string]interface{}{
				"body": text,
			},
			Sender:    c.userID,
			Timestamp: time.Now().UnixMilli(),
		}
	}
	return eventID, nil
}

func (c *mockCSAPI) Event(roomID, eventID string) (*Event, error) {
	ev, ok := c.events[roomID+eventID]
	if !ok {
		return nil, fmt.Errorf("event %s not found", eventID)
	}
	return ev, nil
}
func (c *mockCSAPI) GetUserID() string {
	return c.userID
}
func newMockCSAPI(userID string) *mockCSAPI {
	return &mockCSAPI{
		userID: userID,
		events: make(map[string]*Event),
	}
}

type mockStateMachine map[string]map[string]State

func (sm mockStateMachine) GetInternalState() map[string]map[string]State {
	return sm
}

func createMemberEvent(roomID, sender, target string, membership Membership) Event {
	nowMillis := time.Now().UnixMilli()
	return Event{
		StateKey:  &target,
		Sender:    sender,
		Type:      "m.room.member",
		Timestamp: nowMillis,
		ID:        fmt.Sprintf("$%d", nowMillis),
		RoomID:    roomID,
		Content: map[string]interface{}{
			"membership": string(membership),
		},
	}
}

// Test convergence works in the happy case with a single room and single master
func TestConvergenceSingleMembers(t *testing.T) {
	roomID := "!room:id"
	// golden state from chaos
	sm := mockStateMachine{
		userA: map[string]State{roomID: StateJoined},
		userB: map[string]State{roomID: StateLeft},
		userC: map[string]State{roomID: StateSend},
		userD: map[string]State{roomID: StateStart},
	}
	master := newMockCSAPI("@master:localhost")
	updaterFn := func(payload ws.PayloadConvergence) {}
	master.onMembers = func(requestedRoomID string) ([]Event, error) {
		assert.Equal(t, roomID, requestedRoomID)
		return []Event{
			createMemberEvent(roomID, userA, userA, MembershipJoin),
			createMemberEvent(roomID, userB, userB, MembershipLeave),
			createMemberEvent(roomID, userC, userC, MembershipJoin),
			// no user D as they are in the START state so have never joined
		}, nil
	}
	conv := NewConvergence([]CSAPIConvergence{master}, []string{roomID}, sm, updaterFn)
	err := conv.Assert(context.Background(), 0)
	assert.NoError(t, err)
}

// Test basic convergence mismatch
func TestConvergenceMembersMismatch(t *testing.T) {
	roomID := "!room:id"
	// golden state from chaos
	sm := mockStateMachine{
		userA: map[string]State{roomID: StateJoined},
		userB: map[string]State{roomID: StateLeft},
		userC: map[string]State{roomID: StateSend},
		userD: map[string]State{roomID: StateStart},
	}
	master1 := newMockCSAPI("@master:localhost1") // has the right state
	master2 := newMockCSAPI("@master:localhost2") // has the wrong state
	updaterFn := func(payload ws.PayloadConvergence) {}
	master1.onMembers = func(requestedRoomID string) ([]Event, error) {
		assert.Equal(t, roomID, requestedRoomID)
		t.Logf("master 1 called")
		return []Event{
			createMemberEvent(roomID, userA, userA, MembershipJoin),
			createMemberEvent(roomID, userB, userB, MembershipLeave),
			createMemberEvent(roomID, userC, userC, MembershipJoin),
		}, nil
	}
	master2.onMembers = func(requestedRoomID string) ([]Event, error) {
		assert.Equal(t, roomID, requestedRoomID)
		t.Logf("master 2 called")
		return []Event{
			createMemberEvent(roomID, userA, userA, MembershipJoin),
			createMemberEvent(roomID, userB, userB, MembershipLeave),
			createMemberEvent(roomID, userC, userC, MembershipLeave), // <-- wrong!
		}, nil
	}
	conv := NewConvergence([]CSAPIConvergence{master1, master2}, []string{roomID}, sm, updaterFn)
	err := conv.Assert(context.Background(), 0)
	assert.Error(t, err)
}

// Test convergence sends updates about what it is doing
func TestConvergenceUpdates(t *testing.T) {
	roomID := "!room:id"
	// golden state from chaos
	sm := mockStateMachine{
		userA: map[string]State{roomID: StateJoined},
		userB: map[string]State{roomID: StateLeft},
		userC: map[string]State{roomID: StateSend},
		userD: map[string]State{roomID: StateStart},
	}
	master := newMockCSAPI("@master:localhost")
	i := 0
	wantStates := []string{
		"synchronised",
		"waiting",
		"checking",
	}
	updaterFn := func(payload ws.PayloadConvergence) {
		assert.Equal(t, wantStates[i], payload.State)
		i++
	}
	master.onMembers = func(requestedRoomID string) ([]Event, error) {
		assert.Equal(t, roomID, requestedRoomID)
		return []Event{
			createMemberEvent(roomID, userA, userA, MembershipJoin),
			createMemberEvent(roomID, userB, userB, MembershipLeave),
			createMemberEvent(roomID, userC, userC, MembershipJoin),
		}, nil
	}
	conv := NewConvergence([]CSAPIConvergence{master}, []string{roomID}, sm, updaterFn)
	err := conv.Assert(context.Background(), 0)
	assert.NoError(t, err)
	if i != len(wantStates) {
		t.Errorf("did not see all desired states: i=%d want=%v", i, wantStates)
	}
}

// Tests that if /members returns an error, convergence fails.
func TestConvergenceMembersFailCascades(t *testing.T) {
	roomID := "!room:id"
	// golden state from chaos
	sm := mockStateMachine{
		userA: map[string]State{roomID: StateJoined},
	}
	master := newMockCSAPI("@master:localhost")
	updaterFn := func(payload ws.PayloadConvergence) {}
	master.onMembers = func(requestedRoomID string) ([]Event, error) {
		assert.Equal(t, roomID, requestedRoomID)
		return nil, fmt.Errorf("oh no!")
	}
	conv := NewConvergence([]CSAPIConvergence{master}, []string{roomID}, sm, updaterFn)
	err := conv.Assert(context.Background(), 0)
	assert.Error(t, err)
}

// Tests that if /event never returns the sent event, we time out when the context is cancelled.
func TestConvergenceEventCtxTimeout(t *testing.T) {
	roomID := "!room:id"
	// golden state from chaos
	sm := mockStateMachine{
		userA: map[string]State{roomID: StateJoined},
	}
	master := newMockCSAPI("@master:localhost")
	master.dontAddEventsOnSend = true
	master.onMembers = func(requestedRoomID string) ([]Event, error) {
		assert.Equal(t, roomID, requestedRoomID)
		return []Event{
			createMemberEvent(roomID, userA, userA, MembershipJoin),
		}, nil
	}
	updaterFn := func(payload ws.PayloadConvergence) {}
	conv := NewConvergence([]CSAPIConvergence{master}, []string{roomID}, sm, updaterFn)
	assertFinished := &atomic.Bool{}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(500 * time.Millisecond)
		assert.Equal(t, false, assertFinished.Load())
		// cancel the context to give up. We still run convergence checks even if we fail to synchronise.
		cancel()
	}()
	err := conv.Assert(ctx, 0)
	assertFinished.Store(true)
	assert.NoError(t, err)
	wg.Wait()
}
