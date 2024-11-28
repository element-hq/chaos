package internal

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
)

var adjectives = []string{
	"angry",
	"beautiful",
	"crazy",
	"dangerous",
	"evil",
	"funny",
	"glum",
	"happy",
	"indignant",
	"jovial",
	"kingly",
	"lucky",
	"majestic",
	"naive",
}

var nouns = []string{
	"aardvark",
	"bus",
	"chimp",
	"dream",
	"engine",
	"fridge",
	"goose",
	"house",
	"island",
	"jail",
	"kryptonite",
	"lozenge",
	"mansion",
	"nightmare",
}

type CSAPI struct {
	UserID      string
	AccessToken string
	BaseURL     string
	Domain      string
	Client      *http.Client
	Debug       bool
	counter     int
}

func (c *CSAPI) Register(localpart string) error {
	res, err := c.Do("POST", []string{"_matrix", "client", "v3", "register"}, WithJSONBody(map[string]any{
		"auth": map[string]any{
			"type": "m.login.dummy",
		},
		"username": localpart,
		"password": "loadtestingisfun",
	}))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	r := struct {
		UserID      string `json:"user_id"`
		AccessToken string `json:"access_token"`
		HS          string `json:"home_server"`
	}{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return fmt.Errorf("Register: failed to read response body: %s", err)
	}
	if r.HS != c.Domain {
		return fmt.Errorf(
			"Register: response is for domain '%s' but we thought we were registering on domain '%s'", r.HS, c.Domain,
		)
	}
	c.AccessToken = r.AccessToken
	c.UserID = r.UserID
	return nil
}

func (c *CSAPI) CreateRoom(body map[string]interface{}) (roomID string, err error) {
	res, err := c.Do("POST", []string{"_matrix", "client", "v3", "createRoom"}, WithJSONBody(body))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	r := struct {
		RoomID string `json:"room_id"`
	}{}
	if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
		return "", fmt.Errorf("CreateRoom: failed to read response body: %s", err)
	}
	return r.RoomID, nil
}

func (c *CSAPI) JoinRoom(roomIDOrAlias string, serverNames []string) error {
	// construct URL query parameters
	query := make(url.Values, len(serverNames))
	for _, serverName := range serverNames {
		query.Add("server_name", serverName)
	}
	// join the room
	_, err := c.Do(
		"POST", []string{"_matrix", "client", "v3", "join", roomIDOrAlias},
		WithQueries(query), WithJSONBody(map[string]interface{}{}),
	)
	return err
}

func (c *CSAPI) LeaveRoom(roomID string) error {
	// leave the room
	body := map[string]interface{}{}
	_, err := c.Do("POST", []string{"_matrix", "client", "v3", "rooms", roomID, "leave"}, WithJSONBody(body))
	return err
}

func (c *CSAPI) SendMessage(roomID string) error {
	msg := fmt.Sprintf("%s %s", adjectives[rand.Intn(len(adjectives))], nouns[rand.Intn(len(nouns))])
	_, err := c.SendMessageWithText(roomID, msg)
	return err
}

func (c *CSAPI) SendMessageWithText(roomID string, text string) (string, error) {
	c.counter++
	paths := []string{"_matrix", "client", "v3", "rooms", roomID, "send", "m.room.message", fmt.Sprintf("%d", c.counter)}
	res, err := c.Do("PUT", paths, WithJSONBody(struct {
		MsgType string `json:"msgtype"`
		Body    string `json:"body"`
	}{
		MsgType: "m.text",
		Body:    text,
	}))
	if err != nil {
		return "", err
	}
	body := struct {
		EventID string `json:"event_id"`
	}{}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("failed to decode response body: %s", err)
	}
	return body.EventID, nil
}

type Event struct {
	StateKey    *string                `json:"state_key,omitempty"`    // The state key for the event. Only present on State Events.
	Sender      string                 `json:"sender"`                 // The user ID of the sender of the event
	Type        string                 `json:"type"`                   // The event type
	Timestamp   int64                  `json:"origin_server_ts"`       // The unix timestamp when this message was sent by the origin server
	ID          string                 `json:"event_id"`               // The unique ID of this event
	RoomID      string                 `json:"room_id"`                // The room the event was sent to. May be nil (e.g. for presence)
	Redacts     string                 `json:"redacts,omitempty"`      // The event ID that was redacted if a m.room.redaction event
	Unsigned    map[string]interface{} `json:"unsigned"`               // The unsigned portions of the event, such as age and prev_content
	Content     map[string]interface{} `json:"content"`                // The JSON content of the event.
	PrevContent map[string]interface{} `json:"prev_content,omitempty"` // The JSON prev_content of the event.
}

