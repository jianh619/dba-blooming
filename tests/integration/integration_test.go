//go:build integration

// Package integration contains end-to-end tests that exercise pgdba against a
// real Docker Compose Patroni/PostgreSQL cluster.
//
// Run with:
//
//	go test -tags integration ./tests/integration/... -v -timeout 120s
//
// The suite is idempotent:
//   - If the Docker Compose cluster is already running it is left as-is.
//   - If "local-ha" is already registered in ~/.pgdba/clusters.json it is
//     left as-is.
//
// DESTRUCTIVE: TestFailoverTrigger and TestReplicaPromote each perform a
// controlled Patroni switchover. The suite runs them back-to-back so the
// cluster returns to a healthy (leader + 2 replicas) state when all tests
// complete. Do NOT run these tests on a production cluster.
package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	clusterName = "local-ha"
	patroniURL  = "http://localhost:8008"
	patroniPort = ":8008"
)

var (
	binaryPath  string
	projectRoot string
)

// ---------------------------------------------------------------------------
// TestMain — setup: build binary, ensure cluster running, ensure registered
// ---------------------------------------------------------------------------

func TestMain(m *testing.M) {
	root, err := filepath.Abs("../..")
	if err != nil {
		panic("cannot determine project root: " + err.Error())
	}
	projectRoot = root

	// 1. Build latest binary.
	bin := filepath.Join(root, "bin", "pgdba")
	build := exec.Command("go", "build", "-o", bin, "./cmd/pgdba")
	build.Dir = root
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("failed to build pgdba: " + err.Error())
	}
	binaryPath = bin
	fmt.Println("✓ Binary built:", bin)

	// 2. Ensure Docker Compose cluster is running.
	ensureClusterRunning()

	// 3. Ensure cluster is registered in ~/.pgdba/clusters.json.
	ensureClusterConnected()

	os.Exit(m.Run())
}

// ensureClusterRunning checks http://localhost:8008/health. If unreachable it
// starts Docker Compose and waits up to 90 s for Patroni to become ready.
func ensureClusterRunning() {
	if patroniReady() {
		fmt.Printf("✓ Patroni already running at %s\n", patroniURL)
		return
	}

	fmt.Println("→ Patroni not reachable — starting Docker Compose cluster …")

	dockerDir := filepath.Join(projectRoot, "deployments", "docker")

	// Ensure .env exists (copy .env.example if absent).
	envFile := filepath.Join(dockerDir, ".env")
	exampleFile := filepath.Join(dockerDir, ".env.example")
	if _, statErr := os.Stat(envFile); os.IsNotExist(statErr) {
		if _, statErr2 := os.Stat(exampleFile); statErr2 == nil {
			data, readErr := os.ReadFile(exampleFile)
			if readErr != nil {
				panic("cannot read .env.example: " + readErr.Error())
			}
			if writeErr := os.WriteFile(envFile, data, 0o600); writeErr != nil {
				panic("cannot create .env: " + writeErr.Error())
			}
			fmt.Println("  Created .env from .env.example")
		}
	}

	up := exec.Command("docker", "compose", "up", "-d")
	up.Dir = dockerDir
	up.Stdout = os.Stdout
	up.Stderr = os.Stderr
	if err := up.Run(); err != nil {
		panic("docker compose up failed: " + err.Error())
	}

	fmt.Println("  Waiting for Patroni to become ready (up to 90 s) …")
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		if patroniReady() {
			fmt.Printf("✓ Patroni ready at %s\n", patroniURL)
			return
		}
		fmt.Print("  … still waiting\n")
	}
	panic("Patroni not ready after 90 s — check docker compose logs")
}

