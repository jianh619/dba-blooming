package unit_test

import (
	"testing"

	"github.com/luckyjian/pgdba/internal/inspect"
	"github.com/luckyjian/pgdba/internal/tuning"
)

func TestGenerateRecommendations_OLTP_16GB(t *testing.T) {
	settings := []inspect.PGSetting{
		{Name: "shared_buffers", Setting: "128MB", Context: "postmaster"},
		{Name: "effective_cache_size", Setting: "4GB", Context: "user"},
		{Name: "work_mem", Setting: "4MB", Context: "user"},
		{Name: "maintenance_work_mem", Setting: "64MB", Context: "user"},
		{Name: "max_connections", Setting: "100", Context: "postmaster"},
		{Name: "random_page_cost", Setting: "4", Context: "user"},
		{Name: "checkpoint_completion_target", Setting: "0.5", Context: "sighup"},
		{Name: "wal_buffers", Setting: "-1", Context: "postmaster"},
	}

	sysInfo := tuning.SystemInfo{
		TotalRAMBytes: 16 * 1024 * 1024 * 1024, // 16 GB
		CPUCores:      4,
		StorageType:   tuning.StorageSSD,
	}

	recs := tuning.GenerateRecommendations(settings, sysInfo, tuning.WorkloadOLTP, tuning.ProfileDefault)

	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}

	// Find shared_buffers recommendation.
	var sbRec *inspect.Recommendation
	for i := range recs {
		if recs[i].Parameter == "shared_buffers" {
			sbRec = &recs[i]
			break
		}
	}
	if sbRec == nil {
		t.Fatal("expected shared_buffers recommendation")
	}

	// For 16GB RAM, shared_buffers should be ~4GB (25%).
	if sbRec.Recommended != "4GB" {
		t.Errorf("expected shared_buffers=4GB for 16GB RAM, got %q", sbRec.Recommended)
	}
	if sbRec.Confidence != inspect.ConfidenceHigh {
		t.Errorf("expected high confidence for shared_buffers, got %q", sbRec.Confidence)
	}
	if sbRec.Rationale == "" {
		t.Error("expected non-empty rationale")
	}

	// random_page_cost should be 1.1 for SSD.
	var rpcRec *inspect.Recommendation
	for i := range recs {
		if recs[i].Parameter == "random_page_cost" {
			rpcRec = &recs[i]
			break
		}
	}
	if rpcRec == nil {
		t.Fatal("expected random_page_cost recommendation")
	}
	if rpcRec.Recommended != "1.1" {
		t.Errorf("expected random_page_cost=1.1 for SSD, got %q", rpcRec.Recommended)
	}
}

func TestGenerateRecommendations_OLAP_64GB(t *testing.T) {
	settings := []inspect.PGSetting{
		{Name: "shared_buffers", Setting: "128MB", Context: "postmaster"},
		{Name: "effective_cache_size", Setting: "4GB", Context: "user"},
		{Name: "work_mem", Setting: "4MB", Context: "user"},
		{Name: "maintenance_work_mem", Setting: "64MB", Context: "user"},
		{Name: "max_connections", Setting: "100", Context: "postmaster"},
	}

	sysInfo := tuning.SystemInfo{
		TotalRAMBytes: 64 * 1024 * 1024 * 1024,
		CPUCores:      16,
		StorageType:   tuning.StorageSSD,
	}

	recs := tuning.GenerateRecommendations(settings, sysInfo, tuning.WorkloadOLAP, tuning.ProfileDefault)

	var workMemRec *inspect.Recommendation
	for i := range recs {
		if recs[i].Parameter == "work_mem" {
			workMemRec = &recs[i]
			break
		}
	}
	if workMemRec == nil {
		t.Fatal("expected work_mem recommendation")
	}
	// OLAP should have higher work_mem than OLTP.
	if workMemRec.Recommended == "4MB" {
		t.Error("OLAP work_mem should differ from default 4MB")
	}
}

func TestGenerateRecommendations_Conservative(t *testing.T) {
	settings := []inspect.PGSetting{
		{Name: "shared_buffers", Setting: "128MB", Context: "postmaster"},
		{Name: "effective_cache_size", Setting: "4GB", Context: "user"},
		{Name: "work_mem", Setting: "4MB", Context: "user"},
	}

	sysInfo := tuning.SystemInfo{
		TotalRAMBytes: 8 * 1024 * 1024 * 1024,
		CPUCores:      2,
		StorageType:   tuning.StorageHDD,
	}

	recs := tuning.GenerateRecommendations(settings, sysInfo, tuning.WorkloadOLTP, tuning.ProfileConservative)

	// Conservative profile should have medium/low confidence.
	for _, r := range recs {
		if r.Confidence == inspect.ConfidenceHigh {
			// Some params can still be high (like random_page_cost for HDD).
			// But shared_buffers should be medium in conservative mode.
			if r.Parameter == "shared_buffers" {
				t.Errorf("conservative profile should lower shared_buffers confidence, got %q", r.Confidence)
			}
		}
	}

	// random_page_cost for HDD should be 4.0.
	var rpcRec *inspect.Recommendation
	for i := range recs {
		if recs[i].Parameter == "random_page_cost" {
			rpcRec = &recs[i]
			break
		}
	}
	if rpcRec != nil && rpcRec.Recommended != "4.0" {
		t.Errorf("expected random_page_cost=4.0 for HDD, got %q", rpcRec.Recommended)
	}
}

func TestGenerateRecommendations_NoChangesNeeded(t *testing.T) {
	// Settings already match recommendations — should still return recs
	// but current == recommended.
	settings := []inspect.PGSetting{
		{Name: "shared_buffers", Setting: "2GB", Context: "postmaster"},
		{Name: "effective_cache_size", Setting: "6GB", Context: "user"},
	}

	sysInfo := tuning.SystemInfo{
		TotalRAMBytes: 8 * 1024 * 1024 * 1024,
		CPUCores:      4,
		StorageType:   tuning.StorageSSD,
	}

	recs := tuning.GenerateRecommendations(settings, sysInfo, tuning.WorkloadOLTP, tuning.ProfileDefault)

	var sbRec *inspect.Recommendation
	for i := range recs {
		if recs[i].Parameter == "shared_buffers" {
			sbRec = &recs[i]
			break
		}
	}
	if sbRec == nil {
		t.Fatal("expected shared_buffers recommendation even when already optimal")
	}
	if sbRec.Current != "2GB" {
		t.Errorf("expected current=2GB, got %q", sbRec.Current)
	}
}

func TestWorkloadTypes(t *testing.T) {
	workloads := []tuning.Workload{tuning.WorkloadOLTP, tuning.WorkloadOLAP, tuning.WorkloadMixed}
	for _, w := range workloads {
		if w == "" {
			t.Error("workload should not be empty")
		}
	}
}

func TestStorageTypes(t *testing.T) {
	types := []tuning.StorageType{tuning.StorageSSD, tuning.StorageHDD}
	for _, s := range types {
		if s == "" {
			t.Error("storage type should not be empty")
		}
	}
}
