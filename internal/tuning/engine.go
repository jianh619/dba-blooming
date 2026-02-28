package tuning

import (
	"fmt"

	"github.com/luckyjian/pgdba/internal/inspect"
)

// Workload represents the expected database usage pattern.
type Workload string

const (
	WorkloadOLTP  Workload = "oltp"
	WorkloadOLAP  Workload = "olap"
	WorkloadMixed Workload = "mixed"
)

// Profile controls recommendation aggressiveness.
type Profile string

const (
	ProfileDefault      Profile = "default"
	ProfileConservative Profile = "conservative"
)

// StorageType represents the underlying storage medium.
type StorageType string

const (
	StorageSSD StorageType = "ssd"
	StorageHDD StorageType = "hdd"
)

// SystemInfo holds detected or provided system resource information.
type SystemInfo struct {
	TotalRAMBytes int64
	CPUCores      int
	StorageType   StorageType
}

// GenerateRecommendations produces tuning recommendations based on current
// settings, system info, workload type, and tuning profile.
func GenerateRecommendations(
	settings []inspect.PGSetting,
	sysInfo SystemInfo,
	workload Workload,
	profile Profile,
) []inspect.Recommendation {
	// Build lookup map for current settings.
	current := make(map[string]string)
	contextMap := make(map[string]string)
	for _, s := range settings {
		current[s.Name] = s.Setting
		contextMap[s.Name] = s.Context
	}

	ramGB := float64(sysInfo.TotalRAMBytes) / (1024 * 1024 * 1024)
	var recs []inspect.Recommendation

	// shared_buffers: 25% of RAM (standard PGTune heuristic).
	sbGB := int(ramGB / 4)
	if sbGB < 1 {
		sbGB = 1
	}
	sbConf := inspect.ConfidenceHigh
	if profile == ProfileConservative {
		sbConf = inspect.ConfidenceMedium
	}
	recs = append(recs, inspect.Recommendation{
		Parameter:   "shared_buffers",
		Current:     current["shared_buffers"],
		Recommended: fmt.Sprintf("%dGB", sbGB),
		Confidence:  sbConf,
		Rationale:   fmt.Sprintf("25%% of total RAM (%.0f GB). Standard PGTune heuristic for all workloads.", ramGB),
		Source:      "pgtune",
	})

	// effective_cache_size: 75% of RAM.
	ecGB := int(ramGB * 3 / 4)
	if ecGB < 1 {
		ecGB = 1
	}
	recs = append(recs, inspect.Recommendation{
		Parameter:   "effective_cache_size",
		Current:     current["effective_cache_size"],
		Recommended: fmt.Sprintf("%dGB", ecGB),
		Confidence:  inspect.ConfidenceHigh,
		Rationale:   fmt.Sprintf("75%% of total RAM (%.0f GB). Informs query planner about OS cache.", ramGB),
		Source:      "pgtune",
	})

	// work_mem: depends on workload and RAM.
	workMemMB := computeWorkMem(ramGB, sysInfo.CPUCores, workload)
	wmConf := inspect.ConfidenceHigh
	if profile == ProfileConservative {
		wmConf = inspect.ConfidenceMedium
	}
	recs = append(recs, inspect.Recommendation{
		Parameter:   "work_mem",
		Current:     current["work_mem"],
		Recommended: fmt.Sprintf("%dMB", workMemMB),
		Confidence:  wmConf,
		Rationale:   fmt.Sprintf("Based on RAM/CPU for %s workload. Each sort/hash uses this amount.", workload),
		Source:      "pgdba-heuristic",
	})

	// maintenance_work_mem: 5-10% of RAM, max 2GB.
	maintMB := int(ramGB * 1024 * 0.05)
	if maintMB < 64 {
		maintMB = 64
	}
	if maintMB > 2048 {
		maintMB = 2048
	}
	recs = append(recs, inspect.Recommendation{
		Parameter:   "maintenance_work_mem",
		Current:     current["maintenance_work_mem"],
		Recommended: fmt.Sprintf("%dMB", maintMB),
		Confidence:  inspect.ConfidenceHigh,
		Rationale:   "5% of RAM (capped at 2GB). Used for VACUUM, CREATE INDEX, ALTER TABLE.",
		Source:      "pgtune",
	})

	// random_page_cost: SSD vs HDD.
	rpc := "4.0"
	if sysInfo.StorageType == StorageSSD {
		rpc = "1.1"
	}
	recs = append(recs, inspect.Recommendation{
		Parameter:   "random_page_cost",
		Current:     current["random_page_cost"],
		Recommended: rpc,
		Confidence:  inspect.ConfidenceHigh,
		Rationale:   fmt.Sprintf("Storage type: %s. SSD=1.1 (random reads cheap), HDD=4.0.", sysInfo.StorageType),
		Source:      "pgtune",
	})

	// checkpoint_completion_target: always 0.9.
	if _, ok := current["checkpoint_completion_target"]; ok {
		recs = append(recs, inspect.Recommendation{
			Parameter:   "checkpoint_completion_target",
			Current:     current["checkpoint_completion_target"],
			Recommended: "0.9",
			Confidence:  inspect.ConfidenceHigh,
			Rationale:   "Spread checkpoint writes over more time to reduce I/O spikes.",
			Source:      "pgtune",
		})
	}

	// max_connections: workload-dependent.
	maxConn := computeMaxConnections(workload, sysInfo.CPUCores)
	if _, ok := current["max_connections"]; ok {
		recs = append(recs, inspect.Recommendation{
			Parameter:   "max_connections",
			Current:     current["max_connections"],
			Recommended: fmt.Sprintf("%d", maxConn),
			Confidence:  inspect.ConfidenceMedium,
			Rationale:   fmt.Sprintf("Based on %s workload with %d CPU cores. Use connection pooling for higher concurrency.", workload, sysInfo.CPUCores),
			Source:      "pgdba-heuristic",
		})
	}

	return recs
}

// computeWorkMem calculates work_mem based on available RAM, cores, and workload.
func computeWorkMem(ramGB float64, cpuCores int, workload Workload) int {
	// Base: RAM / (max_connections * 4) in MB.
	// OLAP gets 4x, Mixed gets 2x.
	baseMB := int(ramGB * 1024 / 400) // ~100 connections assumed
	if baseMB < 4 {
		baseMB = 4
	}

	switch workload {
	case WorkloadOLAP:
		baseMB *= 4
	case WorkloadMixed:
		baseMB *= 2
	}

	// Cap at 2GB for safety.
	if baseMB > 2048 {
		baseMB = 2048
	}
	return baseMB
}

// computeMaxConnections suggests max_connections based on workload and cores.
func computeMaxConnections(workload Workload, cpuCores int) int {
	switch workload {
	case WorkloadOLAP:
		// OLAP: fewer connections, heavier queries.
		return cpuCores*4 + 4
	case WorkloadMixed:
		return cpuCores*8 + 20
	default:
		// OLTP: more connections.
		return cpuCores*10 + 20
	}
}
