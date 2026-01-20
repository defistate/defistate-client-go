package indexer

import (
	"math/big"
	"testing"

	uniswapv2 "github.com/defistate/defistate-client-go/protocols/uniswapv2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexableUniswapV2System(t *testing.T) {
	// --- Test Data Setup ---
	testPools := []uniswapv2.Pool{
		{ID: 101, Token0: 0, Token1: 1, Reserve0: big.NewInt(1000), Reserve1: big.NewInt(2000)},
		{ID: 102, Token0: 2, Token1: 3, Reserve0: big.NewInt(3000), Reserve1: big.NewInt(4000)},
	}

	// Create the indexer instance to be tested.
	indexer := NewIndexableUniswapV2System(testPools)
	require.NotNil(t, indexer)

	// --- Sub-tests for different scenarios ---

	t.Run("Successful Lookups", func(t *testing.T) {
		// Test GetByID
		pool, found := indexer.GetByID(101)
		assert.True(t, found, "Pool should be found by ID 101")
		assert.Equal(t, uint64(0), pool.Token0)
		assert.Equal(t, int64(1000), pool.Reserve0.Int64())
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
			allPools[0].Token0 = 99 // Modify the copy
			originalPool, _ := indexer.GetByID(101)
			assert.Equal(t, uint64(0), originalPool.Token0, "Modifying the returned slice should not affect the internal state")
		}
	})

	t.Run("Edge Case - Empty Slice", func(t *testing.T) {
		emptyIndexer := NewIndexableUniswapV2System([]uniswapv2.Pool{})
		require.NotNil(t, emptyIndexer)

		_, found := emptyIndexer.GetByID(1)
		assert.False(t, found)

		allPools := emptyIndexer.All()
		assert.Len(t, allPools, 0)
	})

	t.Run("Edge Case - Nil Slice", func(t *testing.T) {
		nilIndexer := NewIndexableUniswapV2System(nil)
		require.NotNil(t, nilIndexer)

		_, found := nilIndexer.GetByID(1)
		assert.False(t, found)

		allPools := nilIndexer.All()
		assert.Len(t, allPools, 0)
		assert.NotNil(t, allPools, "All() should return an empty slice, not nil")
	})
}
