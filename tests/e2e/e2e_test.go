// Package e2e contains black-box end-to-end tests for the pgdba binary.
// Tests compile the binary once via TestMain, then exercise it through
// os/exec — no internal packages are imported.
package e2e_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// binaryPath is set once in TestMain.
var binaryPath string

// TestMain builds the pgdba binary before running any tests.
func TestMain(m *testing.M) {
	// Resolve project root (two levels up from tests/e2e/).
	root, err := filepath.Abs("../..")
	if err != nil {
		panic("cannot determine project root: " + err.Error())
	}

	bin := filepath.Join(root, "bin", "pgdba")
	build := exec.Command("go", "build", "-o", bin, "./cmd/pgdba")
	build.Dir = root
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("failed to build pgdba: " + err.Error())
	}
	binaryPath = bin

	os.Exit(m.Run())
}

// run executes the pgdba binary with the given arguments and environment.
// It returns stdout, stderr, and the combined exit error (nil = exit 0).
func run(t *testing.T, env []string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), env...)

	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// homeEnv returns HOME=<tempdir> so each test gets an isolated cluster registry.
func homeEnv(t *testing.T) []string {
	t.Helper()
	return []string{"HOME=" + t.TempDir()}
}

// assertJSON parses s as JSON and returns the top-level map.
// Fails the test immediately if s is not valid JSON.
func assertJSON(t *testing.T, s string) map[string]interface{} {
	t.Helper()
	s = strings.TrimSpace(s)
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("output is not valid JSON:\n%s\nerror: %v", s, err)
	}
	return m
}

// assertEnvelopeFields checks that the JSON map contains the mandatory
// response envelope keys: success, timestamp, command.
func assertEnvelopeFields(t *testing.T, m map[string]interface{}) {
	t.Helper()
	for _, key := range []string{"success", "timestamp", "command"} {
		if _, ok := m[key]; !ok {
			t.Errorf("JSON envelope missing required field %q: %v", key, m)
		}
	}
}

// -----------------------------------------------------------------
// Group 1: help / metadata (no external services required)
// -----------------------------------------------------------------

func TestHelp_RootCommand(t *testing.T) {
	stdout, _, err := run(t, homeEnv(t), "--help")
	if err != nil {
		t.Fatalf("--help exited with error: %v", err)
	}
	if !strings.Contains(stdout, "pgdba") {
		t.Errorf("--help output does not mention 'pgdba':\n%s", stdout)
	}
	if !strings.Contains(stdout, "PostgreSQL") {
		t.Errorf("--help output does not mention 'PostgreSQL':\n%s", stdout)
	}
}

func TestHelp_HealthCheck(t *testing.T) {
	_, _, err := run(t, homeEnv(t), "health", "check", "--help")
	if err != nil {
		t.Fatalf("health check --help exited with error: %v", err)
	}
}

func TestHelp_ClusterCommands(t *testing.T) {
	for _, sub := range []string{"status", "connect", "init", "destroy"} {
		sub := sub
		t.Run(sub, func(t *testing.T) {
			_, _, err := run(t, homeEnv(t), "cluster", sub, "--help")
			if err != nil {
				t.Fatalf("cluster %s --help exited with error: %v", sub, err)
			}
		})
	}
}

// -----------------------------------------------------------------
// Group 2: output format validation
// -----------------------------------------------------------------

func TestInvalidFormat_ReturnsError(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t), "--format", "bad", "health", "check")
	if err == nil {
		t.Fatal("expected non-zero exit for invalid --format, got nil")
	}
	// The error is surfaced before any command runs; cobra prints to stderr.
	combined := stderr
	if !strings.Contains(combined, "invalid format") && !strings.Contains(combined, "bad") {
		t.Errorf("expected error mentioning invalid format, got:\n%s", combined)
	}
}

func TestFormat_JSONIsDefault(t *testing.T) {
	// health check will fail (no PG), but the error response must be JSON.
	_, stderr, _ := run(t, homeEnv(t), "health", "check")
	// Failure output goes to stderr.
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false for health check with no PG")
	}
}

func TestFormat_TableOnError(t *testing.T) {
	_, stderr, _ := run(t, homeEnv(t), "--format", "table", "health", "check")
	// Table format: must contain STATUS and COMMAND headers.
	if !strings.Contains(stderr, "STATUS") || !strings.Contains(stderr, "COMMAND") {
		t.Errorf("table output missing STATUS/COMMAND headers:\n%s", stderr)
	}
}

