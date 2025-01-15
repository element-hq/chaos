package internal

import (
	"reflect"
	"slices"
	"testing"
)

func TestStateMachineOnlyDoesValidTransitions(t *testing.T) {
	validStateTransitions := map[State][]State{
		StateJoined: {StateLeft, StateSend},
		StateLeft:   {StateJoined},
		StateSend:   {StateSend, StateLeft},
		StateStart:  {StateJoined},
	}
	sm := NewStateMachine(42, 10, 10, []string{"alice", "bob"}, []string{"!foo", "!bar", "!baz"})

	for i := 0; i < 100; i++ {
		cmds := sm.Tick()
		workingCopy := sm.copyInternalState()
		for _, cmd := range cmds {
			prevState := workingCopy[cmd.UserID][cmd.RoomID]
			validNextStates := validStateTransitions[prevState]
			if !slices.Contains(validNextStates, actionToState(cmd.Action)) {
				t.Fatalf("invalid state transition %v => %v", prevState, cmd.Action)
			}
			workingCopy[cmd.UserID][cmd.RoomID] = actionToState(cmd.Action)
		}
		sm.Apply(cmds)
	}
}

func TestStateMachineIsDeterministic(t *testing.T) {
	sm := NewStateMachine(42, 4, 10, []string{"alice", "bob"}, []string{"!foo", "!bar", "!baz"})
	cmds := sm.Tick()
	sm.Apply(cmds)
	want := sm.userToRoomStates
	for i := 0; i < 100; i++ {
		sm := NewStateMachine(42, 4, 10, []string{"bob", "alice"}, []string{"!foo", "!baz", "!bar"})
		cmds := sm.Tick()
		sm.Apply(cmds)
		if !reflect.DeepEqual(sm.userToRoomStates, want) {
			t.Errorf("iteration %d: got %+v want %+v", i, sm.userToRoomStates, want)
		}
	}
}
