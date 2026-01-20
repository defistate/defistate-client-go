package uniswapv3

import (
	"sort"
)

type UniswapV3SystemDiff struct {
	Additions []Pool   `json:"additions,omitempty"`
	Updates   []Pool   `json:"updates,omitempty"`
	Deletions []uint64 `json:"deletions,omitempty"`
}

// IsEmpty returns true if the diff contains no changes.
func (d UniswapV3SystemDiff) IsEmpty() bool {
	return len(d.Additions) == 0 && len(d.Updates) == 0 && len(d.Deletions) == 0
}

// @todo optimize
func poolChanged(old, new Pool) bool {
	// 1. Compare core dynamic fields

	if old.Tick != new.Tick {
		return true
	}

	if old.SqrtPriceX96.Cmp(new.SqrtPriceX96) != 0 {
		return true
	}

	if old.Liquidity.Cmp(new.Liquidity) != 0 {
		return true
	}

	// 2. Compare ticks (order-insensitive)

	if len(old.Ticks) != len(new.Ticks) {
		return true
	}

	// Make sorted copies so comparison is independent of slice order
	oldTicks := make([]TickInfo, len(old.Ticks))
	copy(oldTicks, old.Ticks)
	sort.Slice(oldTicks, func(i, j int) bool {
		return oldTicks[i].Index < oldTicks[j].Index
	})

	newTicks := make([]TickInfo, len(new.Ticks))
	copy(newTicks, new.Ticks)
	sort.Slice(newTicks, func(i, j int) bool {
		return newTicks[i].Index < newTicks[j].Index
	})

	for i := range oldTicks {
		if oldTicks[i].Index != newTicks[i].Index {
			return true
		}

		if oldTicks[i].LiquidityNet.Cmp(newTicks[i].LiquidityNet) != 0 {
			return true
		}
	}

	// Everything matched
	return false
}

// Differ is a concrete implementation of the UniswapV3SystemDiffer function type.
// It efficiently calculates the difference between two states of Uniswap V3 pools.
// The logic is optimized for performance using maps for O(1) average time complexity lookups.
func Differ(old, new []Pool) UniswapV3SystemDiff {
	// --- 1. Create maps for efficient lookups ---
	// The key is the pool's unique ID, and the value is the Pool itself.
	oldPoolsMap := make(map[uint64]Pool, len(old))
	for _, pool := range old {
		oldPoolsMap[pool.ID] = pool
	}

	newPoolsMap := make(map[uint64]Pool, len(new))
	for _, pool := range new {
		newPoolsMap[pool.ID] = pool
	}

	var additions []Pool
	var updates []Pool
	var deletions []uint64

	// --- 2. Identify Additions and Updates ---
	// Iterate through the new set of pools.
	for newID, newPool := range newPoolsMap {
		oldPool, exists := oldPoolsMap[newID]

		if !exists {
			// If the pool from the new list does not exist in the old list, it's an addition.
			additions = append(additions, newPool)
		} else {
			// If the pool exists in both, we compute a hash of the relevant fields for each
			// version of the pool. If the hashes differ, the pool has been updated.
			if poolChanged(oldPool, newPool) {
				updates = append(updates, newPool)
			}
		}
	}

	// --- 3. Identify Deletions ---
	// Iterate through the old set of pools.
	for oldID := range oldPoolsMap {
		_, exists := newPoolsMap[oldID]
		if !exists {
			// If a pool from the old list does not exist in the new list, it has been deleted.
			deletions = append(deletions, oldID)
		}
	}

	return UniswapV3SystemDiff{
		Additions: additions,
		Updates:   updates,
		Deletions: deletions,
	}
}
