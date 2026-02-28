package unit_test

import (
	"context"
	"testing"

	"github.com/luckyjian/pgdba/internal/query"
)

// mockQueryDB implements query.DB for testing.
type mockQueryDB struct {
	topQueries  []query.TopQuery
	locks       []query.LockInfo
	bloatTables []query.TableBloat
	vacuumStats []query.VacuumHealth
	tableStats  []query.TableStat
}

func (m *mockQueryDB) TopQueries(ctx context.Context, limit int, sortBy string) ([]query.TopQuery, error) {
	if limit > len(m.topQueries) {
		return m.topQueries, nil
	}
	return m.topQueries[:limit], nil
}

func (m *mockQueryDB) ActiveLocks(ctx context.Context) ([]query.LockInfo, error) {
	return m.locks, nil
}

func (m *mockQueryDB) TableBloat(ctx context.Context) ([]query.TableBloat, error) {
	return m.bloatTables, nil
}

func (m *mockQueryDB) VacuumHealthCheck(ctx context.Context) ([]query.VacuumHealth, error) {
	return m.vacuumStats, nil
}

func (m *mockQueryDB) TableStats(ctx context.Context) ([]query.TableStat, error) {
	return m.tableStats, nil
}

func TestTopQueries_Sort(t *testing.T) {
	db := &mockQueryDB{
		topQueries: []query.TopQuery{
			{QueryID: 1, Query: "SELECT 1", Calls: 100, TotalTime: 500.0, MeanTime: 5.0},
			{QueryID: 2, Query: "SELECT 2", Calls: 10, TotalTime: 1000.0, MeanTime: 100.0},
			{QueryID: 3, Query: "SELECT 3", Calls: 50, TotalTime: 200.0, MeanTime: 4.0},
		},
	}

	result, err := db.TopQueries(context.Background(), 2, "total_time")
	if err != nil {
		t.Fatalf("TopQueries failed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
}

func TestIndexSuggest_SmallTableAntiRecommend(t *testing.T) {
	stats := []query.TableStat{
		{Schema: "public", Table: "tiny_table", SeqScan: 1000, SeqTupRead: 5000, NLiveTup: 100},   // small table — no index
		{Schema: "public", Table: "large_table", SeqScan: 5000, SeqTupRead: 500000, NLiveTup: 100000}, // large table — suggest index
	}

	suggestions := query.SuggestIndexes(stats, 10000) // min 10k rows
	if len(suggestions) == 0 {
		t.Fatal("expected at least one index suggestion")
	}

	// Verify small table is excluded.
	for _, s := range suggestions {
		if s.Table == "tiny_table" {
			t.Error("should not suggest index for table with < 10k rows (anti-recommend #9)")
		}
	}

	// Verify large table is included.
	found := false
	for _, s := range suggestions {
		if s.Table == "large_table" {
			found = true
			if s.Reason == "" {
				t.Error("expected non-empty reason")
			}
		}
	}
	if !found {
		t.Error("expected index suggestion for large_table with high seq_scan")
	}
}

func TestLockInfo_WaitChains(t *testing.T) {
	locks := []query.LockInfo{
		{PID: 100, Mode: "AccessExclusiveLock", Granted: true, Relation: "orders", WaitingPIDs: []int{200, 300}},
		{PID: 200, Mode: "RowExclusiveLock", Granted: false, Relation: "orders", BlockedByPID: 100},
		{PID: 300, Mode: "AccessShareLock", Granted: false, Relation: "orders", BlockedByPID: 100},
	}

	chains := query.BuildLockChains(locks)
	if len(chains) == 0 {
		t.Fatal("expected at least one lock chain")
	}

	// PID 100 should be root of chain.
	rootChain := chains[0]
	if rootChain.RootPID != 100 {
		t.Errorf("expected root PID=100, got %d", rootChain.RootPID)
	}
	if len(rootChain.WaitingPIDs) != 2 {
		t.Errorf("expected 2 waiting PIDs, got %d", len(rootChain.WaitingPIDs))
	}
}

func TestVacuumHealth_Fields(t *testing.T) {
	stats := []query.VacuumHealth{
		{
			Schema:          "public",
			Table:           "orders",
			DeadTuples:      5000,
			LiveTuples:      100000,
			LastVacuum:      "2026-02-20 10:00:00",
			LastAutoVacuum:  "2026-02-25 15:30:00",
			AutoVacuumCount: 42,
		},
	}

	if stats[0].DeadTuples != 5000 {
		t.Errorf("expected dead_tuples=5000, got %d", stats[0].DeadTuples)
	}
	if stats[0].DeadTupleRatio() < 0.04 || stats[0].DeadTupleRatio() > 0.06 {
		t.Errorf("expected ~5%% dead tuple ratio, got %.4f", stats[0].DeadTupleRatio())
	}
}

func TestTableBloat_Fields(t *testing.T) {
	bloat := query.TableBloat{
		Schema:       "public",
		Table:        "orders",
		TableBytes:   104857600, // 100MB
		BloatBytes:   20971520,  // 20MB
		BloatRatio:   0.2,
	}

	if bloat.BloatRatio < 0.19 || bloat.BloatRatio > 0.21 {
		t.Errorf("expected ~20%% bloat ratio, got %.2f", bloat.BloatRatio)
	}
}
