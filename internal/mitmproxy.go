package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// must match the value in mitmproxy_addons/controller.py
const magicMITMURL = "http://mitm.code"

type Client struct {
	client *http.Client
}

func NewClient(proxyURL *url.URL) *Client {
	return &Client{
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
		},
	}
}

// Lock mitmproxy with the given set of options.
// See https://docs.mitmproxy.org/stable/concepts-options/ for more
// information about options.
func (m *Client) LockOptions(options map[string]any) (lockID []byte, err error) {
	log.Printf("Locking mitmproxy with options %+v\n", options)
	jsonBody, err := json.Marshal(map[string]interface{}{
		"options": options,
	})
	if err != nil {
		return nil, fmt.Errorf("LockOptions: %s", err)
	}
	u := magicMITMURL + "/options/lock"
	req, err := http.NewRequest("POST", u, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("LockOptions: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("LockOptions: %s", err)
	}
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("LockOptions: returned HTTP %v", res.StatusCode)
	}
	lockID, err = io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("LockOptions: %s", err)
	}
	return lockID, nil
}

// Unlock mitmproxy using the lock ID provided.
// See https://docs.mitmproxy.org/stable/concepts-options/ for more information about options.
func (m *Client) UnlockOptions(lockID []byte) error {
	req, err := http.NewRequest("POST", magicMITMURL+"/options/unlock", bytes.NewBuffer(lockID))
	if err != nil {
		return fmt.Errorf("UnlockOptions: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("UnlockOptions: %s", err)
	}
	if res.StatusCode != 200 {
		return fmt.Errorf("UnlockOptions: returned HTTP %v", res.StatusCode)
	}
	log.Println("Unlocking mitmproxy")
	return nil
}
