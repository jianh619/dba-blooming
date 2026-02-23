package patroni

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// NodeState represents the Patroni node state.
type NodeState string

const (
	StateRunning  NodeState = "running"
	StateStopped  NodeState = "stopped"
	StateCreating NodeState = "creating replica"
)

// Member represents a single cluster member node.
type Member struct {
	Name     string    `json:"name"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	Role     string    `json:"role"`
	State    NodeState `json:"state"`
	Lag      int64     `json:"lag"`
	Timeline int64     `json:"timeline"`
	APIURL   string    `json:"api_url"`
}

// ClusterStatus holds the Patroni cluster topology.
type ClusterStatus struct {
	Members  []Member `json:"members"`
	Failover *string  `json:"failover,omitempty"`
	Pause    bool     `json:"pause"`
}

// NodeInfo holds detailed information about a single Patroni node (from /patroni endpoint).
type NodeInfo struct {
	State         NodeState `json:"state"`
	Role          string    `json:"role"`
	ServerVersion int       `json:"server_version"`
	ClusterName   string    `json:"patroni"`
	Timeline      int64     `json:"timeline"`
	Xlog          struct {
		Location int64 `json:"location"`
	} `json:"xlog"`
}

// Client is a Patroni REST API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Patroni client.
// baseURL example: "http://10.0.0.1:8008"
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// GetClusterStatus queries the cluster topology (GET /cluster).
func (c *Client) GetClusterStatus(ctx context.Context) (*ClusterStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/cluster", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get cluster status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("patroni /cluster returned HTTP %d", resp.StatusCode)
	}

	var status ClusterStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode cluster status: %w", err)
	}
	return &status, nil
}

// GetNodeInfo queries a single node's details (GET /patroni).
func (c *Client) GetNodeInfo(ctx context.Context) (*NodeInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/patroni", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get node info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("patroni /patroni returned HTTP %d", resp.StatusCode)
	}

	var info NodeInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode node info: %w", err)
	}
	return &info, nil
}

// IsPrimary checks if the current node is the primary (GET /primary → 200 means primary).
func (c *Client) IsPrimary(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/primary", nil)
	if err != nil {
		return false, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("check primary: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// Switchover triggers a controlled switchover (POST /switchover).
// leader is the current primary name; candidate is the target node (empty = Patroni chooses).
func (c *Client) Switchover(ctx context.Context, leader, candidate string) error {
	payload := map[string]string{
		"leader":    leader,
		"candidate": candidate,
	}
	return c.postJSON(ctx, "/switchover", payload)
}

// Failover triggers a forced failover (POST /failover) for use when the primary is unreachable.
func (c *Client) Failover(ctx context.Context, candidate string) error {
	payload := map[string]string{
		"candidate": candidate,
	}
	return c.postJSON(ctx, "/failover", payload)
}

// Reinitialize re-initializes the node (POST /reinitialize).
func (c *Client) Reinitialize(ctx context.Context) error {
	return c.postJSON(ctx, "/reinitialize", nil)
}

// Restart restarts the Patroni-managed PostgreSQL (POST /restart).
func (c *Client) Restart(ctx context.Context) error {
	return c.postJSON(ctx, "/restart", nil)
}

// postJSON sends a POST request with an optional JSON body and checks for a 2xx response.
func (c *Client) postJSON(ctx context.Context, path string, body interface{}) error {
	var bodyReader *bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("patroni %s returned HTTP %d", path, resp.StatusCode)
	}
	return nil
}
