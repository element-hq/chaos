package internal

import (
	"math/rand"
	"slices"
)

type State string

var (
	StateStart  State = "start"
	StateJoined State = "joined"
	StateSend   State = "send"
	StateLeft   State = "left"
)

type StateMachine struct {
	Index            int
	source           rand.Source
	opsPerTick       int
	userIDs          []string
	roomIDs          []string
	userToRoomStates map[string]map[string]State // user id => room id => state
}

func NewStateMachine(seed int64, opsPerTick int, userIDs []string, roomIDs []string) *StateMachine {
	userToRoomStates := make(map[string]map[string]State)
	for _, u := range userIDs {
		userToRoomStates[u] = make(map[string]State)
		for _, r := range roomIDs {
			userToRoomStates[u][r] = StateStart
		}
	}

	// ensure we get deterministic execution orders, we'll index into these arrays
	slices.Sort(userIDs)
	slices.Sort(roomIDs)
	return &StateMachine{
		source:           rand.NewSource(seed),
		opsPerTick:       opsPerTick,
		userToRoomStates: userToRoomStates,
		userIDs:          userIDs,
		roomIDs:          roomIDs,
	}
}

func (s *StateMachine) Tick() []WorkerCommand {
	s.Index++
	var cmds []WorkerCommand

	// copy the current state so we can mutate it as we make commands.
	// This allows us to queue up commands for the same (user, room) sensibly.
	workingCopy := s.copyInternalState()

	for i := 0; i < s.opsPerTick; i++ {
		// pick a random user
		userID := s.userIDs[s.random(len(s.userIDs))]
		// pick a random room
		roomID := s.roomIDs[s.random(len(s.roomIDs))]
		// modify the state
		switch workingCopy[userID][roomID] {
		case StateStart:
			fallthrough
		case StateLeft:
			// the only valid state transition is to join, so do it
			cmds = append(cmds, WorkerCommand{
				Action: ActionJoin,
				UserID: userID,
				RoomID: roomID,
			})
			workingCopy[userID][roomID] = StateJoined
		case StateJoined:
			fallthrough
		case StateSend:
			// either send a message or leave. We leave at 10% probability
			shouldLeave := s.random(100)%10 == 0
			if shouldLeave {
				cmds = append(cmds, WorkerCommand{
					Action: ActionLeave,
					UserID: userID,
					RoomID: roomID,
				})
				workingCopy[userID][roomID] = StateLeft
			} else {
				cmds = append(cmds, WorkerCommand{
					Action: ActionSend,
					UserID: userID,
					RoomID: roomID,
				})
				workingCopy[userID][roomID] = StateSend
			}
		}
	}
	return cmds
}

func (s *StateMachine) Apply(cmds []WorkerCommand) {
	for _, cmd := range cmds {
		s.userToRoomStates[cmd.UserID][cmd.RoomID] = actionToState(cmd.Action)
	}
}

func (s *StateMachine) copyInternalState() map[string]map[string]State {
	workingCopy := make(map[string]map[string]State)
	for u := range s.userToRoomStates {
		workingCopy[u] = make(map[string]State)
		for r := range s.userToRoomStates[u] {
			s := s.userToRoomStates[u][r]
			workingCopy[u][r] = s
		}
	}
	return workingCopy
}

func (s *StateMachine) random(max int) int {
	val := int(s.source.Int63())
	return val % max
}

func actionToState(a Action) State {
	switch a {
	case ActionJoin:
		return StateJoined
	case ActionLeave:
		return StateLeft
	case ActionSend:
		return StateSend
	}
	panic("unreachable")
}
