package unit_test

import (
	"testing"

	"github.com/luckyjian/pgdba/internal/failover"
	"github.com/luckyjian/pgdba/internal/patroni"
)

// helpers -------------------------------------------------------------------

func running(name, role string, lag int64) patroni.Member {
	return patroni.Member{Name: name, Role: role, State: patroni.StateRunning, Lag: lag}
}

func stopped(name, role string) patroni.Member {
	return patroni.Member{Name: name, Role: role, State: patroni.StateStopped}
}

func cs(members ...patroni.Member) *patroni.ClusterStatus {
	return &patroni.ClusterStatus{Members: members}
}

// FindPrimary ---------------------------------------------------------------

func TestFindPrimary_Leader(t *testing.T) {
	primary, err := failover.FindPrimary(cs(
		running("pg-primary", "leader", 0),
		running("pg-replica-1", "replica", 100),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if primary != "pg-primary" {
		t.Errorf("expected pg-primary, got %s", primary)
	}
}

func TestFindPrimary_MasterRole(t *testing.T) {
	// Patroni older versions use "master" instead of "leader"
	primary, err := failover.FindPrimary(cs(
		running("pg-primary", "master", 0),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if primary != "pg-primary" {
		t.Errorf("expected pg-primary, got %s", primary)
	}
}

func TestFindPrimary_NoPrimary(t *testing.T) {
	_, err := failover.FindPrimary(cs(
		running("pg-replica-1", "replica", 100),
		running("pg-replica-2", "replica", 200),
	))
	if err == nil {
		t.Fatal("expected error when no primary, got nil")
	}
}

func TestFindPrimary_EmptyCluster(t *testing.T) {
	_, err := failover.FindPrimary(cs())
	if err == nil {
		t.Fatal("expected error for empty cluster")
	}
}

// FindBestCandidate ---------------------------------------------------------

func TestFindBestCandidate_LowestLag(t *testing.T) {
	candidate, err := failover.FindBestCandidate(cs(
		running("pg-primary", "leader", 0),
		running("pg-replica-1", "replica", 500),
		running("pg-replica-2", "replica", 100),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if candidate != "pg-replica-2" {
		t.Errorf("expected pg-replica-2 (lowest lag), got %s", candidate)
	}
}

func TestFindBestCandidate_SkipsStoppedReplica(t *testing.T) {
	candidate, err := failover.FindBestCandidate(cs(
		running("pg-primary", "leader", 0),
		stopped("pg-replica-1", "replica"),
		running("pg-replica-2", "replica", 200),
	))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if candidate != "pg-replica-2" {
		t.Errorf("expected pg-replica-2, got %s", candidate)
	}
}

func TestFindBestCandidate_NoReplicas(t *testing.T) {
	_, err := failover.FindBestCandidate(cs(
		running("pg-primary", "leader", 0),
	))
	if err == nil {
		t.Fatal("expected error when no replicas, got nil")
	}
}

func TestFindBestCandidate_AllReplicasStopped(t *testing.T) {
	_, err := failover.FindBestCandidate(cs(
		running("pg-primary", "leader", 0),
		stopped("pg-replica-1", "replica"),
		stopped("pg-replica-2", "replica"),
	))
	if err == nil {
		t.Fatal("expected error when all replicas stopped")
	}
}

// CheckSwitchover -----------------------------------------------------------

func TestCheckSwitchover_Healthy(t *testing.T) {
	err := failover.CheckSwitchover(cs(
		running("pg-primary", "leader", 0),
		running("pg-replica-1", "replica", 100),
	), "", failover.DefaultMaxLagBytes)
	if err != nil {
		t.Errorf("unexpected error for healthy cluster: %v", err)
	}
}

func TestCheckSwitchover_WithValidCandidate(t *testing.T) {
	err := failover.CheckSwitchover(cs(
		running("pg-primary", "leader", 0),
		running("pg-replica-1", "replica", 100),
	), "pg-replica-1", failover.DefaultMaxLagBytes)
	if err != nil {
		t.Errorf("unexpected error for valid candidate: %v", err)
	}
}

func TestCheckSwitchover_CandidateIsAlreadyPrimary(t *testing.T) {
	err := failover.CheckSwitchover(cs(
		running("pg-primary", "leader", 0),
		running("pg-replica-1", "replica", 100),
	), "pg-primary", failover.DefaultMaxLagBytes)
	if err == nil {
		t.Fatal("expected error when candidate is already primary")
	}
}

func TestCheckSwitchover_CandidateNotFound(t *testing.T) {
	err := failover.CheckSwitchover(cs(
		running("pg-primary", "leader", 0),
		running("pg-replica-1", "replica", 100),
	), "pg-replica-99", failover.DefaultMaxLagBytes)
	if err == nil {
		t.Fatal("expected error when candidate not found in cluster")
	}
}

func TestCheckSwitchover_CandidateLagTooHigh(t *testing.T) {
	err := failover.CheckSwitchover(cs(
		running("pg-primary", "leader", 0),
		running("pg-replica-1", "replica", failover.DefaultMaxLagBytes+1),
	), "pg-replica-1", failover.DefaultMaxLagBytes)
	if err == nil {
		t.Fatal("expected error when candidate lag exceeds threshold")
	}
}

func TestCheckSwitchover_CandidateStopped(t *testing.T) {
	err := failover.CheckSwitchover(cs(
		running("pg-primary", "leader", 0),
		stopped("pg-replica-1", "replica"),
	), "pg-replica-1", failover.DefaultMaxLagBytes)
	if err == nil {
		t.Fatal("expected error when candidate is not running")
	}
}

func TestCheckSwitchover_NoPrimary(t *testing.T) {
	err := failover.CheckSwitchover(cs(
		running("pg-replica-1", "replica", 100),
	), "", failover.DefaultMaxLagBytes)
	if err == nil {
		t.Fatal("expected error when no primary found")
	}
}

func TestCheckSwitchover_NoRunningReplicas(t *testing.T) {
	err := failover.CheckSwitchover(cs(
		running("pg-primary", "leader", 0),
		stopped("pg-replica-1", "replica"),
	), "", failover.DefaultMaxLagBytes)
	if err == nil {
		t.Fatal("expected error when no running replicas available")
	}
}

// ListReplicas --------------------------------------------------------------

func TestListReplicas_ReturnsOnlyNonPrimary(t *testing.T) {
	replicas := failover.ListReplicas(cs(
		running("pg-primary", "leader", 0),
		running("pg-replica-1", "replica", 100),
		running("pg-replica-2", "replica", 200),
	))
	if len(replicas) != 2 {
		t.Errorf("expected 2 replicas, got %d", len(replicas))
	}
	for _, r := range replicas {
		if r.Name == "pg-primary" {
			t.Error("primary should not be in replica list")
		}
	}
}

func TestListReplicas_IncludesStoppedReplicas(t *testing.T) {
	replicas := failover.ListReplicas(cs(
		running("pg-primary", "leader", 0),
		running("pg-replica-1", "replica", 100),
		stopped("pg-replica-2", "replica"),
	))
	if len(replicas) != 2 {
		t.Errorf("expected 2 replicas (including stopped), got %d", len(replicas))
	}
}

func TestListReplicas_EmptyCluster(t *testing.T) {
	replicas := failover.ListReplicas(cs())
	if len(replicas) != 0 {
		t.Errorf("expected 0 replicas for empty cluster, got %d", len(replicas))
	}
}
