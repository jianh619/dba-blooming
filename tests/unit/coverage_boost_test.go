package unit_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/luckyjian/pgdba/internal/output"
	"github.com/luckyjian/pgdba/internal/postgres"
	"github.com/luckyjian/pgdba/internal/provider"
)

// --- output package additional coverage ---

func TestFormatTable_FailureWithError(t *testing.T) {
	err := errors.New("disk full")
	r := output.Failure("backup create", err)
	out, fmtErr := output.FormatResponse(r, output.FormatTable)
	if fmtErr != nil {
		t.Fatalf("unexpected error: %v", fmtErr)
	}
	if !strings.Contains(out, "FAILURE") {
		t.Errorf("expected 'FAILURE' in table output, got: %s", out)
	}
	if !strings.Contains(out, "disk full") {
		t.Errorf("expected error message in table output, got: %s", out)
	}
}

func TestFormatTable_HasHeaders(t *testing.T) {
	r := output.Success("cluster status", nil)
	out, err := output.FormatResponse(r, output.FormatTable)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "STATUS") {
		t.Errorf("expected 'STATUS' header in table, got: %s", out)
	}
	if !strings.Contains(out, "COMMAND") {
		t.Errorf("expected 'COMMAND' header in table, got: %s", out)
	}
	if !strings.Contains(out, "TIMESTAMP") {
		t.Errorf("expected 'TIMESTAMP' header in table, got: %s", out)
	}
}

func TestFailureResponse_CommandPreserved(t *testing.T) {
	err := errors.New("timeout")
	r := output.Failure("replica add", err)
	if r.Command != "replica add" {
		t.Errorf("expected command 'replica add', got %q", r.Command)
	}
	if r.Data != nil {
		t.Error("expected Data=nil for failure response")
	}
}

func TestSuccessResponse_DataPreserved(t *testing.T) {
	data := []string{"node1", "node2"}
	r := output.Success("cluster list", data)
	if r.Data == nil {
		t.Error("expected non-nil Data")
	}
}

// --- postgres package additional coverage ---

func TestPassword_FromEnv(t *testing.T) {
	os.Setenv("PGDBA_PG_PASSWORD", "supersecret")
	defer os.Unsetenv("PGDBA_PG_PASSWORD")

	pw := postgres.Password()
	if pw != "supersecret" {
		t.Errorf("expected 'supersecret', got %q", pw)
	}
}

func TestPassword_Empty_WhenEnvNotSet(t *testing.T) {
	os.Unsetenv("PGDBA_PG_PASSWORD")
	pw := postgres.Password()
	if pw != "" {
		t.Errorf("expected empty string when env not set, got %q", pw)
	}
}

func TestConnect_FailsWithInvalidDSN(t *testing.T) {
	cfg := postgres.Config{
		Host:     "nonexistent-host-12345",
		Port:     5432,
		User:     "postgres",
		Database: "postgres",
		SSLMode:  "disable",
	}
	_, err := postgres.Connect(context.Background(), cfg)
	if err == nil {
		t.Error("expected connection error for invalid host")
	}
}

// --- provider package additional coverage ---

func TestDockerProvider_ListNodes_ReturnsError(t *testing.T) {
	p, _ := provider.New("docker", nil)
	_, err := p.ListNodes(context.Background())
	if err == nil {
		t.Error("expected error from unimplemented ListNodes")
	}
}

func TestDockerProvider_CreateNode_ValidName_ReturnsNotImplemented(t *testing.T) {
	p, _ := provider.New("docker", nil)
	_, err := p.CreateNode(context.Background(), provider.NodeConfig{Name: "pg-primary"})
	if err == nil {
		t.Error("expected not-implemented error for CreateNode with valid name")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected 'not implemented' in error, got: %v", err)
	}
}

func TestDockerProvider_DestroyNode_ValidID_ReturnsNotImplemented(t *testing.T) {
	p, _ := provider.New("docker", nil)
	err := p.DestroyNode(context.Background(), "container-abc123")
	if err == nil {
		t.Error("expected not-implemented error for DestroyNode with valid id")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected 'not implemented' in error, got: %v", err)
	}
}

func TestDockerProvider_ExecOnNode_ValidArgs_ReturnsNotImplemented(t *testing.T) {
	p, _ := provider.New("docker", nil)
	_, err := p.ExecOnNode(context.Background(), "container-abc123", []string{"psql", "-c", "SELECT 1"})
	if err == nil {
		t.Error("expected not-implemented error for ExecOnNode with valid args")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected 'not implemented' in error, got: %v", err)
	}
}

func TestDockerProvider_GetNodeStatus_ValidID_ReturnsNotImplemented(t *testing.T) {
	p, _ := provider.New("docker", nil)
	_, err := p.GetNodeStatus(context.Background(), "container-abc123")
	if err == nil {
		t.Error("expected not-implemented error for GetNodeStatus with valid id")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected 'not implemented' in error, got: %v", err)
	}
}

func TestDockerProvider_PartitionNode_ValidID_ReturnsNotImplemented(t *testing.T) {
	p, _ := provider.New("docker", nil)
	err := p.PartitionNode(context.Background(), "container-abc123", true)
	if err == nil {
		t.Error("expected not-implemented error for PartitionNode with valid id")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("expected 'not implemented' in error, got: %v", err)
	}
}

func TestDockerProvider_PartitionNode_Deactivate(t *testing.T) {
	p, _ := provider.New("docker", nil)
	err := p.PartitionNode(context.Background(), "container-abc123", false)
	if err == nil {
		t.Error("expected not-implemented error for PartitionNode(isolate=false)")
	}
}
