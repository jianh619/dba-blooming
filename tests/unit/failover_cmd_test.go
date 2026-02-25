package unit_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luckyjian/pgdba/internal/cli"
	"github.com/luckyjian/pgdba/internal/cluster"
)

// executeCmd runs any root command args and returns combined output + error.
func executeCmd(t *testing.T, reg *cluster.Registry, args ...string) (string, error) {
	t.Helper()
	var cmd interface{ SetOut(interface{}); SetErr(interface{}); SetArgs([]string); Execute() error }
	_ = cmd
	// Use NewRootCmdWithRegistry so tests get an isolated registry.
	root := cli.NewRootCmdWithRegistry(registryPath(t))
	buf := new(strings.Builder)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// mockPatroniServer starts a minimal mock Patroni server responding to
// GET /cluster and POST /switchover, POST /failover.
func mockPatroniServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/cluster", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		//nolint:errcheck
		w.Write([]byte(`{
			"scope": "test-cluster",
			"members": [
				{"name":"pg-primary","role":"leader","state":"running","host":"pg-primary","port":5432,"lag":0},
				{"name":"pg-replica-1","role":"replica","state":"running","host":"pg-replica-1","port":5432,"lag":512},
				{"name":"pg-replica-2","role":"replica","state":"running","host":"pg-replica-2","port":5432,"lag":1024}
			]
		}`))
	})

	mux.HandleFunc("/switchover", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/failover", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	return httptest.NewServer(mux)
}

// registryPath returns an isolated temp registry path for each test.
func registryPath(t *testing.T) string {
	t.Helper()
	return t.TempDir() + "/clusters.json"
}

// registerCluster is a helper that runs cluster connect against the mock server.
func registerCluster(t *testing.T, name, patroniURL string) {
	t.Helper()
	out, err := executeCmd(t, nil,
		"cluster", "connect",
		"--name", name,
		"--patroni-url", patroniURL,
		"--pg-host", "localhost",
	)
	if err != nil {
		t.Fatalf("setup: cluster connect failed: %v\n%s", err, out)
	}
}

// ------------------------------------------------------------------ failover

func TestFailoverCmd_HelpAvailable(t *testing.T) {
	out, err := executeCmd(t, nil, "failover", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "failover") {
		t.Errorf("help output should contain 'failover', got:\n%s", out)
	}
}

func TestFailoverTrigger_MissingNameAndURL(t *testing.T) {
	_, err := executeCmd(t, nil, "failover", "trigger")
	if err == nil {
		t.Fatal("expected error when neither --name nor --patroni-url provided")
	}
}

func TestFailoverTrigger_SwitchoverSuccess(t *testing.T) {
	srv := mockPatroniServer(t)
	defer srv.Close()

	out, err := executeCmd(t, nil,
		"failover", "trigger",
		"--patroni-url", srv.URL,
	)
	if err != nil {
		t.Fatalf("failover trigger failed: %v\n%s", err, out)
	}
	var resp map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); jsonErr != nil {
		t.Fatalf("output not valid JSON: %v\n%s", jsonErr, out)
	}
	if success, _ := resp["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", resp)
	}
}

func TestFailoverTrigger_WithExplicitCandidate(t *testing.T) {
	srv := mockPatroniServer(t)
	defer srv.Close()

	out, err := executeCmd(t, nil,
		"failover", "trigger",
		"--patroni-url", srv.URL,
		"--candidate", "pg-replica-1",
	)
	if err != nil {
		t.Fatalf("failover trigger with candidate failed: %v\n%s", err, out)
	}
	var resp map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); jsonErr != nil {
		t.Fatalf("output not valid JSON: %v\n%s", jsonErr, out)
	}
	if success, _ := resp["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", resp)
	}
}

func TestFailoverTrigger_InvalidCandidate(t *testing.T) {
	srv := mockPatroniServer(t)
	defer srv.Close()

	out, err := executeCmd(t, nil,
		"failover", "trigger",
		"--patroni-url", srv.URL,
		"--candidate", "nonexistent-node",
	)
	if err == nil {
		t.Fatal("expected error for nonexistent candidate")
	}
	var resp map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); jsonErr != nil {
		t.Fatalf("output not valid JSON: %v\n%s", jsonErr, out)
	}
	if success, _ := resp["success"].(bool); success {
		t.Error("expected success=false for invalid candidate")
	}
}

