// Package failover provides pre-flight validation logic for Patroni cluster
// switchover and failover operations.
package failover

import (
	"fmt"

	"github.com/luckyjian/pgdba/internal/patroni"
)

// DefaultMaxLagBytes is the maximum acceptable replication lag (in bytes) for
// a switchover candidate. 10 MB gives a short catch-up window while still
// preventing divergent timelines.
const DefaultMaxLagBytes int64 = 10 * 1024 * 1024 // 10 MB

// FindPrimary returns the name of the current primary (leader/master) member.
// Returns an error if no primary is found.
func FindPrimary(cs *patroni.ClusterStatus) (string, error) {
	for _, m := range cs.Members {
		if m.Role == "leader" || m.Role == "master" {
			return m.Name, nil
		}
	}
	return "", fmt.Errorf("no primary found in cluster (members: %d)", len(cs.Members))
}

// FindBestCandidate returns the running replica with the lowest replication lag.
// Returns an error if no running replica is available.
func FindBestCandidate(cs *patroni.ClusterStatus) (string, error) {
	var best *patroni.Member
	for i, m := range cs.Members {
		if m.Role == "leader" || m.Role == "master" {
			continue
		}
		if m.State != patroni.StateRunning {
			continue
		}
		if best == nil || m.Lag < best.Lag {
			best = &cs.Members[i]
		}
	}
	if best == nil {
		return "", fmt.Errorf("no running replica found for promotion")
	}
	return best.Name, nil
}

// CheckSwitchover validates that a controlled switchover is safe to perform.
// It checks:
//  1. The cluster has a reachable primary.
//  2. If a candidate is specified: it exists, is a running replica, and its lag
//     is within maxLagBytes.
//  3. At least one running replica is available (if no candidate specified).
func CheckSwitchover(cs *patroni.ClusterStatus, candidate string, maxLagBytes int64) error {
	// Cluster must have a primary.
	if _, err := FindPrimary(cs); err != nil {
		return fmt.Errorf("switchover pre-check: %w", err)
	}

	if candidate != "" {
		return validateCandidate(cs, candidate, maxLagBytes)
	}

	// No candidate specified: ensure at least one running replica exists.
	for _, m := range cs.Members {
		if m.Role != "leader" && m.Role != "master" && m.State == patroni.StateRunning {
			return nil
		}
	}
	return fmt.Errorf("switchover pre-check: no running replicas available")
}

// validateCandidate checks that the named candidate is a valid switchover target.
func validateCandidate(cs *patroni.ClusterStatus, candidate string, maxLagBytes int64) error {
	for _, m := range cs.Members {
		if m.Name != candidate {
			continue
		}
		if m.Role == "leader" || m.Role == "master" {
			return fmt.Errorf("candidate %q is already the primary", candidate)
		}
		if m.State != patroni.StateRunning {
			return fmt.Errorf("candidate %q is not running (state: %s)", candidate, m.State)
		}
		if m.Lag > maxLagBytes {
			return fmt.Errorf("candidate %q replication lag %d bytes exceeds threshold %d bytes",
				candidate, m.Lag, maxLagBytes)
		}
		return nil
	}
	return fmt.Errorf("candidate %q not found in cluster", candidate)
}

// ListReplicas returns all non-primary members regardless of their state.
// The caller can filter by state if needed.
func ListReplicas(cs *patroni.ClusterStatus) []patroni.Member {
	replicas := make([]patroni.Member, 0)
	for _, m := range cs.Members {
		if m.Role != "leader" && m.Role != "master" {
			replicas = append(replicas, m)
		}
	}
	return replicas
}
