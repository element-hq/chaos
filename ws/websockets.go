package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/element-hq/chaos/config"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Server struct {
	upgrader      websocket.Upgrader
	ch            chan Payload
	reqChan       chan RequestPayload
	cfg           *config.Chaos
	workerUserIDs []string

	mu      *sync.Mutex
	conns   map[int]*websocket.Conn
	counter int
}

func NewServer(cfg *config.Chaos) *Server {
	return &Server{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		}, // use default options
		ch:      make(chan Payload, 100),
		reqChan: make(chan RequestPayload, 100),
		mu:      &sync.Mutex{},
		conns:   make(map[int]*websocket.Conn),
		cfg:     cfg,
	}
}

func (s *Server) SetWorkers(workerUserIDs []string) {
	s.workerUserIDs = workerUserIDs
}

func (s *Server) Send(payload WSPayload) {
	select {
	case s.ch <- Payload{
		payload: payload,
	}:
	case <-time.After(time.Second):
		log.Printf("failed to send '%s' payload, timed out after 1s\n", payload.Type())
	}
}
func (s *Server) sendDirect(payload WSPayload, destination int) {
	select {
	case s.ch <- Payload{
		payload:     payload,
		destination: destination,
	}:
	case <-time.After(time.Second):
		log.Printf("failed to send direct '%s' payload, timed out after 1s\n", payload.Type())
	}
}

func (s *Server) toWSMessage(p WSPayload) *WSMessage {
	wrapper := &WSMessage{
		ID:   uuid.NewString(),
		Type: p.Type(),
	}
	jsonPayload, err := json.Marshal(p)
	if err != nil {
		log.Fatalf("toWSMessage: %s", err)
	}
	wrapper.Payload = jsonPayload
	return wrapper
}

func (s *Server) Start(addr string) {
	go func() {
		for p := range s.ch {
			wrapper := s.toWSMessage(p.payload)
			var conns []*websocket.Conn
			// gather connections with lock, but send without the lock
			s.mu.Lock()
			if p.destination != 0 {
				c := s.conns[p.destination]
				if c != nil {
					conns = append(conns, c)
				}
			} else {
				for _, c := range s.conns {
					conns = append(conns, c)
				}
			}
			s.mu.Unlock()
			for _, conn := range conns {
				conn.WriteJSON(wrapper)
			}

		}
	}()
	http.ListenAndServe(addr, s)
}

func (s *Server) addConn(c *websocket.Conn) (id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.counter++
	s.conns[s.counter] = c
	return s.counter
}
func (s *Server) removeConn(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, id)
}

func (s *Server) ClientRequests() <-chan RequestPayload {
	return s.reqChan
}

func (s *Server) readRequests(conn *websocket.Conn) {
	for {
		var wsReq RequestPayload
		err := conn.ReadJSON(&wsReq)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Server.readRequests: %v", err)
			}
			break
		}
		s.reqChan <- wsReq
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("incoming WS connection from %v", r.RemoteAddr)
	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	id := s.addConn(c)
	go s.readRequests(c)
	s.sendDirect(&PayloadConfig{
		Config:        *s.cfg,
		WorkerUserIDs: s.workerUserIDs,
	}, id)
	c.SetCloseHandler(func(code int, text string) error {
		s.removeConn(id)
		message := websocket.FormatCloseMessage(code, "")
		c.WriteControl(websocket.CloseMessage, message, time.Now().Add(time.Second))
		return nil
	})
}