// SyncReq contains all the /sync request configuration options. The empty struct `SyncReq{}` is valid
// which will do a full /sync due to lack of a since token.
type SyncReq struct {
	// A point in time to continue a sync from. This should be the next_batch token returned by an
	// earlier call to this endpoint.
	Since string
	// The ID of a filter created using the filter API or a filter JSON object encoded as a string.
	// The server will detect whether it is an ID or a JSON object by whether the first character is
	// a "{" open brace. Passing the JSON inline is best suited to one off requests. Creating a
	// filter using the filter API is recommended for clients that reuse the same filter multiple
	// times, for example in long poll requests.
	Filter string
	// Controls whether to include the full state for all rooms the user is a member of.
	// If this is set to true, then all state events will be returned, even if since is non-empty.
	// The timeline will still be limited by the since parameter. In this case, the timeout parameter
	// will be ignored and the query will return immediately, possibly with an empty timeline.
	// If false, and since is non-empty, only state which has changed since the point indicated by
	// since will be returned.
	// By default, this is false.
	FullState bool
	// Controls whether the client is automatically marked as online by polling this API. If this
	// parameter is omitted then the client is automatically marked as online when it uses this API.
	// Otherwise if the parameter is set to “offline” then the client is not marked as being online
	// when it uses this API. When set to “unavailable”, the client is marked as being idle.
	// One of: [offline online unavailable].
	SetPresence string
	// The maximum time to wait, in milliseconds, before returning this request. If no events
	// (or other data) become available before this time elapses, the server will return a response
	// with empty fields.
	// By default, this is 1000 for Complement testing.
	TimeoutMillis string // string for easier conversion to query params
}

type SyncResponse struct {
	NextBatch   string `json:"next_batch"`
	AccountData struct {
		Events []Event `json:"events"`
	} `json:"account_data"`
	Presence struct {
		Events []Event `json:"events"`
	} `json:"presence"`
	Rooms struct {
		Leave map[string]struct {
			State struct {
				Events []Event `json:"events"`
			} `json:"state"`
			Timeline struct {
				Events    []Event `json:"events"`
				Limited   bool    `json:"limited"`
				PrevBatch string  `json:"prev_batch"`
			} `json:"timeline"`
		} `json:"leave"`
		Join map[string]struct {
			State struct {
				Events []Event `json:"events"`
			} `json:"state"`
			Timeline struct {
				Events    []Event `json:"events"`
				Limited   bool    `json:"limited"`
				PrevBatch string  `json:"prev_batch"`
			} `json:"timeline"`
			Ephemeral struct {
				Events []Event `json:"events"`
			} `json:"ephemeral"`
		} `json:"join"`
		Invite map[string]struct {
			State struct {
				Events []Event
			} `json:"invite_state"`
		} `json:"invite"`
	} `json:"rooms"`
}

func (c *CSAPI) Sync(syncReq SyncReq) (*SyncResponse, error) {
	query := url.Values{
		"timeout": []string{"1000"},
	}
	// configure the HTTP request based on SyncReq
	if syncReq.TimeoutMillis != "" {
		query["timeout"] = []string{syncReq.TimeoutMillis}
	}
	if syncReq.Since != "" {
		query["since"] = []string{syncReq.Since}
	}
	if syncReq.Filter != "" {
		query["filter"] = []string{syncReq.Filter}
	}
	if syncReq.FullState {
		query["full_state"] = []string{"true"}
	}
	if syncReq.SetPresence != "" {
		query["set_presence"] = []string{syncReq.SetPresence}
	}
	res, err := c.Do("GET", []string{"_matrix", "client", "v3", "sync"}, WithQueries(query))
	if err != nil {
		return nil, fmt.Errorf("Sync failed: %s", err)
	}
	var sr SyncResponse
	if err := json.NewDecoder(res.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("Sync response decoding: %s", err)
	}
	return &sr, nil
}