func TestFormat_YAMLOnError(t *testing.T) {
	_, stderr, _ := run(t, homeEnv(t), "--format", "yaml", "health", "check")
	// YAML: must contain the key 'success' in YAML form.
	if !strings.Contains(stderr, "success:") {
		t.Errorf("yaml output does not contain 'success:' key:\n%s", stderr)
	}
}

// -----------------------------------------------------------------
// Group 3: cluster argument validation (no external services)
// -----------------------------------------------------------------

func TestClusterInit_MissingPrimaryHost(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t), "cluster", "init", "--name", "test")
	if err == nil {
		t.Fatal("expected non-zero exit when --primary-host is missing")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false")
	}
}

func TestClusterInit_MissingName(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t), "cluster", "init", "--primary-host", "localhost")
	if err == nil {
		t.Fatal("expected non-zero exit when --name is missing")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
}

func TestClusterConnect_MissingName(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t),
		"cluster", "connect", "--patroni-url", "http://localhost:8008", "--pg-host", "localhost")
	if err == nil {
		t.Fatal("expected non-zero exit when --name is missing")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
}

func TestClusterConnect_MissingPatroniURL(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t),
		"cluster", "connect", "--name", "test", "--pg-host", "localhost")
	if err == nil {
		t.Fatal("expected non-zero exit when --patroni-url is missing")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
}

func TestClusterConnect_MissingPGHost(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t),
		"cluster", "connect", "--name", "test", "--patroni-url", "http://localhost:8008")
	if err == nil {
		t.Fatal("expected non-zero exit when --pg-host is missing")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
}

func TestClusterDestroy_MissingConfirm(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t), "cluster", "destroy", "--name", "test")
	if err == nil {
		t.Fatal("expected non-zero exit when --confirm is missing")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if errMsg, _ := m["error"].(string); !strings.Contains(errMsg, "confirm") {
		t.Errorf("error should mention 'confirm', got: %s", errMsg)
	}
}

func TestClusterStatus_NoNameAndNoURL(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t), "cluster", "status")
	if err == nil {
		t.Fatal("expected non-zero exit when neither --name nor --patroni-url provided")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
}

// -----------------------------------------------------------------
// Group 4: cluster commands with mock Patroni (full lifecycle)
// -----------------------------------------------------------------

// mockPatroniCluster starts a minimal Patroni REST API mock server.
func mockPatroniCluster(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/cluster", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		//nolint:errcheck
		w.Write([]byte(`{
			"scope": "e2e-cluster",
			"members": [
				{"name": "pg-primary", "role": "leader", "state": "running",
				 "api_url": "http://pg-primary:8008/patroni", "host": "pg-primary", "port": 5432, "lag": 0},
				{"name": "pg-replica-1", "role": "replica", "state": "running",
				 "api_url": "http://pg-replica-1:8009/patroni", "host": "pg-replica-1", "port": 5432, "lag": 0}
			]
		}`))
	})
	return httptest.NewServer(mux)
}

func TestClusterConnect_Success(t *testing.T) {
	srv := mockPatroniCluster(t)
	defer srv.Close()

	env := homeEnv(t)
	stdout, _, err := run(t, env,
		"cluster", "connect",
		"--name", "e2e-cluster",
		"--patroni-url", srv.URL,
		"--pg-host", "localhost",
	)
	if err != nil {
		t.Fatalf("cluster connect failed: %v", err)
	}
	m := assertJSON(t, stdout)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", m)
	}
}

func TestClusterStatus_WithPatroniURL(t *testing.T) {
	srv := mockPatroniCluster(t)
	defer srv.Close()

	stdout, _, err := run(t, homeEnv(t),
		"cluster", "status",
		"--patroni-url", srv.URL,
	)
	if err != nil {
		t.Fatalf("cluster status failed: %v", err)
	}
	m := assertJSON(t, stdout)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", m)
	}
	// Verify data contains members.
	data, ok := m["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'data' object in response, got: %v", m)
	}
	members, ok := data["members"].([]interface{})
	if !ok || len(members) == 0 {
		t.Errorf("expected non-empty members array in data, got: %v", data)
	}
}

