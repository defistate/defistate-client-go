package uniswapv3

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to find a pool by ID in a slice for testing assertions.
func findPoolByID(pools []Pool, id uint64) *Pool {
	for i := range pools {
		if pools[i].ID == id {
			return &pools[i]
		}
	}
	return nil
}

func TestPatcher(t *testing.T) {
	// --- Base Data for Tests ---
	tick1 := TickInfo{Index: 10, LiquidityNet: big.NewInt(100), LiquidityGross: big.NewInt(100)}
	tick2 := TickInfo{Index: 20, LiquidityNet: big.NewInt(200), LiquidityGross: big.NewInt(200)}

	pool1Old := newTestPool(1, 1000, 5000, 100, []TickInfo{tick1})
	pool2Old := newTestPool(2, 2000, 6000, 200, []TickInfo{tick2})
	pool3Old := newTestPool(3, 3000, 7000, 300, nil)

	initialState := []Pool{pool1Old, pool2Old, pool3Old}

	t.Run("should handle only additions", func(t *testing.T) {
		pool4New := newTestPool(4, 4000, 8000, 400, nil)
		diff := UniswapV3SystemDiff{
			Additions: []Pool{pool4New},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 4)
		newPool := findPoolByID(newState, 4)
		require.NotNil(t, newPool)
		assert.Equal(t, int64(4000), newPool.Liquidity.Int64())
	})

	t.Run("should handle only deletions", func(t *testing.T) {
		diff := UniswapV3SystemDiff{
			Deletions: []uint64{2},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 2)
		assert.Nil(t, findPoolByID(newState, 2))
	})

	t.Run("should handle only updates", func(t *testing.T) {
		pool1Updated := newTestPool(1, 1001, 5005, 101, []TickInfo{tick1}) // All core fields changed
		diff := UniswapV3SystemDiff{
			Updates: []Pool{pool1Updated},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 3)
		updatedPool := findPoolByID(newState, 1)
		require.NotNil(t, updatedPool)
		assert.Equal(t, int64(1001), updatedPool.Liquidity.Int64())
		assert.Equal(t, int64(5005), updatedPool.SqrtPriceX96.Int64())
		assert.Equal(t, int64(101), updatedPool.Tick)
	})

	t.Run("should verify deep copy on update", func(t *testing.T) {
		// Create a fresh copy for this test to avoid modifying the global test data.
		localInitialState := []Pool{newTestPool(1, 1000, 5000, 100, []TickInfo{tick1})}

		pool1Updated := newTestPool(1, 1001, 5005, 101, []TickInfo{tick1})
		diff := UniswapV3SystemDiff{
			Updates: []Pool{pool1Updated},
		}

		newState, err := Patcher(localInitialState, diff)
		require.NoError(t, err)
		require.Len(t, newState, 1)

		// CRITICAL: Modify a big.Int in the *original* state slice after patching.
		localInitialState[0].Liquidity.SetInt64(9999)
		// Also modify a nested big.Int.
		localInitialState[0].Ticks[0].LiquidityNet.SetInt64(9999)

		// Verify that the new state was not affected, proving the deep copy worked.
		updatedPool := findPoolByID(newState, 1)
		require.NotNil(t, updatedPool)
		assert.Equal(t, int64(1001), updatedPool.Liquidity.Int64(), "New state should be isolated from changes to the old state")
		assert.Equal(t, int64(100), updatedPool.Ticks[0].LiquidityNet.Int64(), "New state's nested ticks should be isolated")
	})

	t.Run("should handle a mix of operations", func(t *testing.T) {
		pool4New := newTestPool(4, 4000, 8000, 400, nil)
		pool2Updated := newTestPool(2, 2002, 6006, 202, []TickInfo{tick2})
		diff := UniswapV3SystemDiff{
			Additions: []Pool{pool4New},
			Updates:   []Pool{pool2Updated},
			Deletions: []uint64{3},
		}

		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)

		assert.Len(t, newState, 3)
		assert.NotNil(t, findPoolByID(newState, 4))
		updatedPool := findPoolByID(newState, 2)
		require.NotNil(t, updatedPool)
		assert.Equal(t, int64(2002), updatedPool.Liquidity.Int64())
		assert.Nil(t, findPoolByID(newState, 3))
		assert.NotNil(t, findPoolByID(newState, 1))
	})

	t.Run("should handle an empty diff", func(t *testing.T) {
		diff := UniswapV3SystemDiff{}
		newState, err := Patcher(initialState, diff)
		require.NoError(t, err)
		assert.ElementsMatch(t, initialState, newState)
	})
}