func TestFailoverTrigger_ForceFlag(t *testing.T) {
	srv := mockPatroniServer(t)
	defer srv.Close()

	out, err := executeCmd(t, nil,
		"failover", "trigger",
		"--patroni-url", srv.URL,
		"--force",
		"--candidate", "pg-replica-1",
	)
	if err != nil {
		t.Fatalf("failover trigger --force failed: %v\n%s", err, out)
	}
	var resp map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); jsonErr != nil {
		t.Fatalf("output not valid JSON: %v\n%s", jsonErr, out)
	}
	if success, _ := resp["success"].(bool); !success {
		t.Errorf("expected success=true for force failover, got: %v", resp)
	}
}

func TestFailoverStatus_Success(t *testing.T) {
	srv := mockPatroniServer(t)
	defer srv.Close()

	out, err := executeCmd(t, nil,
		"failover", "status",
		"--patroni-url", srv.URL,
	)
	if err != nil {
		t.Fatalf("failover status failed: %v\n%s", err, out)
	}
	var resp map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); jsonErr != nil {
		t.Fatalf("output not valid JSON: %v\n%s", jsonErr, out)
	}
	if success, _ := resp["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", resp)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got: %v", resp)
	}
	if _, ok := data["primary"]; !ok {
		t.Error("data should contain 'primary' field")
	}
}

// ------------------------------------------------------------------ replica

func TestReplicaCmd_HelpAvailable(t *testing.T) {
	out, err := executeCmd(t, nil, "replica", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "replica") {
		t.Errorf("help output should contain 'replica', got:\n%s", out)
	}
}

func TestReplicaList_Success(t *testing.T) {
	srv := mockPatroniServer(t)
	defer srv.Close()

	out, err := executeCmd(t, nil,
		"replica", "list",
		"--patroni-url", srv.URL,
	)
	if err != nil {
		t.Fatalf("replica list failed: %v\n%s", err, out)
	}
	var resp map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); jsonErr != nil {
		t.Fatalf("output not valid JSON: %v\n%s", jsonErr, out)
	}
	if success, _ := resp["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", resp)
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data map, got: %v", resp)
	}
	replicas, ok := data["replicas"].([]interface{})
	if !ok || len(replicas) == 0 {
		t.Errorf("expected non-empty replicas array, got: %v", data)
	}
}

func TestReplicaList_MissingNameAndURL(t *testing.T) {
	_, err := executeCmd(t, nil, "replica", "list")
	if err == nil {
		t.Fatal("expected error when neither --name nor --patroni-url provided")
	}
}

func TestReplicaPromote_MissingCandidate(t *testing.T) {
	srv := mockPatroniServer(t)
	defer srv.Close()

	_, err := executeCmd(t, nil,
		"replica", "promote",
		"--patroni-url", srv.URL,
	)
	if err == nil {
		t.Fatal("expected error when --candidate is missing")
	}
}

func TestReplicaPromote_Success(t *testing.T) {
	srv := mockPatroniServer(t)
	defer srv.Close()

	out, err := executeCmd(t, nil,
		"replica", "promote",
		"--patroni-url", srv.URL,
		"--candidate", "pg-replica-1",
	)
	if err != nil {
		t.Fatalf("replica promote failed: %v\n%s", err, out)
	}
	var resp map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); jsonErr != nil {
		t.Fatalf("output not valid JSON: %v\n%s", jsonErr, out)
	}
	if success, _ := resp["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", resp)
	}
}

func TestReplicaPromote_InvalidCandidate(t *testing.T) {
	srv := mockPatroniServer(t)
	defer srv.Close()

	out, err := executeCmd(t, nil,
		"replica", "promote",
		"--patroni-url", srv.URL,
		"--candidate", "ghost-node",
	)
	if err == nil {
		t.Fatal("expected error for invalid candidate")
	}
	var resp map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(strings.TrimSpace(out)), &resp); jsonErr != nil {
		t.Fatalf("output not valid JSON: %v\n%s", jsonErr, out)
	}
	if success, _ := resp["success"].(bool); success {
		t.Error("expected success=false for invalid candidate")
	}
}
