package internal

import (
	"context"
	"fmt"
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
	userID      string
	events      map[string]*Event
	sentCounter int
	onSync      func(syncReq SyncReq) (*SyncResponse, error)
	onMembers   func(roomID string) ([]Event, error)
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