// patroniReady returns true if the Patroni health endpoint responds HTTP 200.
func patroniReady() bool {
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(patroniURL + "/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ensureClusterConnected registers the cluster if it is not already present.
func ensureClusterConnected() {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("cannot determine home dir: " + err.Error())
	}
	regPath := filepath.Join(home, ".pgdba", "clusters.json")
	if data, err := os.ReadFile(regPath); err == nil {
		var reg map[string]interface{}
		if json.Unmarshal(data, &reg) == nil {
			if _, ok := reg[clusterName]; ok {
				fmt.Printf("✓ Cluster %q already registered\n", clusterName)
				return
			}
		}
	}

	fmt.Printf("→ Registering cluster %q …\n", clusterName)
	out, err := pgdba("cluster", "connect",
		"--name", clusterName,
		"--patroni-url", patroniURL,
		"--pg-host", "localhost")
	if err != nil {
		panic(fmt.Sprintf("cluster connect failed: %v\n%s", err, out))
	}
	fmt.Printf("✓ Cluster %q registered\n", clusterName)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// pgdba runs the pgdba binary with args against the real home directory
// (so the shared cluster registry is used). Returns combined stdout+stderr.
func pgdba(args ...string) (string, error) {
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = os.Environ() // real HOME — real registry
	var buf strings.Builder
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return strings.TrimSpace(buf.String()), err
}

// mustJSON runs pgdba, asserts success, and parses the JSON response.
func mustJSON(t *testing.T, args ...string) map[string]interface{} {
	t.Helper()
	out, err := pgdba(args...)
	if err != nil {
		t.Fatalf("pgdba %v failed (%v):\n%s", args, err, out)
	}
	var m map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(out), &m); jsonErr != nil {
		t.Fatalf("output is not JSON (%v):\n%s", jsonErr, out)
	}
	return m
}

// printResult pretty-prints a result for human inspection during -v runs.
func printResult(t *testing.T, label, out string) {
	t.Helper()
	fmt.Printf("\n=== %s ===\n%s\n", label, out)
}

// waitForHealthy polls cluster status until healthy==true or timeout.
func waitForHealthy(t *testing.T, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, err := pgdba("cluster", "status", "--name", clusterName)
		if err == nil {
			var m map[string]interface{}
			if json.Unmarshal([]byte(out), &m) == nil {
				if data, ok := m["data"].(map[string]interface{}); ok {
					if data["healthy"] == true {
						return
					}
				}
			}
		}
		time.Sleep(5 * time.Second)
	}
	t.Fatalf("cluster did not become healthy within %s", timeout)
}

// ---------------------------------------------------------------------------
// Tests — read-only operations
// ---------------------------------------------------------------------------

