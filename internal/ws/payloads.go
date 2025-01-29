package ws

import (
	"encoding/json"
	"fmt"

	"github.com/element-hq/chaos/config"
)

type Payload struct {
	payload     WSPayload
	destination int // the conn ID or if 0 multicast
}

type WSPayload interface {
	String() string
	Type() string
}

type WSMessage struct {
	ID      string
	Type    string
	Payload json.RawMessage
}

func decodeAs[T WSPayload](msg *WSMessage) (T, error) {
	val := new(T)
	err := json.Unmarshal(msg.Payload, &val)
	return *val, err
}

func (w *WSMessage) DecodePayload() (WSPayload, error) {
	switch w.Type {
	case "PayloadConfig":
		return decodeAs[*PayloadConfig](w)
	case "PayloadWorkerAction":
		return decodeAs[*PayloadWorkerAction](w)
	case "PayloadFederationRequest":
		return decodeAs[*PayloadFederationRequest](w)
	case "PayloadTickGeneration":
		return decodeAs[*PayloadTickGeneration](w)
	case "PayloadNetsplit":
		return decodeAs[*PayloadNetsplit](w)
	case "PayloadConvergence":
		return decodeAs[*PayloadConvergence](w)
	case "PayloadRestart":
		return decodeAs[*PayloadRestart](w)
	default:
		return nil, fmt.Errorf("unknown type: %s", w.Type)
	}
}

type PayloadConfig struct {
	Config config.Chaos
}

func (w *PayloadConfig) String() string {
	b, _ := json.Marshal(w.Config)
	return string(b)
}

func (w *PayloadConfig) Type() string {
	return "PayloadConfig"
}

type PayloadWorkerAction struct {
	UserID string
	RoomID string
	Action string
	Body   string
}

func (w *PayloadWorkerAction) String() string {
	return fmt.Sprintf("WorkerAction: %s %s %s %s", w.UserID, w.Action, w.RoomID, w.Body)
}

func (w *PayloadWorkerAction) Type() string {
	return "PayloadWorkerAction"
}

type PayloadFederationRequest struct {
	Method  string
	URL     string
	Body    json.RawMessage
	Blocked bool
}

func (w *PayloadFederationRequest) String() string {
	if w.Blocked {
		return fmt.Sprintf("BLOCKED: %s %s", w.Method, w.URL)
	}
	return fmt.Sprintf("%s %s", w.Method, w.URL)
}

func (w *PayloadFederationRequest) Type() string {
	return "PayloadFederationRequest"
}

type PayloadTickGeneration struct {
	Number int
	Joins  int
	Sends  int
	Leaves int
}

func (w *PayloadTickGeneration) String() string {
	return fmt.Sprintf("Tick %d: (Joins=%d, Sends=%d, Leaves=%d)", w.Number, w.Joins, w.Sends, w.Leaves)
}

func (w *PayloadTickGeneration) Type() string {
	return "PayloadTickGeneration"
}

type PayloadNetsplit struct {
	Started bool
}

func (w *PayloadNetsplit) String() string {
	if w.Started {
		return "========== NETSPLIT! ========="
	}
	return "========== NETSPLIT RESOLVED! ========="
}

func (w *PayloadNetsplit) Type() string {
	return "PayloadNetsplit"
}

type PayloadConvergence struct {
	State string
	Error string
}

func (w *PayloadConvergence) String() string {
	return fmt.Sprintf("Convergence[%s]: err=%v", w.State, w.Error)
}

func (w *PayloadConvergence) Type() string {
	return "PayloadConvergence"
}

type PayloadRestart struct {
	Domain   string
	Finished bool
}

func (w *PayloadRestart) String() string {
	if w.Finished {
		return fmt.Sprintf("Restarted server '%s'", w.Domain)
	}
	return fmt.Sprintf("Restarting server '%s'", w.Domain)
}

func (w *PayloadRestart) Type() string {
	return "PayloadRestart"
}

type PayloadSnapshot struct {
	// TODO
}

type RequestPayload struct {
	RestartServers []string
	Netsplit       *bool // TODO specify links
}
