package uniswapv3

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper function to create a new Pool with all relevant fields for testing the hash.
func newTestPool(id uint64, liquidity, sqrtPrice, tick int64, ticks []TickInfo) Pool {
	return Pool{
		PoolViewMinimal: PoolViewMinimal{
			ID:           id,
			Liquidity:    big.NewInt(liquidity),
			SqrtPriceX96: big.NewInt(sqrtPrice),
			Tick:         tick,
		},
		Ticks: ticks,
	}
}

func TestDiffer(t *testing.T) {
	// --- Base Data for Tests ---
	// Define some ticks to test updates within the nested slice.
	tick1 := TickInfo{Index: 10, LiquidityNet: big.NewInt(100)}
	tick2 := TickInfo{Index: 20, LiquidityNet: big.NewInt(200)}

	pool1Old := newTestPool(1, 1000, 5000, 100, []TickInfo{tick1})
	pool2Old := newTestPool(2, 2000, 6000, 200, []TickInfo{tick2})
	pool3Old := newTestPool(3, 3000, 7000, 300, nil)

	t.Run("should identify additions correctly", func(t *testing.T) {
		oldState := []Pool{pool1Old}
		newState := []Pool{pool1Old, pool2Old} // pool2Old is the addition

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Len(t, diff.Additions, 1, "Should have one addition")
		assert.Equal(t, pool2Old.ID, diff.Additions[0].ID, "The correct pool should be marked as an addition")
		assert.Empty(t, diff.Updates, "Should have no updates")
		assert.Empty(t, diff.Deletions, "Should have no deletions")
	})

	t.Run("should identify deletions correctly", func(t *testing.T) {
		oldState := []Pool{pool1Old, pool2Old} // pool2Old will be deleted
		newState := []Pool{pool1Old}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions, "Should have no additions")
		assert.Empty(t, diff.Updates, "Should have no updates")
		assert.Len(t, diff.Deletions, 1, "Should have one deletion")
		assert.Equal(t, pool2Old.ID, diff.Deletions[0], "The correct pool ID should be marked for deletion")
	})

	t.Run("should identify updates when a core field changes", func(t *testing.T) {
		pool1Updated := newTestPool(1, 1001, 5000, 100, []TickInfo{tick1}) // Liquidity changed

		oldState := []Pool{pool1Old}
		newState := []Pool{pool1Updated}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions, "Should have no additions")
		assert.Len(t, diff.Updates, 1, "Should have one update")
		assert.Equal(t, pool1Updated.ID, diff.Updates[0].ID, "The correct pool should be marked as an update")
		assert.Empty(t, diff.Deletions, "Should have no deletions")
	})

	t.Run("should identify updates when a nested tick changes", func(t *testing.T) {
		// The liquidity within the tick has changed, which should trigger the hash difference.
		tick1Updated := TickInfo{Index: 10, LiquidityNet: big.NewInt(101)}
		pool1UpdatedWithTickChange := newTestPool(1, 1000, 5000, 100, []TickInfo{tick1Updated})

		oldState := []Pool{pool1Old}
		newState := []Pool{pool1UpdatedWithTickChange}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions)
		assert.Len(t, diff.Updates, 1, "A change in a nested tick should trigger an update")
		assert.Equal(t, pool1UpdatedWithTickChange.ID, diff.Updates[0].ID)
		assert.Empty(t, diff.Deletions)
	})

	t.Run("should handle a mix of additions, updates, and deletions", func(t *testing.T) {
		// pool1 is updated, pool2 is unchanged, pool3 is deleted
		// pool4 is added
		pool1Updated := newTestPool(1, 1000, 5001, 100, []TickInfo{tick1}) // SqrtPriceX96 changed
		pool4New := newTestPool(4, 4000, 8000, 400, nil)

		oldState := []Pool{pool1Old, pool2Old, pool3Old}
		newState := []Pool{pool1Updated, pool2Old, pool4New}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Len(t, diff.Additions, 1, "Should have one addition")
		assert.Equal(t, pool4New.ID, diff.Additions[0].ID)

		assert.Len(t, diff.Updates, 1, "Should have one update")
		assert.Equal(t, pool1Updated.ID, diff.Updates[0].ID)

		assert.Len(t, diff.Deletions, 1, "Should have one deletion")
		assert.Equal(t, pool3Old.ID, diff.Deletions[0])
	})

	t.Run("should produce an empty diff when there are no changes", func(t *testing.T) {
		oldState := []Pool{pool1Old, pool2Old}
		newState := []Pool{pool1Old, pool2Old}

		diff := Differ(oldState, newState)

		require.NotNil(t, diff)
		assert.Empty(t, diff.Additions, "Should have no additions")
		assert.Empty(t, diff.Updates, "Should have no updates")
		assert.Empty(t, diff.Deletions, "Should have no deletions")
	})
}
