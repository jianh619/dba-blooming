package unit_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luckyjian/pgdba/internal/patroni"
)

func TestGetClusterStatus_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster" {
			http.NotFound(w, r)
			return
		}
		status := patroni.ClusterStatus{
			Members: []patroni.Member{
				{Name: "pg-primary", Role: "master", State: "running", Host: "10.0.0.1", Port: 5432},
				{Name: "pg-replica-1", Role: "replica", State: "running", Host: "10.0.0.2", Port: 5432, Lag: 0},
			},
		}
		json.NewEncoder(w).Encode(status)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	cs, err := client.GetClusterStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cs.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(cs.Members))
	}
	if cs.Members[0].Role != "master" {
		t.Errorf("expected first member role 'master', got %q", cs.Members[0].Role)
	}
}

func TestGetClusterStatus_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	_, err := client.GetClusterStatus(context.Background())
	if err == nil {
		t.Error("expected error for HTTP 503")
	}
}

func TestGetNodeInfo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/patroni" {
			http.NotFound(w, r)
			return
		}
		info := patroni.NodeInfo{
			State: "running",
			Role:  "master",
		}
		json.NewEncoder(w).Encode(info)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	info, err := client.GetNodeInfo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Role != "master" {
		t.Errorf("expected role 'master', got %q", info.Role)
	}
}

func TestIsPrimary_True(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/primary" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	isPrimary, err := client.IsPrimary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isPrimary {
		t.Error("expected IsPrimary to return true")
	}
}

func TestIsPrimary_False(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // replica returns 404
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	isPrimary, err := client.IsPrimary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isPrimary {
		t.Error("expected IsPrimary to return false for replica")
	}
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster" {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		json.NewEncoder(w).Encode(patroni.ClusterStatus{})
	}))
	defer srv.Close()

	c := patroni.NewClient(srv.URL + "/")
	_, _ = c.GetClusterStatus(context.Background())
}

func TestSwitchover_SendsCorrectRequest(t *testing.T) {
	var receivedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/switchover" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	err := client.Switchover(context.Background(), "pg-primary", "pg-replica-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedBody["leader"] != "pg-primary" {
		t.Errorf("expected leader='pg-primary', got %q", receivedBody["leader"])
	}
	if receivedBody["candidate"] != "pg-replica-1" {
		t.Errorf("expected candidate='pg-replica-1', got %q", receivedBody["candidate"])
	}
}

func TestReinitialize_PostsToCorrectEndpoint(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/reinitialize" {
			called = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	if err := client.Reinitialize(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected POST /reinitialize to be called")
	}
}

func TestGetClusterStatus_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	_, err := client.GetClusterStatus(context.Background())
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestGetNodeInfo_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	_, err := client.GetNodeInfo(context.Background())
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

func TestSwitchover_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "conflict", http.StatusConflict)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	err := client.Switchover(context.Background(), "leader", "candidate")
	if err == nil {
		t.Error("expected error for HTTP 409 conflict")
	}
}

func TestFailover_SendsCorrectRequest(t *testing.T) {
	var receivedBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/failover" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		json.NewDecoder(r.Body).Decode(&receivedBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	err := client.Failover(context.Background(), "pg-replica-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedBody["candidate"] != "pg-replica-1" {
		t.Errorf("expected candidate='pg-replica-1', got %q", receivedBody["candidate"])
	}
}

func TestRestart_PostsToCorrectEndpoint(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/restart" {
			called = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	if err := client.Restart(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected POST /restart to be called")
	}
}

func TestIsPrimary_ServiceUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	isPrimary, err := client.IsPrimary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isPrimary {
		t.Error("expected IsPrimary to return false for 503")
	}
}

// TestGetClusterStatus_StringLag verifies that a string-valued lag field (e.g.
// "unknown") returned by Patroni immediately after a switchover is decoded as 0
// rather than causing a JSON unmarshal error.
func TestGetClusterStatus_StringLag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Raw JSON: leader has no lag field; replica has lag="unknown" (string).
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"members":[
			{"name":"pg-replica-1","role":"leader","state":"running","host":"10.0.0.2","port":5432,"timeline":2},
			{"name":"pg-primary","role":"replica","state":"streaming","host":"10.0.0.1","port":5432,"timeline":2,"lag":"unknown"}
		]}`))
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	cs, err := client.GetClusterStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error with string lag: %v", err)
	}
	if len(cs.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(cs.Members))
	}
	// The replica with lag="unknown" must decode to Lag=0.
	for _, m := range cs.Members {
		if m.Lag != 0 {
			t.Errorf("member %q: expected Lag=0, got %d", m.Name, m.Lag)
		}
	}
}

// TestGetClusterStatus_MissingLag verifies that a missing lag field (as Patroni
// omits it for the leader) decodes to 0 without error.
func TestGetClusterStatus_MissingLag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"members":[
			{"name":"pg-primary","role":"leader","state":"running","host":"10.0.0.1","port":5432,"timeline":1}
		]}`))
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	cs, err := client.GetClusterStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error with missing lag: %v", err)
	}
	if cs.Members[0].Lag != 0 {
		t.Errorf("expected Lag=0 for leader, got %d", cs.Members[0].Lag)
	}
}

func TestClusterStatus_PauseField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/cluster" {
			http.NotFound(w, r)
			return
		}
		status := patroni.ClusterStatus{
			Pause: true,
		}
		json.NewEncoder(w).Encode(status)
	}))
	defer srv.Close()

	client := patroni.NewClient(srv.URL)
	cs, err := client.GetClusterStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cs.Pause {
		t.Error("expected Pause field to be true")
	}
}
