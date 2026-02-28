package query

import "fmt"

// SuggestIndexes analyzes table statistics and returns index suggestions.
// Tables with fewer than minRows live tuples are excluded (anti-recommend #9).
func SuggestIndexes(stats []TableStat, minRows int64) []IndexSuggestion {
	var suggestions []IndexSuggestion

	for _, s := range stats {
		// Anti-recommend: skip small tables.
		if s.NLiveTup < minRows {
			continue
		}

		// High sequential scans relative to index scans suggest missing index.
		if s.SeqScan > 100 && (s.IdxScan == 0 || s.SeqScan > s.IdxScan*10) {
			suggestions = append(suggestions, IndexSuggestion{
				Schema:  s.Schema,
				Table:   s.Table,
				SeqScan: s.SeqScan,
				NLiveTup: s.NLiveTup,
				Reason: fmt.Sprintf(
					"High sequential scan count (%d) with %d live tuples. "+
						"Consider adding indexes on frequently queried columns.",
					s.SeqScan, s.NLiveTup),
			})
		}
	}

	return suggestions
}

// BuildLockChains groups locks into dependency chains rooted at blocking PIDs.
func BuildLockChains(locks []LockInfo) []LockChain {
	// Find root locks (granted=true with waiting PIDs).
	var chains []LockChain

	for _, l := range locks {
		if l.Granted && len(l.WaitingPIDs) > 0 {
			chains = append(chains, LockChain{
				RootPID:     l.PID,
				Mode:        l.Mode,
				Relation:    l.Relation,
				WaitingPIDs: l.WaitingPIDs,
			})
		}
	}

	return chains
}
