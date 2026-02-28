package inspect

import (
	"crypto/sha256"
	"fmt"
)

// IdentityTier represents the quality tier of cluster identification.
// Higher tiers use more stable identifiers.
type IdentityTier int

const (
	// TierSystemIdentifier uses pg_control_system() — PG 13+, most stable.
	TierSystemIdentifier IdentityTier = iota
	// TierResolvedAddr uses inet_server_addr():inet_server_port():datid.
	TierResolvedAddr
	// TierConfigAddr uses user-provided config_host:config_port (fallback).
	TierConfigAddr
)

// String returns a human-readable label for the identity tier.
func (t IdentityTier) String() string {
	switch t {
	case TierSystemIdentifier:
		return "system_identifier"
	case TierResolvedAddr:
		return "resolved_addr"
	case TierConfigAddr:
		return "config_addr"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// ClusterIdentity holds all information needed to uniquely identify a
// PostgreSQL cluster. The Fingerprint is computed from the highest
// available tier of identification data.
type ClusterIdentity struct {
	Tier             IdentityTier
	SystemIdentifier string // from pg_control_system(); empty if PG <13
	ResolvedAddr     string // inet_server_addr()
	ResolvedPort     int    // inet_server_port() — separate from ConfigPort (H1)
	ConfigHost       string // user-provided host (display/audit only)
	ConfigPort       int    // user-provided port (display/audit only)
	DatID            uint32 // current database OID
	ServerVersionNum int    // from server_version_num setting
	Fingerprint      string // computed by ComputeFingerprint
}

// ComputeFingerprint produces a deterministic SHA-256 hex string based on the
// highest available identification tier:
//
//	Tier 0: SHA256("t0:" + system_identifier)
//	Tier 1: SHA256("t1:" + resolved_addr + ":" + resolved_port + ":" + datid)
//	Tier 2: SHA256("t2:" + config_host + ":" + config_port)
func (id *ClusterIdentity) ComputeFingerprint() string {
	var input string
	switch id.Tier {
	case TierSystemIdentifier:
		input = fmt.Sprintf("t0:%s", id.SystemIdentifier)
	case TierResolvedAddr:
		input = fmt.Sprintf("t1:%s:%d:%d", id.ResolvedAddr, id.ResolvedPort, id.DatID)
	case TierConfigAddr:
		input = fmt.Sprintf("t2:%s:%d", id.ConfigHost, id.ConfigPort)
	default:
		input = fmt.Sprintf("t?:%s:%d", id.ConfigHost, id.ConfigPort)
	}

	h := sha256.Sum256([]byte(input))
	id.Fingerprint = fmt.Sprintf("%x", h)
	return id.Fingerprint
}
