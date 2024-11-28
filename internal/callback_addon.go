package internal

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
)

// Fn represents the callback function to invoke
type Fn func(Data) *Response

type Data struct {
	Method       string          `json:"method"`
	URL          string          `json:"url"`
	AccessToken  string          `json:"access_token"`
	ResponseCode int             `json:"response_code"`
	ResponseBody json.RawMessage `json:"response_body"`
	RequestBody  json.RawMessage `json:"request_body"`
}

type Response struct {
	// if set, changes the HTTP response status code for this request.
	RespondStatusCode int `json:"respond_status_code,omitempty"`
	// if set, changes the HTTP response body for this request.
	RespondBody json.RawMessage `json:"respond_body,omitempty"`
}

func (cd Data) String() string {
	return fmt.Sprintf("%s %s (token=%s) req_len=%d => HTTP %v", cd.Method, cd.URL, cd.AccessToken, len(cd.RequestBody), cd.ResponseCode)
}

const (
	requestPath  = "/request"
	responsePath = "/response"
)

type CallbackServer struct {
	srv     *http.Server
	mux     *http.ServeMux
	baseURL string

	mu         *sync.Mutex
	onRequest  http.HandlerFunc
	onResponse http.HandlerFunc
}

func (s *CallbackServer) SetOnRequestCallback(cb Fn) (callbackURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onRequest = s.createHandler(cb)
	return s.baseURL + requestPath
}
func (s *CallbackServer) SetOnResponseCallback(cb Fn) (callbackURL string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onResponse = s.createHandler(cb)
	return s.baseURL + responsePath
}

// Shut down the server.
func (s *CallbackServer) Close() {
	s.srv.Close()
}
func (s *CallbackServer) createHandler(cb Fn) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var data Data
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			log.Printf("CallbackServer: error decoding json: %s\n", err)
			w.WriteHeader(500)
			return
		}
		cbRes := cb(data)
		w.Header().Add("Content-Type", "application/json")
		w.WriteHeader(200)
		if cbRes == nil {
			w.Write([]byte(`{}`))
			return
		}
		cbResBytes, err := json.Marshal(cbRes)
		if err != nil {
			log.Printf("CallbackServer: failed to marshal callback response: %s", err)
			return
		}
		w.Write(cbResBytes)
	}
}

// NewCallbackServer runs a local HTTP server that can read callbacks from mitmproxy.
// Automatically listens on a high numbered port. Must be Close()d at the end of the test.
// Register callback handlers via CallbackServer.SetOnRequestCallback and CallbackServer.SetOnResponseCallback
func NewCallbackServer(hostnameRunningComplement string) (*CallbackServer, error) {
	mux := http.NewServeMux()

	// listen on a random high numbered port
	ln, err := net.Listen("tcp", ":0") //nolint
	if err != nil {
		return nil, fmt.Errorf("failed to listen on a tcp port: %s", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: mux,
	}
	go srv.Serve(ln)

	callbackServer := &CallbackServer{
		mux:     mux,
		srv:     srv,
		mu:      &sync.Mutex{},
		baseURL: fmt.Sprintf("http://%s:%d", hostnameRunningComplement, port),
	}
	mux.HandleFunc(requestPath, func(w http.ResponseWriter, r *http.Request) {
		callbackServer.mu.Lock()
		h := callbackServer.onRequest
		callbackServer.mu.Unlock()
		if h == nil {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"no request handler registered"}`))
			return
		}
		h(w, r)
	})
	mux.HandleFunc(responsePath, func(w http.ResponseWriter, r *http.Request) {
		callbackServer.mu.Lock()
		h := callbackServer.onResponse
		callbackServer.mu.Unlock()
		if h == nil {
			w.WriteHeader(404)
			w.Write([]byte(`{"error":"no response handler registered"}`))
			return
		}
		h(w, r)
	})

	return callbackServer, nil
}

// SendError returns a callback.Fn which returns the provided statusCode
// along with a JSON error $count times, after which it lets the response
// pass through. This is useful for testing retries. If count=0, always send
// an error response.
func SendError(count uint32, statusCode int) Fn {
	var seen atomic.Uint32
	return func(d Data) *Response {
		next := seen.Add(1)
		if count > 0 && next > count {
			return nil
		}
		return &Response{
			RespondStatusCode: statusCode,
			RespondBody:       json.RawMessage(`{"error":"callback.SendError"}`),
		}
	}
}
