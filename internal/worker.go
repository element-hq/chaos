package internal

import (
	"errors"
	"log"
	"time"

	"github.com/element-hq/chaos/internal/ws"
)

type Action string

const (
	ActionJoin    Action = "join"
	ActionSend    Action = "send"
	ActionLeave   Action = "leave"
	ActionTickEOF Action = "tick_eof"
)

// Sentinel error indicating the end of the tick. Used as a synchronisation mechanism
// to let the Master know when workers have finished all their work.
var ErrTickEOF = errors.New("tick EOF")

type WorkerCommand struct {
	Action      Action
	UserID      string
	RoomID      string
	ServerNames []string
}

type Worker struct {
	Users      map[string]*CSAPI
	Chan       chan WorkerCommand
	SignalChan chan error
	wsServer   *ws.Server
}

func NewWorker(users []CSAPI, wsServer *ws.Server, recv chan WorkerCommand, err chan error) *Worker {
	w := &Worker{
		Users:      make(map[string]*CSAPI),
		Chan:       recv,
		SignalChan: err,
		wsServer:   wsServer,
	}
	for i := range users {
		w.Users[users[i].UserID] = &users[i]
	}
	return w
}

func (w *Worker) Run() {
	for cmd := range w.Chan {
		time.Sleep(time.Millisecond) // ensure a maximum frequency of 1000/second
		if cmd.Action == ActionTickEOF {
			w.SignalChan <- ErrTickEOF
			continue
		}
		user := w.Users[cmd.UserID]
		if user == nil {
			log.Fatalf("Worker received instruction for unknown user '%s' known users = %d", cmd.UserID, len(w.Users))
		}
		w.wsServer.Send(&ws.PayloadWorkerAction{
			Action: string(cmd.Action),
			UserID: cmd.UserID,
			RoomID: cmd.RoomID,
		})
		var err error
		switch cmd.Action {
		case ActionJoin:
			err = user.JoinRoom(cmd.RoomID, cmd.ServerNames)
		case ActionLeave:
			err = user.LeaveRoom(cmd.RoomID)
		case ActionSend:
			err = user.SendMessage(cmd.RoomID)
		}
		if err != nil {
			w.SignalChan <- err
		}
	}
}