func (c *CSAPI) Members(roomID string) ([]Event, error) {
	res, err := c.Do("GET", []string{"_matrix", "client", "v3", "rooms", roomID, "members"})
	if err != nil {
		return nil, fmt.Errorf("Members failed: %s", err)
	}
	body := struct {
		Chunk []Event `json:"chunk"`
	}{}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("Members response decoding: %s", err)
	}
	return body.Chunk, nil
}

func (c *CSAPI) Event(roomID, eventID string) (*Event, error) {
	res, err := c.Do("GET", []string{"_matrix", "client", "v3", "rooms", roomID, "event", eventID})
	if err != nil {
		return nil, fmt.Errorf("Event failed: %s", err)
	}
	body := &Event{}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("Members response decoding: %s", err)
	}
	return body, nil
}

func (c *CSAPI) Do(method string, paths []string, opts ...RequestOpt) (*http.Response, error) {
	for i := range paths {
		paths[i] = url.PathEscape(paths[i])
	}
	reqURL := c.BaseURL + "/" + strings.Join(paths, "/")
	req, err := http.NewRequest(method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("CSAPI.Do failed to create http.NewRequest: %s", err)
	}
	// set defaults before RequestOpts
	if c.AccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	}
	// set functional options
	for _, o := range opts {
		o(req)
	}
	// set defaults after RequestOpts
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	// debug log the request
	if c.Debug {
		var bodyStr string
		contentType := req.Header.Get("Content-Type")
		if contentType == "application/json" || strings.HasPrefix(contentType, "text/") {
			if req.Body != nil {
				body, _ := io.ReadAll(req.Body)
				bodyStr = fmt.Sprintf("Request body: %s", string(body))
				req.Body = io.NopCloser(bytes.NewBuffer(body))
			}
		} else {
			bodyStr = fmt.Sprintf("Request body: <binary:%s>", contentType)
		}
		log.Printf("%s : %s %s %s", c.UserID, method, req.URL.Path, bodyStr)
	}
	// Perform the HTTP request
	res, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CSAPI.Do response returned error: %s", err)
	}
	// debug log the response
	if c.Debug && res != nil {
		/*
			var dump []byte
			dump, err = httputil.DumpResponse(res, true)
			if err != nil {
				return nil, fmt.Errorf("CSAPI.Do failed to dump response body: %s", err)
			}
			log.Printf("%s", string(dump)) */
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("CSAPI.Do %s failed with response code HTTP %d : %s", req.URL.String(), res.StatusCode, string(body))
	}
	return res, nil
}

// RequestOpt is a functional option which will modify an outgoing HTTP request.
// See functions starting with `With...` in this package for more info.
type RequestOpt func(req *http.Request)

// WithContentType sets the HTTP request Content-Type header to `cType`
func WithContentType(cType string) RequestOpt {
	return func(req *http.Request) {
		req.Header.Set("Content-Type", cType)
	}
}

// WithJSONBody sets the HTTP request body to the JSON serialised form of `obj`
func WithJSONBody(obj interface{}) RequestOpt {
	return func(req *http.Request) {
		b, err := json.Marshal(obj)
		if err != nil {
			log.Fatalf("CSAPI.Do failed to marshal JSON body: %s", err)
		}
		WithRawBody(b)(req)
	}
}

// WithQueries sets the query parameters on the request.
// This function should not be used to set an "access_token" parameter for Matrix authentication.
// Instead, set CSAPI.AccessToken.
func WithQueries(q url.Values) RequestOpt {
	return func(req *http.Request) {
		req.URL.RawQuery = q.Encode()
	}
}

// WithRawBody sets the HTTP request body to `body`
func WithRawBody(body []byte) RequestOpt {
	return func(req *http.Request) {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) {
			r := bytes.NewReader(body)
			return io.NopCloser(r), nil
		}
		// we need to manually set this because we don't set the body
		// in http.NewRequest due to using functional options, and only in NewRequest
		// does the stdlib set this for us.
		req.ContentLength = int64(len(body))
	}
}

// Maps *.localhost to 127.0.0.1 like curl. Go doesn't do this by default.
type LocalhostRoundTripper struct{}

func (t *LocalhostRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	hostname := req.URL.Hostname()
	if strings.HasSuffix(hostname, ".localhost") {
		req.URL.Host = "localhost"
	}
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	return transport.RoundTrip(req)
}
