package indexer

import (
	"math/big"
	"testing"

	uniswapv3 "github.com/defistate/defistate-client-go/protocols/uniswapv3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexableUniswapV3System(t *testing.T) {
	// --- Test Data Setup ---
	testPools := []uniswapv3.Pool{
		{
			PoolViewMinimal: uniswapv3.PoolViewMinimal{
				ID:           201,
				Token0:       0,
				Token1:       1,
				Tick:         200000,
				Liquidity:    big.NewInt(1234567890),
				SqrtPriceX96: big.NewInt(5602277097478614198),
			},
			Ticks: []uniswapv3.TickInfo{
				{Index: 199980, LiquidityGross: big.NewInt(10000), LiquidityNet: big.NewInt(10000)},
				{Index: 200040, LiquidityGross: big.NewInt(10000), LiquidityNet: big.NewInt(-10000)},
			},
		},
		{
			PoolViewMinimal: uniswapv3.PoolViewMinimal{
				ID:           202,
				Token0:       2,
				Token1:       3,
				Tick:         -50000,
				Liquidity:    big.NewInt(9876543210),
				SqrtPriceX96: big.NewInt(7922816251426433759),
			},
			Ticks: []uniswapv3.TickInfo{
				{Index: -50010, LiquidityGross: big.NewInt(5000), LiquidityNet: big.NewInt(5000)},
			},
		},
	}

	// Create the indexer instance to be tested.
	indexer := NewIndexableUniswapV3System(testPools)
	require.NotNil(t, indexer)

	// --- Sub-tests for different scenarios ---

	t.Run("Successful Lookups", func(t *testing.T) {
		// Test GetByID
		pool, found := indexer.GetByID(201)
		assert.True(t, found, "Pool should be found by ID 201")
		assert.Equal(t, uint64(0), pool.Token0)
		assert.Equal(t, int64(200000), pool.Tick)
		// Assert that the tick info was correctly included.
		require.Len(t, pool.Ticks, 2, "Pool should have 2 ticks")
		assert.Equal(t, int64(199980), pool.Ticks[0].Index)
	})

	t.Run("Not Found Lookups", func(t *testing.T) {
		// Test GetByID with a non-existent ID
		_, found := indexer.GetByID(999)
		assert.False(t, found, "Should not find a pool with ID 999")
	})

	t.Run("All Method", func(t *testing.T) {
		allPools := indexer.All()
		assert.Len(t, allPools, 2, "All() should return 2 pools")

		// Verify it's a copy by modifying the returned slice and checking the original
		if len(allPools) > 0 {
			allPools[0].Tick = -1 // Modify the copy
			originalPool, _ := indexer.GetByID(201)
			assert.Equal(t, int64(200000), originalPool.Tick, "Modifying the returned slice should not affect the internal state")
		}
	})

	t.Run("Edge Case - Empty Slice", func(t *testing.T) {
		emptyIndexer := NewIndexableUniswapV3System([]uniswapv3.Pool{})
		require.NotNil(t, emptyIndexer)

		_, found := emptyIndexer.GetByID(1)
		assert.False(t, found)

		allPools := emptyIndexer.All()
		assert.Len(t, allPools, 0)
	})

	t.Run("Edge Case - Nil Slice", func(t *testing.T) {
		nilIndexer := NewIndexableUniswapV3System(nil)
		require.NotNil(t, nilIndexer)

		_, found := nilIndexer.GetByID(1)
		assert.False(t, found)

		allPools := nilIndexer.All()
		assert.Len(t, allPools, 0)
		assert.NotNil(t, allPools, "All() should return an empty slice, not nil")
	})
}
