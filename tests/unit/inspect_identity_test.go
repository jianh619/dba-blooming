package unit_test

import (
	"testing"

	"github.com/luckyjian/pgdba/internal/inspect"
)

func TestComputeFingerprint_TierSystemIdentifier(t *testing.T) {
	id := inspect.ClusterIdentity{
		Tier:             inspect.TierSystemIdentifier,
		SystemIdentifier: "6789012345678901234",
		ResolvedAddr:     "10.0.0.1",
		ResolvedPort:     5432,
		ConfigHost:       "myhost",
		ConfigPort:       5432,
		DatID:            12345,
		ServerVersionNum: 150000,
	}

	fp := id.ComputeFingerprint()
	if fp == "" {
		t.Fatal("expected non-empty fingerprint")
	}

	// Same identity should produce same fingerprint.
	fp2 := id.ComputeFingerprint()
	if fp != fp2 {
		t.Errorf("expected deterministic fingerprint, got %q and %q", fp, fp2)
	}

	// Different system_identifier should produce different fingerprint.
	id2 := id
	id2.SystemIdentifier = "9999999999999999999"
	fp3 := id2.ComputeFingerprint()
	if fp == fp3 {
		t.Errorf("expected different fingerprint for different system_identifier")
	}

	// ConfigHost/ConfigPort should NOT affect Tier 0 fingerprint.
	id3 := id
	id3.ConfigHost = "otherhost"
	id3.ConfigPort = 9999
	fp4 := id3.ComputeFingerprint()
	if fp != fp4 {
		t.Errorf("config host/port should not affect tier 0 fingerprint, got %q vs %q", fp, fp4)
	}
}

func TestComputeFingerprint_TierResolvedAddr(t *testing.T) {
	id := inspect.ClusterIdentity{
		Tier:             inspect.TierResolvedAddr,
		SystemIdentifier: "", // unavailable
		ResolvedAddr:     "10.0.0.1",
		ResolvedPort:     5432,
		ConfigHost:       "myhost",
		ConfigPort:       5432,
		DatID:            12345,
		ServerVersionNum: 120000,
	}

	fp := id.ComputeFingerprint()
	if fp == "" {
		t.Fatal("expected non-empty fingerprint for tier 1")
	}

	// H1: Different ResolvedPort should produce different fingerprint.
	id2 := id
	id2.ResolvedPort = 5433
	fp2 := id2.ComputeFingerprint()
	if fp == fp2 {
		t.Errorf("H1: different resolved_port must produce different fingerprint")
	}

	// Different DatID should produce different fingerprint.
	id3 := id
	id3.DatID = 99999
	fp3 := id3.ComputeFingerprint()
	if fp == fp3 {
		t.Errorf("different datid must produce different fingerprint")
	}

	// ConfigHost should NOT affect Tier 1 fingerprint.
	id4 := id
	id4.ConfigHost = "completely-different-host"
	fp4 := id4.ComputeFingerprint()
	if fp != fp4 {
		t.Errorf("config host should not affect tier 1 fingerprint")
	}
}

func TestComputeFingerprint_TierConfigAddr(t *testing.T) {
	id := inspect.ClusterIdentity{
		Tier:             inspect.TierConfigAddr,
		SystemIdentifier: "",
		ResolvedAddr:     "",
		ResolvedPort:     0,
		ConfigHost:       "myhost",
		ConfigPort:       5432,
		DatID:            0,
		ServerVersionNum: 100000,
	}

	fp := id.ComputeFingerprint()
	if fp == "" {
		t.Fatal("expected non-empty fingerprint for tier 2")
	}

	// Different config host should produce different fingerprint.
	id2 := id
	id2.ConfigHost = "otherhost"
	fp2 := id2.ComputeFingerprint()
	if fp == fp2 {
		t.Errorf("different config host must produce different fingerprint at tier 2")
	}
}

func TestComputeFingerprint_DifferentTiersSameData(t *testing.T) {
	// A Tier 0 identity and a Tier 1 identity with same resolved info
	// should produce different fingerprints (different input domains).
	id0 := inspect.ClusterIdentity{
		Tier:             inspect.TierSystemIdentifier,
		SystemIdentifier: "12345",
		ResolvedAddr:     "10.0.0.1",
		ResolvedPort:     5432,
		DatID:            1,
	}
	id1 := inspect.ClusterIdentity{
		Tier:         inspect.TierResolvedAddr,
		ResolvedAddr: "10.0.0.1",
		ResolvedPort: 5432,
		DatID:        1,
	}

	fp0 := id0.ComputeFingerprint()
	fp1 := id1.ComputeFingerprint()
	if fp0 == fp1 {
		t.Errorf("tier 0 and tier 1 fingerprints should differ")
	}
}

func TestIdentityTier_String(t *testing.T) {
	tests := []struct {
		tier inspect.IdentityTier
		want string
	}{
		{inspect.TierSystemIdentifier, "system_identifier"},
		{inspect.TierResolvedAddr, "resolved_addr"},
		{inspect.TierConfigAddr, "config_addr"},
	}
	for _, tt := range tests {
		got := tt.tier.String()
		if got != tt.want {
			t.Errorf("IdentityTier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}