func TestClusterStatus_FromRegistry(t *testing.T) {
	srv := mockPatroniCluster(t)
	defer srv.Close()

	env := homeEnv(t)

	// First: register the cluster.
	_, _, err := run(t, env,
		"cluster", "connect",
		"--name", "reg-cluster",
		"--patroni-url", srv.URL,
		"--pg-host", "localhost",
	)
	if err != nil {
		t.Fatalf("cluster connect failed: %v", err)
	}

	// Then: query by name (should look up URL from registry).
	stdout, _, err := run(t, env,
		"cluster", "status",
		"--name", "reg-cluster",
	)
	if err != nil {
		t.Fatalf("cluster status by name failed: %v", err)
	}
	m := assertJSON(t, stdout)
	if success, _ := m["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", m)
	}
}

func TestClusterDestroy_RefusesExternal(t *testing.T) {
	srv := mockPatroniCluster(t)
	defer srv.Close()

	env := homeEnv(t)

	// Register as external.
	_, _, err := run(t, env,
		"cluster", "connect",
		"--name", "ext-cluster",
		"--patroni-url", srv.URL,
		"--pg-host", "localhost",
	)
	if err != nil {
		t.Fatalf("setup: cluster connect failed: %v", err)
	}

	// Destroy should be refused for external clusters.
	_, stderr, err := run(t, env,
		"cluster", "destroy",
		"--name", "ext-cluster",
		"--confirm",
	)
	if err == nil {
		t.Fatal("expected destroy to fail for external cluster")
	}
	m := assertJSON(t, stderr)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false")
	}
	if errMsg, _ := m["error"].(string); !strings.Contains(errMsg, "connected") {
		t.Errorf("error should mention 'connected', got: %s", errMsg)
	}
}

func TestClusterStatus_UnknownName(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t), "cluster", "status", "--name", "ghost")
	if err == nil {
		t.Fatal("expected non-zero exit for unknown cluster name")
	}
	m := assertJSON(t, stderr)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false")
	}
}