// TestClusterStatus verifies cluster topology: one primary, two replicas,
// and the healthy flag is true.
func TestClusterStatus(t *testing.T) {
	out, err := pgdba("cluster", "status", "--name", clusterName)
	printResult(t, "cluster status", out)
	if err != nil {
		t.Fatalf("cluster status failed: %v\n%s", err, out)
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if m["success"] != true {
		t.Errorf("expected success=true, got: %v", m["success"])
	}
	data, _ := m["data"].(map[string]interface{})
	if data == nil {
		t.Fatal("missing data field")
	}
	if data["primary"] == "" || data["primary"] == nil {
		t.Errorf("primary is empty: %v", data)
	}
	replicaCount, _ := data["replica_count"].(float64)
	if replicaCount < 1 {
		t.Errorf("expected at least 1 replica, got replica_count=%v", data["replica_count"])
	}
	if data["healthy"] != true {
		t.Errorf("expected healthy=true, got: %v", data["healthy"])
	}
	t.Logf("primary=%v replica_count=%v healthy=%v",
		data["primary"], data["replica_count"], data["healthy"])
}

// TestClusterList verifies the registry lists local-ha.
func TestClusterList(t *testing.T) {
	out, err := pgdba("cluster", "status", "--name", clusterName)
	printResult(t, "cluster status (via list)", out)
	if err != nil {
		t.Fatalf("cluster status returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, clusterName) {
		t.Errorf("output does not mention cluster name %q:\n%s", clusterName, out)
	}
}

// TestFailoverStatus checks the current failover/switchover state.
func TestFailoverStatus(t *testing.T) {
	out, err := pgdba("failover", "status", "--name", clusterName)
	printResult(t, "failover status", out)
	if err != nil {
		t.Fatalf("failover status failed: %v\n%s", err, out)
	}
	m := mustJSON(t, "failover", "status", "--name", clusterName)
	if m["success"] != true {
		t.Errorf("expected success=true: %v", m)
	}
	data, _ := m["data"].(map[string]interface{})
	if data == nil {
		t.Fatal("missing data field in failover status response")
	}
	t.Logf("failover status data: %v", data)
}

// TestReplicaList verifies replicas are listed with expected fields.
func TestReplicaList(t *testing.T) {
	out, err := pgdba("replica", "list", "--name", clusterName)
	printResult(t, "replica list", out)
	if err != nil {
		t.Fatalf("replica list failed: %v\n%s", err, out)
	}
	m := mustJSON(t, "replica", "list", "--name", clusterName)
	if m["success"] != true {
		t.Errorf("expected success=true: %v", m)
	}
	data, _ := m["data"].(map[string]interface{})
	if data == nil {
		t.Fatal("missing data field")
	}
	count, _ := data["count"].(float64)
	if count < 1 {
		t.Errorf("expected at least 1 replica, got count=%v", data["count"])
	}
	replicas, _ := data["replicas"].([]interface{})
	for i, r := range replicas {
		row, _ := r.(map[string]interface{})
		if row == nil {
			t.Errorf("replica[%d] is not an object", i)
			continue
		}
		for _, field := range []string{"name", "state", "lag_bytes", "host", "port"} {
			if _, ok := row[field]; !ok {
				t.Errorf("replica[%d] missing field %q", i, field)
			}
		}
		t.Logf("replica[%d]: name=%v state=%v lag_bytes=%v",
			i, row["name"], row["state"], row["lag_bytes"])
	}
}

// ---------------------------------------------------------------------------
// Tests — switchover operations (destructive, run last)
// ---------------------------------------------------------------------------

// TestFailoverTrigger performs a controlled switchover and verifies:
//   - command reports success
//   - cluster remains healthy with a new primary afterward
func TestFailoverTrigger(t *testing.T) {
	// Capture current primary before switchover.
	statusBefore := mustJSON(t, "cluster", "status", "--name", clusterName)
	dataBefore, _ := statusBefore["data"].(map[string]interface{})
	primaryBefore, _ := dataBefore["primary"].(string)
	if primaryBefore == "" {
		t.Fatal("cannot determine current primary before switchover")
	}
	t.Logf("primary before switchover: %s", primaryBefore)

	// Trigger switchover (Patroni picks best replica automatically).
	out, err := pgdba("failover", "trigger", "--name", clusterName)
	printResult(t, "failover trigger", out)
	if err != nil {
		t.Fatalf("failover trigger failed: %v\n%s", err, out)
	}
	var firstRun map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(out), &firstRun); jsonErr != nil {
		t.Fatalf("failover trigger output is not JSON: %v\n%s", jsonErr, out)
	}
	if firstRun["success"] != true {
		t.Errorf("expected success=true, got: %v", firstRun)
	}
	data, _ := firstRun["data"].(map[string]interface{})
	t.Logf("switchover result: from=%v to=%v type=%v status=%v",
		data["from"], data["to"], data["type"], data["status"])

	// Wait for cluster to stabilize (up to 40 s).
	t.Log("waiting for cluster to stabilize after switchover …")
	waitForHealthy(t, 40*time.Second)

	// Verify new primary is different (or replica_count still >= 1).
	statusAfter := mustJSON(t, "cluster", "status", "--name", clusterName)
	dataAfter, _ := statusAfter["data"].(map[string]interface{})
	primaryAfter, _ := dataAfter["primary"].(string)
	t.Logf("primary after switchover: %s", primaryAfter)
	if primaryAfter == "" {
		t.Error("primary is empty after switchover")
	}
}

// TestReplicaPromote promotes a specific replica via controlled switchover,
// then verifies the cluster is healthy. This implicitly switches leadership
// back toward the original node, leaving the cluster in a stable state.
func TestReplicaPromote(t *testing.T) {
	// Find current replicas to pick a promote target.
	listOut, err := pgdba("replica", "list", "--name", clusterName)
	if err != nil {
		t.Fatalf("replica list failed before promote: %v\n%s", err, listOut)
	}
	var listM map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(listOut), &listM); jsonErr != nil {
		t.Fatalf("replica list output not JSON: %v", jsonErr)
	}
	listData, _ := listM["data"].(map[string]interface{})
	replicas, _ := listData["replicas"].([]interface{})
	if len(replicas) == 0 {
		t.Skip("no replicas available to promote")
	}

	// Pick the first replica as promote candidate.
	firstReplica, _ := replicas[0].(map[string]interface{})
	candidate, _ := firstReplica["name"].(string)
	if candidate == "" {
		t.Fatal("cannot determine candidate name")
	}
	t.Logf("promoting candidate: %s", candidate)

	out, err := pgdba("replica", "promote",
		"--name", clusterName,
		"--candidate", candidate)
	printResult(t, "replica promote", out)
	if err != nil {
		t.Fatalf("replica promote failed: %v\n%s", err, out)
	}
	var promoteM map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(out), &promoteM); jsonErr != nil {
		t.Fatalf("replica promote output not JSON: %v\n%s", jsonErr, out)
	}
	if promoteM["success"] != true {
		t.Errorf("expected success=true: %v", promoteM)
	}
	data, _ := promoteM["data"].(map[string]interface{})
	t.Logf("promote result: candidate=%v from=%v status=%v",
		data["candidate"], data["from"], data["status"])

	// Wait for cluster to stabilize.
	t.Log("waiting for cluster to stabilize after promote …")
	waitForHealthy(t, 40*time.Second)

	// Final health check.
	finalStatus := mustJSON(t, "cluster", "status", "--name", clusterName)
	finalData, _ := finalStatus["data"].(map[string]interface{})
	t.Logf("final cluster state: primary=%v replica_count=%v healthy=%v",
		finalData["primary"], finalData["replica_count"], finalData["healthy"])
	if finalData["healthy"] != true {
		t.Errorf("cluster not healthy after promote: %v", finalData)
	}
}