// mockPatroniFull starts a Patroni mock that also handles POST /switchover
// and POST /failover, for testing Phase 3 commands.
func mockPatroniFull(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/cluster", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		//nolint:errcheck
		w.Write([]byte(`{
			"scope": "e2e-cluster",
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

// -----------------------------------------------------------------
// Group 6: failover / replica help (no external services)
// -----------------------------------------------------------------

func TestHelp_FailoverCommands(t *testing.T) {
	for _, sub := range []string{"trigger", "status"} {
		sub := sub
		t.Run(sub, func(t *testing.T) {
			stdout, _, err := run(t, homeEnv(t), "failover", sub, "--help")
			if err != nil {
				t.Fatalf("failover %s --help exited with error: %v", sub, err)
			}
			if !strings.Contains(stdout, sub) {
				t.Errorf("failover %s --help does not mention %q:\n%s", sub, sub, stdout)
			}
		})
	}
}

func TestHelp_ReplicaCommands(t *testing.T) {
	for _, sub := range []string{"list", "promote"} {
		sub := sub
		t.Run(sub, func(t *testing.T) {
			stdout, _, err := run(t, homeEnv(t), "replica", sub, "--help")
			if err != nil {
				t.Fatalf("replica %s --help exited with error: %v", sub, err)
			}
			if !strings.Contains(stdout, sub) {
				t.Errorf("replica %s --help does not mention %q:\n%s", sub, sub, stdout)
			}
		})
	}
}

// -----------------------------------------------------------------
// Group 7: failover / replica argument validation (no external services)
// -----------------------------------------------------------------

func TestFailoverTrigger_MissingNameAndURL(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t), "failover", "trigger")
	if err == nil {
		t.Fatal("expected non-zero exit when neither --name nor --patroni-url provided")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false")
	}
}

func TestFailoverStatus_MissingNameAndURL(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t), "failover", "status")
	if err == nil {
		t.Fatal("expected non-zero exit when neither --name nor --patroni-url provided")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false")
	}
}

func TestReplicaList_MissingNameAndURL(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t), "replica", "list")
	if err == nil {
		t.Fatal("expected non-zero exit when neither --name nor --patroni-url provided")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false")
	}
}

func TestReplicaPromote_MissingNameAndURL(t *testing.T) {
	_, stderr, err := run(t, homeEnv(t), "replica", "promote", "--candidate", "pg-replica-1")
	if err == nil {
		t.Fatal("expected non-zero exit when neither --name nor --patroni-url provided")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false")
	}
}

func TestReplicaPromote_MissingCandidate(t *testing.T) {
	srv := mockPatroniFull(t)
	defer srv.Close()

	_, stderr, err := run(t, homeEnv(t),
		"replica", "promote",
		"--patroni-url", srv.URL,
	)
	if err == nil {
		t.Fatal("expected non-zero exit when --candidate is missing")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false")
	}
	if errMsg, _ := m["error"].(string); !strings.Contains(errMsg, "candidate") {
		t.Errorf("error should mention 'candidate', got: %s", errMsg)
	}
}

func TestFailoverTrigger_ForceRequiresCandidate(t *testing.T) {
	srv := mockPatroniFull(t)
	defer srv.Close()

	_, stderr, err := run(t, homeEnv(t),
		"failover", "trigger",
		"--patroni-url", srv.URL,
		"--force",
	)
	if err == nil {
		t.Fatal("expected non-zero exit when --force used without --candidate")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false")
	}
}

// -----------------------------------------------------------------
// Group 8: failover / replica with mock Patroni server
// -----------------------------------------------------------------

func TestFailoverStatus_Success(t *testing.T) {
	srv := mockPatroniFull(t)
	defer srv.Close()

	stdout, _, err := run(t, homeEnv(t),
		"failover", "status",
		"--patroni-url", srv.URL,
	)
	if err != nil {
		t.Fatalf("failover status failed: %v", err)
	}
	m := assertJSON(t, stdout)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", m)
	}
	data, ok := m["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'data' object, got: %v", m)
	}
	if _, ok := data["primary"]; !ok {
		t.Error("data must contain 'primary' field")
	}
	if _, ok := data["replicas"]; !ok {
		t.Error("data must contain 'replicas' field")
	}
	if _, ok := data["member_count"]; !ok {
		t.Error("data must contain 'member_count' field")
	}
}

func TestFailoverTrigger_AutoCandidate(t *testing.T) {
	srv := mockPatroniFull(t)
	defer srv.Close()

	// No --candidate: pgdba should auto-select the replica with lowest lag.
	stdout, _, err := run(t, homeEnv(t),
		"failover", "trigger",
		"--patroni-url", srv.URL,
	)
	if err != nil {
		t.Fatalf("failover trigger (auto-candidate) failed: %v", err)
	}
	m := assertJSON(t, stdout)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", m)
	}
	data, ok := m["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'data' object, got: %v", m)
	}
	if switchType, _ := data["type"].(string); switchType != "switchover" {
		t.Errorf("expected type=switchover, got: %s", switchType)
	}
	// Auto-selection should pick pg-replica-1 (lag 512 < 1024).
	if to, _ := data["to"].(string); to != "pg-replica-1" {
		t.Errorf("expected to=pg-replica-1 (lowest lag), got: %s", to)
	}
}

func TestFailoverTrigger_ExplicitCandidate(t *testing.T) {
	srv := mockPatroniFull(t)
	defer srv.Close()

	stdout, _, err := run(t, homeEnv(t),
		"failover", "trigger",
		"--patroni-url", srv.URL,
		"--candidate", "pg-replica-2",
	)
	if err != nil {
		t.Fatalf("failover trigger with explicit candidate failed: %v", err)
	}
	m := assertJSON(t, stdout)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", m)
	}
	data, _ := m["data"].(map[string]interface{})
	if to, _ := data["to"].(string); to != "pg-replica-2" {
		t.Errorf("expected to=pg-replica-2, got: %s", to)
	}
}

func TestFailoverTrigger_InvalidCandidate(t *testing.T) {
	srv := mockPatroniFull(t)
	defer srv.Close()

	_, stderr, err := run(t, homeEnv(t),
		"failover", "trigger",
		"--patroni-url", srv.URL,
		"--candidate", "ghost-node",
	)
	if err == nil {
		t.Fatal("expected non-zero exit for nonexistent candidate")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false for invalid candidate")
	}
}

func TestFailoverTrigger_ForcedFailover(t *testing.T) {
	srv := mockPatroniFull(t)
	defer srv.Close()

	stdout, _, err := run(t, homeEnv(t),
		"failover", "trigger",
		"--patroni-url", srv.URL,
		"--force",
		"--candidate", "pg-replica-1",
	)
	if err != nil {
		t.Fatalf("failover trigger --force failed: %v", err)
	}
	m := assertJSON(t, stdout)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", m)
	}
	data, _ := m["data"].(map[string]interface{})
	if switchType, _ := data["type"].(string); switchType != "failover" {
		t.Errorf("expected type=failover for --force, got: %s", switchType)
	}
}

func TestReplicaList_Success(t *testing.T) {
	srv := mockPatroniFull(t)
	defer srv.Close()

	stdout, _, err := run(t, homeEnv(t),
		"replica", "list",
		"--patroni-url", srv.URL,
	)
	if err != nil {
		t.Fatalf("replica list failed: %v", err)
	}
	m := assertJSON(t, stdout)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", m)
	}
	data, ok := m["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'data' object, got: %v", m)
	}
	replicas, ok := data["replicas"].([]interface{})
	if !ok || len(replicas) == 0 {
		t.Errorf("expected non-empty replicas array, got: %v", data)
	}
	// Verify each replica has expected fields.
	for i, r := range replicas {
		row, ok := r.(map[string]interface{})
		if !ok {
			t.Errorf("replica[%d] is not an object: %v", i, r)
			continue
		}
		for _, field := range []string{"name", "state", "lag_bytes", "host", "port"} {
			if _, ok := row[field]; !ok {
				t.Errorf("replica[%d] missing field %q: %v", i, field, row)
			}
		}
	}
	if count, _ := data["count"].(float64); int(count) != len(replicas) {
		t.Errorf("count field (%v) does not match replicas length (%d)", count, len(replicas))
	}
}

func TestReplicaPromote_Success(t *testing.T) {
	srv := mockPatroniFull(t)
	defer srv.Close()

	stdout, _, err := run(t, homeEnv(t),
		"replica", "promote",
		"--patroni-url", srv.URL,
		"--candidate", "pg-replica-1",
	)
	if err != nil {
		t.Fatalf("replica promote failed: %v", err)
	}
	m := assertJSON(t, stdout)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); !success {
		t.Errorf("expected success=true, got: %v", m)
	}
	data, ok := m["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected 'data' object, got: %v", m)
	}
	if candidate, _ := data["candidate"].(string); candidate != "pg-replica-1" {
		t.Errorf("expected candidate=pg-replica-1, got: %s", candidate)
	}
	if status, _ := data["status"].(string); status != "promoted" {
		t.Errorf("expected status=promoted, got: %s", status)
	}
}

func TestReplicaPromote_InvalidCandidate(t *testing.T) {
	srv := mockPatroniFull(t)
	defer srv.Close()

	_, stderr, err := run(t, homeEnv(t),
		"replica", "promote",
		"--patroni-url", srv.URL,
		"--candidate", "ghost-node",
	)
	if err == nil {
		t.Fatal("expected non-zero exit for invalid candidate")
	}
	m := assertJSON(t, stderr)
	assertEnvelopeFields(t, m)
	if success, _ := m["success"].(bool); success {
		t.Error("expected success=false for invalid candidate")
	}
}

// -----------------------------------------------------------------
// Group 5: JSON envelope contract
// -----------------------------------------------------------------

func TestJSONEnvelope_FailureHasErrorField(t *testing.T) {
	_, stderr, _ := run(t, homeEnv(t), "cluster", "init", "--name", "x", "--primary-host", "h")
	m := assertJSON(t, stderr)
	if _, ok := m["error"]; !ok {
		t.Errorf("failure response must contain 'error' field, got: %v", m)
	}
}

func TestJSONEnvelope_SuccessHasDataField(t *testing.T) {
	srv := mockPatroniCluster(t)
	defer srv.Close()

	stdout, _, err := run(t, homeEnv(t),
		"cluster", "status", "--patroni-url", srv.URL,
	)
	if err != nil {
		t.Fatalf("cluster status failed: %v", err)
	}
	m := assertJSON(t, stdout)
	if _, ok := m["data"]; !ok {
		t.Errorf("success response must contain 'data' field, got: %v", m)
	}
}

func TestJSONEnvelope_TimestampIsRFC3339(t *testing.T) {
	_, stderr, _ := run(t, homeEnv(t), "cluster", "init", "--name", "x", "--primary-host", "h")
	m := assertJSON(t, stderr)
	ts, ok := m["timestamp"].(string)
	if !ok || ts == "" {
		t.Errorf("timestamp field missing or empty: %v", m)
	}
	// RFC3339 timestamps contain 'T' and 'Z' or timezone offset.
	if !strings.Contains(ts, "T") {
		t.Errorf("timestamp does not look like RFC3339: %s", ts)
	}
}
